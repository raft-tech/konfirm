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
	"time"

	"github.com/raft-tech/konfirm/logging"
	"github.com/robfig/cron/v3"
	"k8s.io/utils/clock"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	konfirmv1alpha1 "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	"github.com/raft-tech/konfirm/internal/impersonate"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(konfirmv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {

	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")

	var enableLeaderElection bool
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	var helmDir string
	flag.StringVar(&helmDir, "helm-dir", "", "Helm data directory. Defaults to current working dir.")

	var probeAddr string
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	opts := zap.Options{
		EncoderConfigOptions: []zap.EncoderConfigOption{
			logging.EncoderLevelConfig(logging.LowercaseLevelEncoder),
		},
	}
	opts.BindFlags(flag.CommandLine)

	flag.Parse()

	if helmDir == "" {
		helmDir = "."
		if d, e := os.Getwd(); e == nil {
			helmDir = d
		}
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1337e21f.goraft.tech",
		NewClient:              impersonate.NewImpersonatingClient,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	recorder := mgr.GetEventRecorderFor("konfirm")
	errMsg := "unable to create controller"
	if err = (&controllers.TestReconciler{
		Client:          mgr.GetClient().(impersonate.Client),
		Scheme:          mgr.GetScheme(),
		Recorder:        recorder,
		ErrRequeueDelay: time.Minute,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errMsg, "controller", "Test")
		os.Exit(1)
	}
	if err = (&controllers.TestRunReconciler{
		Client:          mgr.GetClient().(impersonate.Client),
		Scheme:          mgr.GetScheme(),
		Recorder:        recorder,
		ErrRequeueDelay: time.Minute,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errMsg, "controller", "TestRun")
		os.Exit(1)
	}
	if err = (&controllers.TestSuiteReconciler{
		Client:          mgr.GetClient().(impersonate.Client),
		Scheme:          mgr.GetScheme(),
		Recorder:        recorder,
		ErrRequeueDelay: time.Minute,
		CronParser:      cron.ParseStandard,
		Clock:           clock.RealClock{},
		DataDir:         helmDir,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errMsg, "controller", "TestSuite")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
