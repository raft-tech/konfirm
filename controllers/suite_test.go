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

package controllers_test

import (
	"context"
	"errors"
	"github.com/onsi/ginkgo/config"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raft-tech/konfirm/controllers"
	"github.com/raft-tech/konfirm/logging"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	konfirmv1alpha1 "github.com/raft-tech/konfirm/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	mgrCtx    context.Context
	mgrCancel context.CancelFunc
	trand     *rand.Rand
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	origConifg := config.DefaultReporterConfig
	if v := os.Getenv("GINGKO_VERBOSE"); v != "" {
		config.DefaultReporterConfig.Verbose = true
	}
	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
	config.DefaultReporterConfig = origConifg
}

var _ = BeforeSuite(func() {

	logf.SetLogger(zap.New(
		zap.WriteTo(GinkgoWriter),
		zap.UseDevMode(true),
		zap.Level(zapcore.DebugLevel-2),
		logging.EncoderLevel(logging.CapitalLevelEncoder),
		func(o *zap.Options) {
			o.EncoderConfigOptions = append(o.EncoderConfigOptions, func(config *zapcore.EncoderConfig) {
				config.EncodeTime = zapcore.RFC3339TimeEncoder
			})
		}))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = konfirmv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create a manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	// Set up TestController
	err = (&controllers.TestReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("test-controller"),
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	// Set up TestRunController
	err = (&controllers.TestRunReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("testrun-controller"),
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	// Set up TestSuiteController
	err = (&controllers.TestSuiteReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("testsuite-controller"),
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	// Start manager
	mgrCtx, mgrCancel = context.WithCancel(context.TODO())
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(mgrCtx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// Seeded randomness generator
	trand = rand.New(rand.NewSource(GinkgoRandomSeed()))

}, 60)

var _ = AfterSuite(func() {
	mgrCancel() // Stops mgr started in BeforeSuite
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var namespaceAlphabet = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

// generateNamespace generates a pseudorandom namespace name using trand
func generateNamespace() (string, error) {

	n := 6
	k := len(namespaceAlphabet)
	m := k * (int(math.Floor(256 / float64(k)))) // Random values >= m must be discarded for uniformity
	ns := &strings.Builder{}
	src := make([]byte, n)

	for pos := 0; pos < n; {
		src = src[:n-pos]
		if _, e := trand.Read(src); e != nil {
			return "", errors.New("error reading from random source") // Never return a partial ID
		}
		for i := 0; i < len(src); i++ {
			j := int(src[i])
			if j < m {
				ns.WriteRune(namespaceAlphabet[j%k])
				pos++
			}
		}
	}

	return ns.String(), nil
}
