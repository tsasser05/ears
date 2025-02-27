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

package kinesis

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kinesis"
	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/xmidt-org/ears/internal/pkg/rtsemconv"
	"github.com/xmidt-org/ears/pkg/checkpoint"
	"github.com/xmidt-org/ears/pkg/event"
	"github.com/xmidt-org/ears/pkg/panics"
	pkgplugin "github.com/xmidt-org/ears/pkg/plugin"
	"github.com/xmidt-org/ears/pkg/receiver"
	"github.com/xmidt-org/ears/pkg/secret"
	"github.com/xmidt-org/ears/pkg/sharder"
	"github.com/xmidt-org/ears/pkg/tenant"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/unit"
	"golang.org/x/net/http2"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	errorTimeoutSecShort   = 1
	errorTimeoutSecLong    = 5
	monitorTimeoutSecShort = 10
	monitorTimeoutSecLong  = 30
)

// https://docs.aws.amazon.com/streams/latest/dev/troubleshooting-consumers.html

func NewReceiver(tid tenant.Id, plugin string, name string, config interface{}, secrets secret.Vault) (receiver.Receiver, error) {
	var cfg ReceiverConfig
	var err error
	switch c := config.(type) {
	case string:
		err = yaml.Unmarshal([]byte(c), &cfg)
	case []byte:
		err = yaml.Unmarshal(c, &cfg)
	case ReceiverConfig:
		cfg = c
	case *ReceiverConfig:
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
	r := &Receiver{
		config:                         cfg,
		name:                           name,
		plugin:                         plugin,
		tid:                            tid,
		logger:                         event.GetEventLogger(),
		stopped:                        true,
		secrets:                        secrets,
		shardMonitorStopChannel:        make(chan bool),
		shardUpdateListenerStopChannel: make(chan bool),
	}
	r.Lock()
	r.stopChannelMap = make(map[int]chan bool)
	r.Unlock()
	hostname, _ := os.Hostname()
	// metric recorders
	meter := global.Meter(rtsemconv.EARSMeterName)
	commonLabels := []attribute.KeyValue{
		attribute.String(rtsemconv.EARSPluginTypeLabel, rtsemconv.EARSPluginTypeKinesisReceiver),
		attribute.String(rtsemconv.EARSPluginNameLabel, r.Name()),
		attribute.String(rtsemconv.EARSAppIdLabel, r.tid.AppId),
		attribute.String(rtsemconv.EARSOrgIdLabel, r.tid.OrgId),
		attribute.String(rtsemconv.KinesisStreamNameLabel, r.config.StreamName),
		attribute.String(rtsemconv.HostnameLabel, hostname),
	}
	r.eventSuccessCounter = metric.Must(meter).
		NewInt64Counter(
			rtsemconv.EARSMetricEventSuccess,
			metric.WithDescription("measures the number of successful events"),
		).Bind(commonLabels...)
	r.eventFailureCounter = metric.Must(meter).
		NewInt64Counter(
			rtsemconv.EARSMetricEventFailure,
			metric.WithDescription("measures the number of unsuccessful events"),
		).Bind(commonLabels...)
	r.eventBytesCounter = metric.Must(meter).
		NewInt64Counter(
			rtsemconv.EARSMetricEventBytes,
			metric.WithDescription("measures the number of event bytes processed"),
			metric.WithUnit(unit.Bytes),
		).Bind(commonLabels...)
	//defer eventLagMillis to startShardReceiverEFO() because shardIdx is wanted as tag
	return r, nil
}

//TODO: generic solution for retry policy on nack
//TODO: checkpoint to advance on nack after final retry

func (r *Receiver) getStopChannel(shardIdx int) chan bool {
	var c chan bool
	r.Lock()
	c = r.stopChannelMap[shardIdx]
	r.Unlock()
	return c
}

func (r *Receiver) stopShardReceiver(shardIdx int) {
	if shardIdx < 0 {
		r.Lock()
		m := r.stopChannelMap
		for _, stopChan := range m {
			go func(stopChan chan bool) { stopChan <- true }(stopChan)
		}
		r.Unlock()
	} else {
		stopChan := r.getStopChannel(shardIdx)
		if stopChan != nil {
			stopChan <- true
		}
	}
}

func (r *Receiver) getCheckpointId(shardID int) string {
	return r.name + "-" + r.config.ConsumerName + "-" + r.config.StreamName + "-" + strconv.Itoa(shardID)
}

func (r *Receiver) shardMonitor(svc *kinesis.Kinesis, distributor sharder.ShardDistributor) {
	r.logger.Info().Str("op", "Kinesis.shardMonitor").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("starting shard monitor")
	go func() {
		defer func() {
			p := recover()
			if p != nil {
				panicErr := panics.ToError(p)
				r.logger.Error().Str("op", "Kinesis.shardMonitor").Str("error", panicErr.Error()).
					Str("stackTrace", panicErr.StackTrace()).Msg("a panic has occurred in shard monitor")
			}
		}()
		for {
			// the stop function will wait for 5 sec in case the shard monitor is busy describing the stream and not waiting on the channel
			select {
			case <-r.shardMonitorStopChannel:
				r.logger.Info().Str("op", "Kinesis.shardMonitor").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("stopping shard monitor")
				return
			case <-time.After(monitorTimeoutSecShort * time.Second):
			}
			if !*r.config.UseShardMonitor {
				// keep the shard monitor on, so we keep listening on the stop channel
				//r.logger.Info().Str("op", "Kinesis.shardMonitor").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("shard monitor disabled")
				continue
			}
			stream, err := svc.DescribeStream(&kinesis.DescribeStreamInput{
				StreamName: &r.config.StreamName,
			})
			if err != nil {
				r.logger.Error().Str("op", "Kinesis.shardMonitor").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("cannot get number of shards: " + err.Error())
				continue
			}
			if len(stream.StreamDescription.Shards) == 0 {
				continue
			}
			update := false
			if len(stream.StreamDescription.Shards) != len(r.stream.StreamDescription.Shards) {
				update = true
			}
			if update {
				// should we wait here until kinesis is ready? compare with code for registering efo consumer!
				r.logger.Info().Str("op", "Kinesis.shardMonitor").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("numShards", len(stream.StreamDescription.Shards)).Msg("update number of shards")
				r.stream = stream
				// this will trigger updates on the
				distributor.UpdateNumberShards(len(stream.StreamDescription.Shards))
			}
		}
	}()
}

func (r *Receiver) registerStreamConsumer(svc *kinesis.Kinesis, streamName, consumerName string) (*kinesis.DescribeStreamConsumerOutput, error) {
	stream, err := svc.DescribeStream(&kinesis.DescribeStreamInput{
		StreamName: &streamName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe stream, %s, %v", streamName, err)
	}
	descParams := &kinesis.DescribeStreamConsumerInput{
		StreamARN:    stream.StreamDescription.StreamARN,
		ConsumerName: &consumerName,
	}
	_, err = svc.DescribeStreamConsumer(descParams)
	if aerr, ok := err.(awserr.Error); ok && aerr.Code() == kinesis.ErrCodeResourceNotFoundException {
		_, err := svc.RegisterStreamConsumer(
			&kinesis.RegisterStreamConsumerInput{
				ConsumerName: aws.String(consumerName),
				StreamARN:    stream.StreamDescription.StreamARN,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream consumer %s, %v", consumerName, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to describe stream consumer %s, %v", consumerName, err)
	}
	for i := 0; i < 10; i++ {
		streamConsumer, err := svc.DescribeStreamConsumer(descParams)
		if err != nil || aws.StringValue(streamConsumer.ConsumerDescription.ConsumerStatus) != kinesis.ConsumerStatusActive {
			time.Sleep(time.Second * monitorTimeoutSecLong)
			continue
		}
		return streamConsumer, nil
	}
	return nil, fmt.Errorf("failed to wait for consumer to exist, %v, %v", *descParams.StreamARN, *descParams.ConsumerName)
}

func (r *Receiver) startShardReceiverEFO(svc *kinesis.Kinesis, stream *kinesis.DescribeStreamOutput, consumer *kinesis.DescribeStreamConsumerOutput, shardIdx int) {
	// metric recorders
	meter := global.Meter(rtsemconv.EARSMeterName)
	commonLabels := []attribute.KeyValue{
		attribute.String(rtsemconv.EARSPluginTypeLabel, rtsemconv.EARSPluginTypeKinesisReceiver),
		attribute.String(rtsemconv.EARSPluginNameLabel, r.Name()),
		attribute.String(rtsemconv.EARSAppIdLabel, r.tid.AppId),
		attribute.String(rtsemconv.EARSOrgIdLabel, r.tid.OrgId),
		attribute.String(rtsemconv.KinesisStreamNameLabel, r.config.StreamName),
		attribute.Int(rtsemconv.KinesisShardIdxLabel, shardIdx),
	}
	r.eventLagMillis = metric.Must(meter).
		NewInt64Histogram(
			rtsemconv.EARSMetricMillisBehindLatest,
			metric.WithDescription("measures the number of milliseconds current event is behind tip of stream"),
			metric.WithUnit(unit.Milliseconds),
		).Bind(commonLabels...)
	r.eventTrueLagMillis = metric.Must(meter).
		NewInt64Histogram(
			rtsemconv.EARSMetricTrueLagMillis,
			metric.WithDescription("measures the number of milliseconds current event is behind tip of stream"),
			metric.WithUnit(unit.Milliseconds),
		).Bind(commonLabels...)
	// this is an enhanced consumer that will only consume from a dedicated shard
	go func() {
		defer func() {
			p := recover()
			if p != nil {
				panicErr := panics.ToError(p)
				r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("error", panicErr.Error()).
					Str("stackTrace", panicErr.StackTrace()).Msg("a panic has occurred in efo shard receiver")
			}
		}()
		r.Lock()
		r.stopChannelMap[shardIdx] = make(chan bool)
		r.Unlock()
		checkpoint, err := checkpoint.GetDefaultCheckpointManager(nil)
		if err != nil {
			r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("checkpoint error: " + err.Error())
			return
		}
		checkpointId := r.getCheckpointId(shardIdx)
		// note, there may be dead parent shards in the mix
		shard := stream.StreamDescription.Shards[shardIdx]
		params := &kinesis.SubscribeToShardInput{
			ConsumerARN: consumer.ConsumerDescription.ConsumerARN,
			StartingPosition: &kinesis.StartingPosition{
				Type: aws.String(r.config.ShardIteratorType),
				//SequenceNumber: aws.String(""),
				//Timestamp: aws.Time(time.Now()),
			},
			ShardId: shard.ShardId,
		}
		if *r.config.UseCheckpoint {
			sequenceId, lastUpdate, err := checkpoint.GetCheckpoint(checkpointId)
			if err == nil {
				if int(time.Since(lastUpdate).Seconds()) <= *r.config.MaxCheckpointAgeSeconds {
					params.StartingPosition.Type = aws.String(kinesis.ShardIteratorTypeAtSequenceNumber)
					params.StartingPosition.SequenceNumber = aws.String(sequenceId)
				}
			} else {
				r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("checkpoint error: " + err.Error())
			}
			//} else if *r.config.StartingTimestamp > 0 {
			//	params.StartingPosition.Type = aws.String(kinesis.ShardIteratorTypeAtTimestamp)
			//	params.StartingPosition.Timestamp = aws.Time(time.UnixMilli(*r.config.StartingTimestamp))
		} else if r.config.StartingSequenceNumber != "" {
			params.StartingPosition.Type = aws.String(kinesis.ShardIteratorTypeAtSequenceNumber)
			params.StartingPosition.SequenceNumber = aws.String(r.config.StartingSequenceNumber)
		}
		for {
			select {
			case <-r.getStopChannel(shardIdx):
				r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("receive loop stopped")
				r.Lock()
				close(r.stopChannelMap[shardIdx])
				delete(r.stopChannelMap, shardIdx)
				r.Unlock()
				return
			default:
			}
			sub, err := svc.SubscribeToShard(params)
			if err != nil {
				r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("subscribe error: " + err.Error())
				// a fully processed parent shard will cause errors here until its decommission
				time.Sleep(errorTimeoutSecLong * time.Second)
				continue
			} else {
				r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("subscribe to shard")
				osstr := ""
				for idx, os := range r.shardConfig.OwnedShards {
					if idx > 0 {
						osstr += ", "
					}
					osstr += os
				}
				r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).
					Int("num_shards", r.shardConfig.NumShards).Int("num_owned_shards", len(r.shardConfig.OwnedShards)).Str("owned_shards", osstr).Msg("report owned shards")
			}
			for evt := range sub.EventStream.Events() {
				select {
				case <-r.getStopChannel(shardIdx):
					r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("receive loop stopped")
					sub.EventStream.Close()
					r.Lock()
					close(r.stopChannelMap[shardIdx])
					delete(r.stopChannelMap, shardIdx)
					r.Unlock()
					return
				default:
				}
				switch kinEvt := evt.(type) {
				case *kinesis.SubscribeToShardEvent:
					if kinEvt.ContinuationSequenceNumber != nil {
						params.StartingPosition.Type = aws.String(kinesis.ShardIteratorTypeAtSequenceNumber)
						params.StartingPosition.SequenceNumber = kinEvt.ContinuationSequenceNumber
						//r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Str("continuationSequenceNumber", *kinEvt.ContinuationSequenceNumber).Msg("subscription confirmation")
					} else {
						// shard is closed, wait for shard decommission
						for {
							select {
							case <-r.getStopChannel(shardIdx):
								r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("receive loop stopped")
								r.Lock()
								close(r.stopChannelMap[shardIdx])
								delete(r.stopChannelMap, shardIdx)
								r.Unlock()
								return
							case <-time.After(monitorTimeoutSecShort * time.Second):
								r.logger.Info().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("waiting for decommission")
							}
						}
					}
					for _, rec := range kinEvt.Records {
						if len(rec.Data) == 0 {
							continue
						} else {
							r.Lock()
							r.receiveCount++
							r.Unlock()
							var payload interface{}
							err = json.Unmarshal(rec.Data, &payload)
							if err != nil {
								r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("cannot parse message " + (*rec.SequenceNumber) + ": " + err.Error())
								continue
							}
							ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*r.config.AcknowledgeTimeout)*time.Second)
							r.eventBytesCounter.Add(ctx, int64(len(rec.Data)))
							r.eventLagMillis.Record(ctx, *kinEvt.MillisBehindLatest)
							trueLagMillis := time.Since(*rec.ApproximateArrivalTimestamp).Milliseconds()
							r.eventTrueLagMillis.Record(ctx, trueLagMillis)
							r.logger.Debug().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Str("partitionId", *rec.PartitionKey).Str("sequenceId", *rec.SequenceNumber).Msg("message received")
							e, err := event.New(ctx, payload, event.WithMetadataKeyValue("kinesisMessage", rec), event.WithAck(
								func(e event.Event) {
									r.eventSuccessCounter.Add(ctx, 1)
									if kinEvt.ContinuationSequenceNumber != nil {
										checkpoint.SetCheckpoint(checkpointId, *kinEvt.ContinuationSequenceNumber)
									}
									cancel()
								},
								func(e event.Event, err error) {
									r.eventFailureCounter.Add(ctx, 1)
									if kinEvt.ContinuationSequenceNumber != nil {
										checkpoint.SetCheckpoint(checkpointId, *kinEvt.ContinuationSequenceNumber)
									}
									cancel()
								}),
								event.WithTenant(r.Tenant()),
								event.WithOtelTracing(r.Name()),
								event.WithTracePayloadOnNack(*r.config.TracePayloadOnNack))
							if err != nil {
								r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("cannot create event: " + err.Error())
								return
							}
							r.Trigger(e)
						}
					}
				}
			}
			if err := sub.EventStream.Err(); err != nil {
				r.logger.Error().Str("op", "kinesis.startShardReceiverEFO").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("subscription event stream error: " + err.Error())
				if sub != nil && sub.EventStream != nil && sub.EventStream.Reader != nil {
					sub.EventStream.Close()
				}
				time.Sleep(1 * time.Second)
				continue
			}
			if sub != nil && sub.EventStream != nil && sub.EventStream.Reader != nil {
				sub.EventStream.Close()
			}
		}
	}()
}

