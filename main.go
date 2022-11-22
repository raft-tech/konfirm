/*
 Copyright 2022 Raft, LLC

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

package main

import (
	"flag"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/utils/clock"

	"github.com/raft-tech/konfirm/logging"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(konfirm.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {

	var err error

	// Logging flags
	opts := zap.Options{
		EncoderConfigOptions: []zap.EncoderConfigOption{
			logging.EncoderLevelConfig(logging.LowercaseLevelEncoder),
		},
	}
	opts.BindFlags(flag.CommandLine)

	// Manager flags
	config := konfirm.Config{}
	options := ctrl.Options{
		Scheme: scheme,
	}
	flag.StringVar(&options.MetricsBindAddress, "listen-metrics", "",
		"The address the metric endpoint binds to.")
	flag.StringVar(&options.HealthProbeBindAddress, "listen-healthz", "",
		"The address the health-probe endpoint binds to.")
	flag.BoolVar(&options.LeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will "+
			"ensure there is only one active controller manager.")

	// Config file
	var configFile string
	flag.StringVar(&configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. Command-line "+
			"flags override configuration from this file.")

	// Parse flags
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Load config file if set
	if configFile != "" {
		options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile).OfKind(&config))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
	}

	// Predicates for ignored and watched namespaces
	var predicates []predicate.Predicate
	if len(config.IgnoredNamespaces) > 0 {
		predicates = append(predicates, &controllers.NamespaceIsNotIgnored{IgnoredNamespaces: config.IgnoredNamespaces})
	}
	if len(config.WatchedNamespaces) > 0 {
		predicates = append(predicates, &controllers.NamespaceIsWatched{WatchedNamespaces: config.WatchedNamespaces})
	}

	// Create the manager
	ctx := ctrl.SetupSignalHandler()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	recorder := mgr.GetEventRecorderFor("konfirm")
	if err = (&controllers.TestReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        recorder,
		ErrRequeueDelay: time.Minute,
		Predicates:      predicates,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Test")
		os.Exit(1)
	}
	if err = (&controllers.TestRunReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        recorder,
		ErrRequeueDelay: time.Minute,
		Predicates:      predicates,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TestRun")
		os.Exit(1)
	}
	if err = (&controllers.TestSuiteReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        recorder,
		ErrRequeueDelay: time.Minute,
		Predicates:      predicates,
		CronParser:      cron.ParseStandard,
		Clock:           clock.RealClock{},
	}).SetupWithManager(mgr, ctx); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TestSuite")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err = mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
