/*
 Copyright 2023 Raft, LLC

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

package setup_test

import (
	"errors"
	"math"
	"math/rand"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/konfirm/logging"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	random    *rand.Rand
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helm Suite")
}

var _ = BeforeSuite(func() {

	logf.SetLogger(zap.New(
		zap.WriteTo(GinkgoWriter),
		zap.UseDevMode(true),
		zap.Level(zapcore.DebugLevel-2),
		logging.EncoderLevel(logging.CapitalLevelEncoder),
		func(o *zap.Options) {
			o.EncoderConfigOptions = append(o.EncoderConfigOptions, func(config *zapcore.EncoderConfig) {
				config.EncodeTime = zapcore.ISO8601TimeEncoder
			})
		}))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	random = rand.New(rand.NewSource(GinkgoRandomSeed()))
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var namespaceAlphabet = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

// generateNamespace generates a pseudorandom namespace name using random
func generateNamespace() (string, error) {

	n := 6
	k := len(namespaceAlphabet)
	m := k * (int(math.Floor(256 / float64(k)))) // Random values >= m must be discarded for uniformity
	ns := &strings.Builder{}
	src := make([]byte, n)

	for pos := 0; pos < n; {
		src = src[:n-pos]
		if _, e := random.Read(src); e != nil {
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
