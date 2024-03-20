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

package cmd

import (
	"time"

	"github.com/go-logr/zapr"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	"github.com/raft-tech/konfirm/internal/cli"
	"github.com/raft-tech/konfirm/internal/impersonate"
	"github.com/raft-tech/konfirm/internal/logging"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	//+kubebuilder:scaffold:imports
)

var (
	setupLog = ctrl.Log.WithName("init")
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		RunE:          serve,
		SilenceErrors: true,
		SilenceUsage:  true,
		Use:           "konfirm",
	}
	cli.RegisterFlags(cmd.PersistentFlags())
	return cmd
}

func serve(cmd *cobra.Command, args []string) error {

	var err error
	var config *cli.Config
	config, err = cli.NewConfig(cmd, ctrl.Log)
	if err != nil {
		return err
	}

	ctrl.SetLogger(zapr.NewLogger(config.Logger))
	ctx := logging.IntoContext(cmd.Context(), config.Logger.Named("konfirm"))

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(konfirm.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	var mgr ctrl.Manager
	if m, err := ctrl.NewManager(config.RestConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: config.MetricsBindAddress,
		},
		HealthProbeBindAddress: config.HealthProbeBindAddress,
		LeaderElection:         config.LeaderElectEnabled,
		LeaderElectionID:       config.LeaderElectID,
		Logger:                 zapr.NewLogger(config.Logger).WithName("controller-runtime.manager"),
		NewClient:              impersonate.NewImpersonatingClient,
	}); err == nil {
		mgr = m
	} else {
		return cli.Error(1, "unable to start manager")
	}

	errMsg := "unable to create controller"
	if err = (&controllers.TestReconciler{
		Client:          mgr.GetClient().(impersonate.Client),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("konfirm"),
		ErrRequeueDelay: time.Minute,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errMsg, "controller", "Test")
		return err
	}
	if err = (&controllers.TestRunReconciler{
		Client:          mgr.GetClient().(impersonate.Client),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("konfirm"),
		ErrRequeueDelay: time.Minute,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errMsg, "controller", "TestRun")
		return err
	}
	if err = (&controllers.TestSuiteReconciler{
		Client:          mgr.GetClient().(impersonate.Client),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("konfirm"),
		ErrRequeueDelay: time.Minute,
		CronParser:      cron.ParseStandard,
		Clock:           clock.RealClock{},
		DataDir:         config.HelmDir,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errMsg, "controller", "TestSuite")
		return err
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}
