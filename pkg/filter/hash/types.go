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

package hash

import "github.com/xmidt-org/ears/pkg/tenant"

// Config can be passed into NewFilter() in order to configure
// the behavior of the sender.
type Config struct {
	FromPath      string `json:"fromPath,omitempty"`
	From          string `json:"from,omitempty"`
	ToPath        string `json:"toPath,omitempty"`
	HashAlgorithm string `json:"hashAlgorithm,omitempty"`
	Key           string `json:"key,omitempty"`      // optional key for certain hash algorithms
	Encoding      string `json:"encoding,omitempty"` // optional encoding of hash, base64, hex etc.
}

var DefaultConfig = Config{
	FromPath:      "",
	From:          "",
	ToPath:        "",
	HashAlgorithm: "md5",
	Key:           "",
	Encoding:      "",
}

type Filter struct {
	config Config
	name   string
	plugin string
	tid    tenant.Id
}
