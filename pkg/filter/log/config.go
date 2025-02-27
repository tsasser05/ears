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
	"github.com/xmidt-org/ears/pkg/config"
	pkgconfig "github.com/xmidt-org/ears/pkg/config"
	"github.com/xmidt-org/ears/pkg/errs"
	"github.com/xmidt-org/ears/pkg/filter"
)

var _ config.Exporter = (*Config)(nil)
var _ config.Importer = (*Config)(nil)

func NewConfig(config interface{}) (*Config, error) {
	var cfg Config
	err := pkgconfig.NewConfig(config, &cfg)
	if err != nil {
		return nil, &filter.InvalidConfigError{
			Err: err,
		}
	}
	return &cfg, nil
}

// WithDefaults will set default values
func (c Config) WithDefaults() *Config {
	cfg := c
	if c.Path == "" {
		cfg.Path = DefaultConfig.Path
	}
	if c.Tag == "" {
		cfg.Tag = DefaultConfig.Tag
	}
	if c.AsString == nil {
		cfg.AsString = DefaultConfig.AsString
	}
	return &cfg
}

func (c *Config) String() string {
	s, err := c.YAML()
	if err != nil {
		return errs.String("error", nil, err)
	}
	return s
}

func (c *Config) YAML() (string, error) {
	return config.ToYAML(c)
}

func (c *Config) FromYAML(in string) error {
	return config.FromYAML(in, c)
}

func (c *Config) JSON() (string, error) {
	return config.ToJSON(c)
}

func (c *Config) FromJSON(in string) error {
	return config.FromJSON(in, c)
}

func (c *Config) Validate() error {
	return nil
}