func (r *Receiver) startShardReceiver(svc *kinesis.Kinesis, stream *kinesis.DescribeStreamOutput, shardIdx int) {
	// metric recorders
	meter := global.Meter(rtsemconv.EARSMeterName)
	commonLabels := []attribute.KeyValue{
		attribute.String(rtsemconv.EARSPluginTypeLabel, rtsemconv.EARSPluginTypeKinesisReceiver),
		attribute.String(rtsemconv.EARSPluginNameLabel, r.Name()),
		attribute.String(rtsemconv.EARSAppIdLabel, r.tid.AppId),
		attribute.String(rtsemconv.EARSOrgIdLabel, r.tid.OrgId),
		attribute.String(rtsemconv.KinesisStreamNameLabel, r.config.StreamName),
		attribute.Int(rtsemconv.KinesisShardIdxLabel, shardIdx),
	}

	r.eventLagMillis = metric.Must(meter).
		NewInt64Histogram(
			rtsemconv.EARSMetricMillisBehindLatest,
			metric.WithDescription("measures the number of milliseconds current event is behind tip of stream"),
			metric.WithUnit(unit.Milliseconds),
		).Bind(commonLabels...)

	// this is a non-enhanced consumer that will only consume from one shard
	// n is number of worker in pool
	go func() {
		defer func() {
			p := recover()
			if p != nil {
				panicErr := panics.ToError(p)
				r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("error", panicErr.Error()).
					Str("stackTrace", panicErr.StackTrace()).Msg("a panic has occurred in shard receiver")
			}
		}()
		r.Lock()
		r.stopChannelMap[shardIdx] = make(chan bool)
		r.Unlock()
		checkpoint, err := checkpoint.GetDefaultCheckpointManager(nil)
		if err != nil {
			r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("checkpoint error: " + err.Error())
			return
		}
		checkpointId := r.getCheckpointId(shardIdx)
		// receive messages
		for {
			// this is a normal receiver
			//startingTimestamp := time.Now().Add(-(time.Second) * 30)
			shardIteratorInput := &kinesis.GetShardIteratorInput{
				ShardId:           aws.String(*stream.StreamDescription.Shards[shardIdx].ShardId),
				ShardIteratorType: aws.String(r.config.ShardIteratorType),
				// Timestamp:         aws.Time(startingTimestamp),
				// StartingSequenceNumber: aws.String(""),
				StreamName: aws.String(r.config.StreamName),
			}
			if *r.config.UseCheckpoint {
				sequenceId, lastUpdate, err := checkpoint.GetCheckpoint(checkpointId)
				if err == nil {
					if int(time.Since(lastUpdate).Seconds()) <= *r.config.MaxCheckpointAgeSeconds {
						shardIteratorInput.ShardIteratorType = aws.String(kinesis.ShardIteratorTypeAfterSequenceNumber)
						shardIteratorInput.StartingSequenceNumber = aws.String(sequenceId)
					}
				} else {
					r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("checkpoint error: " + err.Error())
				}
				//} else if *r.config.StartingTimestamp > 0 {
				//	shardIteratorInput.ShardIteratorType = aws.String(kinesis.ShardIteratorTypeAtTimestamp)
				//	shardIteratorInput.Timestamp = aws.Time(time.UnixMilli(*r.config.StartingTimestamp))
			} else if r.config.StartingSequenceNumber != "" {
				shardIteratorInput.ShardIteratorType = aws.String(kinesis.ShardIteratorTypeAtSequenceNumber)
				shardIteratorInput.StartingSequenceNumber = aws.String(r.config.StartingSequenceNumber)
			}
			iteratorOutput, err := svc.GetShardIterator(shardIteratorInput)
			if err != nil {
				r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg(err.Error())
				time.Sleep(errorTimeoutSecShort * time.Second)
				continue
			}
			shardIterator := iteratorOutput.ShardIterator
			for {
				select {
				case <-r.getStopChannel(shardIdx):
					r.logger.Info().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("receive loop stopped")
					r.Lock()
					close(r.stopChannelMap[shardIdx])
					delete(r.stopChannelMap, shardIdx)
					r.Unlock()
					return
				default:
				}
				getRecordsOutput, err := svc.GetRecords(&kinesis.GetRecordsInput{
					ShardIterator: shardIterator,
				})
				if err != nil {
					r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg(err.Error())
					time.Sleep(errorTimeoutSecShort * time.Second)
					continue
				}
				records := getRecordsOutput.Records
				if len(records) > 0 {
					r.Lock()
					r.logger.Debug().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("receiveCount", r.receiveCount).Int("batchSize", len(records)).Int("shardIdx", shardIdx).Msg("received message batch")
					r.Unlock()
				} else {
					r.logger.Debug().Str("op", "kinesis.startShardReceiver").Msg("no new records")
				}
				for _, msg := range records {
					if len(msg.Data) == 0 {
						continue
					} else {
						go func() {
							r.Lock()
							r.receiveCount++
							r.Unlock()
							var payload interface{}
							err = json.Unmarshal(msg.Data, &payload)
							if err != nil {
								r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("cannot parse message " + (*msg.SequenceNumber) + ": " + err.Error())
								return
							}
							ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*r.config.AcknowledgeTimeout)*time.Second)
							r.eventLagMillis.Record(ctx, *getRecordsOutput.MillisBehindLatest)
							trueLagMillis := time.Since(*msg.ApproximateArrivalTimestamp).Milliseconds()
							r.eventTrueLagMillis.Record(ctx, trueLagMillis)
							r.eventBytesCounter.Add(ctx, int64(len(msg.Data)))
							e, err := event.New(ctx, payload, event.WithMetadataKeyValue("kinesisMessage", *msg), event.WithAck(
								func(e event.Event) {
									r.eventSuccessCounter.Add(ctx, 1)
									checkpoint.SetCheckpoint(checkpointId, *msg.SequenceNumber)
									cancel()
								},
								func(e event.Event, err error) {
									r.eventFailureCounter.Add(ctx, 1)
									checkpoint.SetCheckpoint(checkpointId, *msg.SequenceNumber)
									cancel()
								}),
								event.WithTenant(r.Tenant()),
								event.WithOtelTracing(r.Name()),
								event.WithTracePayloadOnNack(*r.config.TracePayloadOnNack))
							if err != nil {
								r.logger.Error().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("cannot create event: " + err.Error())
								return
							}
							r.Trigger(e)
						}()
					}
				}
				// this the next sequence number to read from in the shard, if nil the shard has been closed pending expiration
				shardIterator = getRecordsOutput.NextShardIterator
				if shardIterator == nil {
					for {
						// just wait for shard decommission
						select {
						case <-r.getStopChannel(shardIdx):
							r.logger.Info().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("receive loop stopped")
							r.Lock()
							close(r.stopChannelMap[shardIdx])
							delete(r.stopChannelMap, shardIdx)
							r.Unlock()
							return
						case <-time.After(monitorTimeoutSecShort * time.Second):
							r.logger.Info().Str("op", "kinesis.startShardReceiver").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("waiting for decommission")
						}
					}
				}
			}
		}
	}()
}

