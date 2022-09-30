// Copyright 2020 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/xmidt-org/ears/internal/pkg/rtsemconv"
	"github.com/xmidt-org/ears/pkg/event"
	pkgplugin "github.com/xmidt-org/ears/pkg/plugin"
	"github.com/xmidt-org/ears/pkg/secret"
	"github.com/xmidt-org/ears/pkg/sender"
	"github.com/xmidt-org/ears/pkg/tenant"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/unit"
	"os"
	"time"
)

//TODO: MessageAttributes
//TODO: improve graceful shutdown

func NewSender(tid tenant.Id, plugin string, name string, config interface{}, secrets secret.Vault) (sender.Sender, error) {
	var cfg SenderConfig
	var err error
	switch c := config.(type) {
	case string:
		err = yaml.Unmarshal([]byte(c), &cfg)
	case []byte:
		err = yaml.Unmarshal(c, &cfg)
	case SenderConfig:
		cfg = c
	case *SenderConfig:
		cfg = *c
	}
	if err != nil {
		return nil, &pkgplugin.InvalidConfigError{
			Err: err,
		}
	}
	cfg = cfg.WithDefaults()
	err = cfg.Validate()
	if err != nil {
		return nil, err
	}
	s := &Sender{
		name:   name,
		plugin: plugin,
		tid:    tid,
		config: cfg,
		logger: event.GetEventLogger(),
	}
	s.initPlugin()
	// metric recorders
	hostname, _ := os.Hostname()
	meter := global.Meter(rtsemconv.EARSMeterName)
	s.commonLabels = []attribute.KeyValue{
		attribute.String(rtsemconv.EARSPluginTypeLabel, rtsemconv.EARSPluginTypeSQSSender),
		attribute.String(rtsemconv.EARSPluginNameLabel, s.Name()),
		attribute.String(rtsemconv.EARSAppIdLabel, s.tid.AppId),
		attribute.String(rtsemconv.EARSOrgIdLabel, s.tid.OrgId),
		attribute.String(rtsemconv.SQSQueueUrlLabel, s.config.QueueUrl),
		attribute.String(rtsemconv.HostnameLabel, hostname),
	}
	s.eventSuccessCounter = metric.Must(meter).
		NewInt64Counter(
			rtsemconv.EARSMetricEventSuccess,
			metric.WithDescription("measures the number of successful events"),
		)
	s.eventFailureCounter = metric.Must(meter).
		NewInt64Counter(
			rtsemconv.EARSMetricEventFailure,
			metric.WithDescription("measures the number of unsuccessful events"),
		).Bind(s.commonLabels...)
	s.eventBytesCounter = metric.Must(meter).
		NewInt64Counter(
			rtsemconv.EARSMetricEventBytes,
			metric.WithDescription("measures the number of event bytes processed"),
			metric.WithUnit(unit.Bytes),
		).Bind(s.commonLabels...)
	s.eventProcessingTime = metric.Must(meter).
		NewInt64Histogram(
			rtsemconv.EARSMetricEventProcessingTime,
			metric.WithDescription("measures the time an event spends in ears"),
			metric.WithUnit(unit.Milliseconds),
		).Bind(s.commonLabels...)
	s.eventSendOutTime = metric.Must(meter).
		NewInt64Histogram(
			rtsemconv.EARSMetricEventSendOutTime,
			metric.WithDescription("measures the time ears spends to send an event to a downstream data sink"),
			metric.WithUnit(unit.Milliseconds),
		).Bind(s.commonLabels...)
	return s, nil
}

func (s *Sender) initPlugin() error {
	s.Lock()
	defer s.Unlock()
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(endpoints.UsWest2RegionID),
	})
	if nil != err {
		return err
	}
	_, err = sess.Config.Credentials.Get()
	if nil != err {
		return err
	}
	s.sqsService = sqs.New(sess)
	s.done = make(chan struct{})
	s.startTimedSender()
	return nil
}

