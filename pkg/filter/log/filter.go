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

package log

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/xmidt-org/ears/pkg/event"
	"github.com/xmidt-org/ears/pkg/filter"
	"github.com/xmidt-org/ears/pkg/secret"
	"github.com/xmidt-org/ears/pkg/tenant"
)

var _ filter.Filterer = (*Filter)(nil)

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

// Filter log event and pass it on
func (f *Filter) Filter(evt event.Event) []event.Event {
	if f == nil {
		evt.Nack(&filter.InvalidConfigError{
			Err: fmt.Errorf("<nil> pointer filter"),
		})
		return nil
	}
	var obj interface{}
	if f.config.Path == "" {
		m := map[string]interface{}{}
		pl, _, _ := evt.GetPathValue("payload")
		md, _, _ := evt.GetPathValue("metadata")
		m["payload"] = pl
		m["metadata"] = md
		obj = m
	} else {
		obj, _, _ = evt.GetPathValue(f.config.Path)
	}
	buf, err := json.Marshal(obj)
	if err != nil {
		log.Ctx(evt.Context()).Error().Str("op", "filter").Str("filterType", "log").Str("name", f.Name()).Msg(err.Error())
	} else {
		if *f.config.AsString {
			log.Ctx(evt.Context()).Info().Str("op", "filter").Str("filterType", "log").Str("tag", f.config.Tag).Str("name", f.Name()).Str("payload", string(buf)).Msg("log")
		} else {
			log.Ctx(evt.Context()).Info().Str("op", "filter").Str("filterType", "log").Str("tag", f.config.Tag).Str("name", f.Name()).RawJSON("payload", buf).Msg("log")
		}
	}
	return []event.Event{evt}
}

func (f *Filter) Config() interface{} {
	if f == nil {
		return Config{}
	}
	return nil
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