func (r *Receiver) shardUpdateListener(distributor sharder.ShardDistributor) {
	go func() {
		defer func() {
			p := recover()
			if p != nil {
				panicErr := panics.ToError(p)
				r.logger.Error().Str("op", "kinesis.shardUpdateListener").Str("error", panicErr.Error()).
					Str("stackTrace", panicErr.StackTrace()).Msg("a panic has occurred in shard update listener")
			}
		}()
		for {
			select {
			case config := <-distributor.Updates():
				r.logger.Info().Str("op", "kinesis.shardUpdateListener").Str("stream", *r.stream.StreamDescription.StreamName).Str("identity", config.Identity).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("shard update received")
				r.updateShards(config)
			case <-r.shardUpdateListenerStopChannel:
				r.logger.Info().Str("op", "kinesis.shardUpdateListener").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("stopping shard update listener")
				return
			}
		}
	}()
}

func (r *Receiver) updateShards(newShards sharder.ShardConfig) {
	// shut down old shards not needed any more
	for _, oldShardStr := range r.shardConfig.OwnedShards {
		shutDown := true
		for _, newShardStr := range newShards.OwnedShards {
			if newShardStr == oldShardStr {
				shutDown = false
			}
		}
		if shutDown {
			shardIdx, err := strconv.Atoi(oldShardStr)
			if err != nil {
				continue
			}
			r.logger.Info().Str("op", "Kinesis.UpdateShards").Str("stream", *r.stream.StreamDescription.StreamName).Str("identity", newShards.Identity).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("stopping shard consumer")
			r.stopShardReceiver(shardIdx)
		}
	}
	// start new shards
	for _, newShardStr := range newShards.OwnedShards {
		startUp := true
		for _, oldShardStr := range r.shardConfig.OwnedShards {
			if newShardStr == oldShardStr {
				startUp = false
			}
		}
		if startUp {
			shardIdx, err := strconv.Atoi(newShardStr)
			if err != nil {
				r.logger.Error().Str("op", "Kinesis.UpdateShards").Str("stream", *r.stream.StreamDescription.StreamName).Str("identity", newShards.Identity).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg(err.Error())
				continue
			}
			if *r.config.EnhancedFanOut {
				r.logger.Info().Str("op", "Kinesis.UpdateShards").Str("stream", *r.stream.StreamDescription.StreamName).Str("identity", newShards.Identity).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("launching efo shard consumer")
				r.startShardReceiverEFO(r.svc, r.stream, r.consumer, shardIdx)
			} else {
				r.logger.Info().Str("op", "Kinesis.UpdateShards").Str("stream", *r.stream.StreamDescription.StreamName).Str("identity", newShards.Identity).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("shardIdx", shardIdx).Msg("launching shard consumer")
				r.startShardReceiver(r.svc, r.stream, shardIdx)
			}
		}
	}
	r.shardConfig = newShards
}