func (s *Sender) startTimedSender() {
	go func() {
		for {
			select {
			case <-s.done:
				s.logger.Info().Str("op", "SQS.timedSender").Str("name", s.Name()).Str("tid", s.Tenant().ToString()).Int("sendCount", s.count).Msg("stopping sqs sender")
				return
			case <-time.After(time.Duration(*s.config.SendTimeout) * time.Second):
			}
			s.Lock()
			if s.eventBatch == nil {
				s.eventBatch = make([]event.Event, 0)
			}
			evtBatch := s.eventBatch
			s.eventBatch = make([]event.Event, 0)
			s.Unlock()
			if len(evtBatch) > 0 {
				s.send(evtBatch)
			}
		}
	}()
}

func (s *Sender) Count() int {
	s.Lock()
	defer s.Unlock()
	return s.count
}

func (s *Sender) StopSending(ctx context.Context) {
	s.Lock()
	if s.done != nil {
		//s.eventSuccessCounter.Unbind()
		s.eventFailureCounter.Unbind()
		s.eventBytesCounter.Unbind()
		s.eventProcessingTime.Unbind()
		s.eventSendOutTime.Unbind()
		s.done <- struct{}{}
		s.done = nil
	}
	s.Unlock()
}

func (s *Sender) send(events []event.Event) {
	if len(events) == 0 {
		return
	}
	entries := make([]*sqs.SendMessageBatchRequestEntry, 0)
	for idx, evt := range events {
		if idx == 0 {
			log.Ctx(evt.Context()).Debug().Str("op", "SQS.sendWorker").Str("name", s.Name()).Str("tid", s.Tenant().ToString()).Int("eventIdx", idx).Int("batchSize", len(events)).Int("sendCount", s.count).Msg("send message batch")
		}
		buf, err := json.Marshal(evt.Payload())
		if err != nil {
			continue
		}
		attributes := make(map[string]*sqs.MessageAttributeValue)
		entry := &sqs.SendMessageBatchRequestEntry{
			Id:          aws.String(evt.Id()),
			MessageBody: aws.String(string(buf)),
		}
		otel.GetTextMapPropagator().Inject(evt.Context(), NewSqsMessageAttributeCarrier(attributes))
		if len(attributes) > 0 {
			entry.MessageAttributes = attributes
		}
		if *s.config.DelaySeconds > 0 {
			entry.DelaySeconds = aws.Int64(int64(*s.config.DelaySeconds))
		}
		entries = append(entries, entry)
		s.eventBytesCounter.Add(evt.Context(), int64(len(buf)))
		s.eventProcessingTime.Record(evt.Context(), time.Since(evt.Created()).Milliseconds())
	}
	sqsSendBatchParams := &sqs.SendMessageBatchInput{
		Entries:  entries,
		QueueUrl: aws.String(s.config.QueueUrl),
	}
	start := time.Now()
	output, err := s.sqsService.SendMessageBatch(sqsSendBatchParams)
	s.eventSendOutTime.Record(events[0].Context(), time.Since(start).Milliseconds())
	if err != nil {
		for _, evt := range events {
			s.eventFailureCounter.Add(evt.Context(), 1)
			evt.Nack(err)
			break
		}
	} else {
		for _, failEvent := range output.Failed {
			for _, evt := range events {
				if evt.Id() == *failEvent.Id {
					s.eventFailureCounter.Add(evt.Context(), 1)
					evt.Nack(errors.New(*failEvent.Message))
					break
				}
			}
		}
		for _, successEvent := range output.Successful {
			for _, evt := range events {
				if evt.Id() == *successEvent.Id {
					s.eventSuccessCounter.Add(evt.Context(), 1, s.commonLabels...)
					evt.Ack()
					s.Lock()
					s.count++
					s.Unlock()
					break
				}
			}
		}
	}
}

func (s *Sender) Send(e event.Event) {
	s.Lock()
	if s.eventBatch == nil {
		s.eventBatch = make([]event.Event, 0)
	}
	s.eventBatch = append(s.eventBatch, e)
	if len(s.eventBatch) >= *s.config.MaxNumberOfMessages {
		eventBatch := s.eventBatch
		s.eventBatch = make([]event.Event, 0)
		s.Unlock()
		s.send(eventBatch)
	} else {
		s.Unlock()
	}
}

func (s *Sender) Unwrap() sender.Sender {
	return s
}

func (s *Sender) Config() interface{} {
	return s.config
}

func (s *Sender) Name() string {
	return s.name
}

func (s *Sender) Plugin() string {
	return s.plugin
}

func (s *Sender) Tenant() tenant.Id {
	return s.tid
}
