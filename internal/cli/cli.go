/*
 Copyright 2024 Raft, LLC

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cli

import (
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	"github.com/raft-tech/konfirm/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	HealthProbeBindAddressFlag = "health-probe-bind-address"
	MetricsBindAddressFlag     = "metrics-bind-address"
	LeaderElectIDFlag          = "leader-elect"
	HelmDirFlag                = "helm-dir"
	KubeConfigFlag             = clientcmd.RecommendedConfigPathFlag
	DebugFlag                  = "debug"
	WarnFlag                   = "warn"
	LoggingJsonFlag            = "json"
)

func RegisterFlags(pflags *pflag.FlagSet) {
	pflags.StringP("config", "c", "", "path to config file")
	pflags.String(HealthProbeBindAddressFlag, ":8081", "address the probe endpoint binds to")
	pflags.String(MetricsBindAddressFlag, ":8080", "address the metric endpoint binds to")
	pflags.String(LeaderElectIDFlag, "", "enable leader election with the specified ID")
	pflags.String(HelmDirFlag, "", "Helm data directory")
	pflags.String(KubeConfigFlag, "", "path to kubeconfig file")
	pflags.Bool(DebugFlag, false, "enable debug logging")
	pflags.Bool(WarnFlag, false, "restrict logging to warnings and errors")
	pflags.Bool(LoggingJsonFlag, false, "encode log messages in JSON")
}

type Config struct {
	HealthProbeBindAddress string
	HelmDir                string
	LeaderElectEnabled     bool
	LeaderElectID          string
	Logger                 *zap.Logger
	MetricsBindAddress     string
	RestConfig             *rest.Config
}

func NewConfig(cmd *cobra.Command, initLog logr.Logger) (c *Config, err error) {

	cfg := viper.NewWithOptions(viper.WithLogger(slog.New(logr.ToSlogHandler(initLog))))
	cfg.SetEnvPrefix(cmd.Name())
	cfg.AutomaticEnv()
	_ = cfg.BindPFlags(cmd.Flags())
	if f, _ := cmd.Flags().GetString("config"); f != "" {
		cfg.SetConfigFile(f)
		if e := cfg.ReadInConfig(); e != nil {
			err = Errorf(2, "invalid config file: %s", f)
			return
		}
	}

	// Set up the config object
	c = new(Config)
	c.HealthProbeBindAddress = cfg.GetString(HealthProbeBindAddressFlag)
	c.MetricsBindAddress = cfg.GetString(MetricsBindAddressFlag)
	c.LeaderElectID = cfg.GetString(LeaderElectIDFlag)
	c.LeaderElectEnabled = c.LeaderElectID != ""
	c.HelmDir = cfg.GetString(HelmDirFlag)
	if c.HelmDir == "" {
		c.HelmDir = "."
		if cwd, e := os.Getwd(); e == nil {
			c.HelmDir = cwd
		}
	}

	// Logger
	lcfg := zap.NewProductionConfig()
	if cfg.GetBool(DebugFlag) {
		lcfg.Level.SetLevel(zapcore.DebugLevel)
	} else if cfg.GetBool(WarnFlag) {
		lcfg.Level.SetLevel(zapcore.WarnLevel)
	}
	if !cfg.GetBool(LoggingJsonFlag) {
		lcfg.Encoding = "console"
		lcfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
		lcfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	logging.RegisterSink(cmd.Name(), cmd.OutOrStdout())
	lcfg.OutputPaths = []string{"konfirm:///" + cmd.Name()}
	c.Logger, err = lcfg.Build()
	if err != nil {
		initLog.Error(err, "unable to configure logger")
		return nil, Errorf(2, "error configuring logger: %s", err.Error())
	}

	// REST config
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	loader.ExplicitPath = cfg.GetString(KubeConfigFlag)
	c.RestConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, nil).ClientConfig()
	if err != nil {
		initLog.Error(err, "unable to load kubeconfig")
		err = Errorf(2, "kubeconfig error: %s", err.Error())
	}

	return
}