func (r *Receiver) Receive(next receiver.NextFn) error {
	if r == nil {
		return &pkgplugin.Error{
			Err: fmt.Errorf("Receive called on <nil> pointer"),
		}
	}
	if next == nil {
		return &receiver.InvalidConfigError{
			Err: fmt.Errorf("next cannot be nil"),
		}
	}
	r.Lock()
	r.startTime = time.Now()
	r.stopped = false
	r.done = make(chan struct{})
	r.next = next
	r.Unlock()
	sess, err := session.NewSession()
	if err != nil {
		return err
	}
	var creds *credentials.Credentials
	if r.config.AWSRoleARN != "" {
		creds = stscreds.NewCredentials(sess, r.config.AWSRoleARN)
	} else if r.config.AWSAccessKeyId != "" && r.config.AWSSecretAccessKey != "" {
		creds = credentials.NewStaticCredentials(r.secrets.Secret(r.config.AWSAccessKeyId), r.secrets.Secret(r.config.AWSSecretAccessKey), "")
	} else {
		creds = sess.Config.Credentials
	}
	tr := &http.Transport{
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
	}
	err = http2.ConfigureTransport(tr)
	if err != nil {
		return &pkgplugin.Error{Err: err}
	}
	httpClient := &http.Client{
		Transport: tr,
	}
	sess, err = session.NewSession(&aws.Config{Region: aws.String(r.config.AWSRegion), Credentials: creds, HTTPClient: httpClient})
	if nil != err {
		return &KinesisError{op: "NewSession", err: err}
	}
	_, err = sess.Config.Credentials.Get()
	if nil != err {
		return &KinesisError{op: "GetCredentials", err: err}
	}
	r.svc = kinesis.New(sess)
	//r.svc = kinesis.New(sess, aws.NewConfig().WithLogLevel(aws.LogDebug))
	r.stream, err = r.svc.DescribeStream(&kinesis.DescribeStreamInput{StreamName: aws.String(r.config.StreamName)})
	if err != nil {
		return &KinesisError{op: "DescribeStream", err: err}
	}
	if *r.config.EnhancedFanOut {
		r.consumer, err = r.registerStreamConsumer(r.svc, r.config.StreamName, r.config.ConsumerName)
		if err != nil {
			return &KinesisError{op: "registerStreamConsumer", err: err}
		}
	}
	sharderConfig := sharder.DefaultControllerConfig()
	r.shardDistributor, err = sharder.GetDefaultHashDistributor(sharderConfig.NodeName, len(r.stream.StreamDescription.Shards), sharderConfig.StorageConfig)
	if err != nil {
		return &KinesisError{op: "GetDefaultHashDistributor", err: err}
	}
	r.shardUpdateListener(r.shardDistributor)
	r.shardMonitor(r.svc, r.shardDistributor)
	r.logger.Info().Str("op", "Kinesis.Receive").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Msg("waiting for receive done")
	<-r.done
	r.Lock()
	elapsedMs := time.Since(r.startTime).Milliseconds()
	receiveThroughput := 1000 * r.receiveCount / (int(elapsedMs) + 1)
	deleteThroughput := 1000 * r.deleteCount / (int(elapsedMs) + 1)
	receiveCnt := r.receiveCount
	deleteCnt := r.deleteCount
	r.Unlock()
	r.logger.Info().Str("op", "Kinesis.Receive").Str("stream", *r.stream.StreamDescription.StreamName).Str("name", r.Name()).Str("tid", r.Tenant().ToString()).Int("elapsedMs", int(elapsedMs)).Int("deleteCount", deleteCnt).Int("receiveCount", receiveCnt).Int("receiveThroughput", receiveThroughput).Int("deleteThroughput", deleteThroughput).Msg("receive done")
	return nil
}

