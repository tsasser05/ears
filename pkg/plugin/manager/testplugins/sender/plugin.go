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

package main

import (
	"context"
	pkgplugin "github.com/xmidt-org/ears/pkg/plugin"
	"github.com/xmidt-org/ears/pkg/secret"
	"github.com/xmidt-org/ears/pkg/tenant"

	"github.com/xmidt-org/ears/pkg/event"
	"github.com/xmidt-org/ears/pkg/sender"
)

func main() {
	// required for `go build` to not fail
}

var Plugin, PluginErr = NewPlugin()

// for golangci-lint
var _ = Plugin
var _ = PluginErr

var _ sender.Sender = (*plugin)(nil)

type plugin struct{}

const (
	Name     = "name"
	Version  = "version"
	CommitID = "commitID"
)

// =====================================================

func NewPlugin() (*pkgplugin.Plugin, error) {
	return NewPluginVersion(Name, Version, CommitID)
}

func NewPluginVersion(name string, version string, commitID string) (*pkgplugin.Plugin, error) {
	return pkgplugin.NewPlugin(
		pkgplugin.WithName(name),
		pkgplugin.WithVersion(version),
		pkgplugin.WithCommitID(commitID),
		pkgplugin.WithNewSender(NewSender),
	)
}

// Sender ===========================================================

func NewSender(tid tenant.Id, pluginType string, name string, config interface{}, secrets secret.Vault) (sender.Sender, error) {
	return &plugin{}, nil
}

func (p *plugin) Send(e event.Event) {
}

func (p *plugin) Unwrap() sender.Sender {
	return p
}

func (p *plugin) StopSending(ctx context.Context) {
}

func (p *plugin) Config() interface{} {
	return nil
}

func (p *plugin) Name() string {
	return ""
}

func (p *plugin) Plugin() string {
	return "plugin"
}

func (p *plugin) Tenant() tenant.Id {
	return tenant.Id{OrgId: "myorg", AppId: "myapp"}
}
