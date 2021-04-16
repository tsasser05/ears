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
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xmidt-org/ears/internal/pkg/app"
	"github.com/xmidt-org/ears/internal/pkg/config"
	"github.com/xmidt-org/ears/internal/pkg/fx/pluginmanagerfx"
	"github.com/xmidt-org/ears/internal/pkg/fx/routestorerfx"
	"github.com/xmidt-org/ears/internal/pkg/fx/routetablesyncerfx"
	"github.com/xmidt-org/ears/internal/pkg/tablemgr"
	"github.com/xmidt-org/ears/pkg/cli"
	"github.com/xmidt-org/ears/pkg/panics"
	"go.uber.org/fx"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Runs the EARS microservice",
	Long:  `Runs the EARS microservice`,
	Run: func(cmd *cobra.Command, args []string) {

		defer func() {
			p := recover()
			if p != nil {
				panicErr := panics.ToError(p)
				log.Logger.Fatal().Str("op", "run").Str("error", panicErr.Error()).
					Str("stackTrace", panicErr.StackTrace()).Msg("A panic has occurred during startup")
			}
		}()

		logger, err := app.ProvideLogger(AppConfig())
		if err != nil {
			log.Logger.Fatal().Str("op", "InitLogger").Str("error", "panic").
				Msg("Error initialize logger")
		}
		earsApp := fx.New(
			pluginmanagerfx.Module,
			routestorerfx.Module,
			routetablesyncerfx.Module,
			fx.Provide(
				AppConfig,
				app.ProvideLogger,
				tablemgr.NewRoutingTableManager,
				app.NewAPIManager,
				app.NewMiddleware,
				app.NewMux,
			),
			fx.Logger(logger),
			fx.Invoke(app.SetupAPIServer),
			fx.Invoke(tablemgr.SetupRoutingManager),
		)
		earsApp.Run()
	},
}

func AppConfig() config.Config {
	return viper.GetViper()
}

func init() {
	rootCmd.AddCommand(runCmd)
	cli.ViperAddArguments(
		runCmd,
		[]cli.Argument{
			cli.Argument{
				Name: "logLevel", Shorthand: "", Type: cli.ArgTypeString,
				Default: "info", LookupKey: "ears.logLevel",
				Description: "log level",
			},
			cli.Argument{
				Name: "port", Shorthand: "", Type: cli.ArgTypeInt,
				Default: 8080, LookupKey: "ears.api.port",
				Description: "API port",
			},
			cli.Argument{
				Name: "storageType", Shorthand: "", Type: cli.ArgTypeString,
				Default: "inmemory", LookupKey: "ears.storage.type",
				Description: "persistence layer storage type (inmemory, dynamodb)",
			},
			cli.Argument{
				Name: "storageDynamoRegion", Shorthand: "", Type: cli.ArgTypeString,
				Default: "us-west-2", LookupKey: "ears.storage.region",
				Description: "dynamodb region",
			},
			cli.Argument{
				Name: "storageDynamoTable", Shorthand: "", Type: cli.ArgTypeString,
				Default: "gears.dev.ears", LookupKey: "ears.storage.table",
				Description: "dynamodb table name",
			},
			cli.Argument{
				Name: "redisEndpoint", Shorthand: "", Type: cli.ArgTypeString,
				Default: "gears-redis-qa-001.6bteey.0001.usw2.cache.amazonaws.com:6379", LookupKey: "ears.synchronization.endpoint",
				Description: "redis endpoint for routing table synchronization",
			},
		},
	)
}