func (r *Receiver) Count() int {
	r.Lock()
	defer r.Unlock()
	return r.receiveCount
}

func (r *Receiver) StopReceiving(ctx context.Context) error {
	r.Lock()
	stopped := r.stopped
	r.stopped = true
	r.Unlock()
	if !stopped {
		// check if there is a shard monitor to stop
		cleanupShardMonitor := false
		select {
		case r.shardMonitorStopChannel <- true:
			cleanupShardMonitor = true
		case <-time.After(errorTimeoutSecLong * time.Second):
			cleanupShardMonitor = false
		}
		if cleanupShardMonitor {
			log.Ctx(ctx).Info().Str("op", "StopReceiving").Str("name", r.name).Msg("cleaning up shard monitor and distributor")
			r.shardUpdateListenerStopChannel <- true
			r.stopShardReceiver(-1)
			r.shardDistributor.Stop()
		}
		r.eventSuccessCounter.Unbind()
		r.eventFailureCounter.Unbind()
		r.eventBytesCounter.Unbind()
		r.Lock()
		close(r.done)
		r.Unlock()
	}
	return nil
}

func (r *Receiver) Trigger(e event.Event) {
	r.Lock()
	next := r.next
	r.Unlock()
	if next != nil {
		next(e)
	}
}

func (r *Receiver) Config() interface{} {
	return r.config
}

func (r *Receiver) Name() string {
	return r.name
}

func (r *Receiver) Plugin() string {
	return r.plugin
}

func (r *Receiver) Tenant() tenant.Id {
	return r.tid
}
