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

package split

import (
	"fmt"
	//"github.com/gohobby/deepcopy"
	"github.com/boriwo/deepcopy"
	"github.com/rs/zerolog/log"
	"github.com/xmidt-org/ears/pkg/event"
	"github.com/xmidt-org/ears/pkg/filter"
	"github.com/xmidt-org/ears/pkg/secret"
	"github.com/xmidt-org/ears/pkg/tenant"
	"go.opentelemetry.io/otel/trace"
)

// a SplitFilter splits and event into two or more events
func NewFilter(tid tenant.Id, plugin string, name string, config interface{}, secrets secret.Vault) (*Filter, error) {
	cfg, err := NewConfig(config)
	if err != nil {
		return nil, &filter.InvalidConfigError{
			Err: err,
		}
	}
	cfg = cfg.WithDefaults()
	err = cfg.Validate()
	if err != nil {
		return nil, err
	}
	f := &Filter{
		config: *cfg,
		name:   name,
		plugin: plugin,
		tid:    tid,
	}
	return f, nil
}

// Filter splits an event containing an array into multiple events
func (f *Filter) Filter(evt event.Event) []event.Event {
	if f == nil {
		evt.Nack(&filter.InvalidConfigError{
			Err: fmt.Errorf("<nil> pointer filter")})
		return nil
	}
	events := []event.Event{}
	obj, _, _ := evt.GetPathValue(f.config.Path)
	if obj == nil {
		log.Ctx(evt.Context()).Error().Str("op", "filter").Str("filterType", "split").Str("name", f.Name()).Msg("nil object at " + f.config.Path)
		if span := trace.SpanFromContext(evt.Context()); span != nil {
			span.AddEvent("nil object at " + f.config.Path)
		}
		evt.Ack()
		return []event.Event{}
	}
	arr, ok := obj.([]interface{})
	if !ok {
		log.Ctx(evt.Context()).Error().Str("op", "filter").Str("filterType", "split").Str("name", f.Name()).Msg("split on non array type at " + f.config.Path)
		if span := trace.SpanFromContext(evt.Context()); span != nil {
			span.AddEvent("split on non array type at " + f.config.Path)
		}
		evt.Ack()
		return []event.Event{}
	}
	for _, p := range arr {
		nevt, err := evt.Clone(evt.Context())
		if err != nil {
			log.Ctx(evt.Context()).Error().Str("op", "filter").Str("filterType", "split").Str("name", f.Name()).Msg(err.Error())
			if span := trace.SpanFromContext(evt.Context()); span != nil {
				span.AddEvent(err.Error())
			}
			evt.Ack()
			return []event.Event{}
		}
		// no deepcopy needed for payload because we set new payload at root level, we do need deepcopy for metadata though
		err = nevt.SetPayload(p)
		if err == nil {
			nevt.SetMetadata(deepcopy.DeepCopy(evt.Metadata()).(map[string]interface{}))
		}
		if err != nil {
			log.Ctx(evt.Context()).Error().Str("op", "filter").Str("filterType", "split").Str("name", f.Name()).Msg(err.Error())
			if span := trace.SpanFromContext(evt.Context()); span != nil {
				span.AddEvent(err.Error())
			}
			evt.Ack()
			return []event.Event{}
		}
		events = append(events, nevt)
	}
	evt.Ack()
	log.Ctx(evt.Context()).Debug().Str("op", "filter").Str("filterType", "split").Str("name", f.Name()).Int("eventCount", len(events)).Msg("split")
	return events
}

func (f *Filter) Config() interface{} {
	if f == nil {
		return Config{}
	}
	return f.config
}

func (f *Filter) Name() string {
	return f.name
}

func (f *Filter) Plugin() string {
	return f.plugin
}

func (f *Filter) Tenant() tenant.Id {
	return f.tid
}
