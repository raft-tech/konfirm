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

package cmd_test

import (
	"bytes"
	"context"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/konfirm/cmd"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	zapc "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCLI(t *testing.T) {
	RegisterFailHandler(Fail)
	suite, reporter := GinkgoConfiguration()
	reporter.Verbose = true
	RunSpecs(t, "CLI", suite, reporter)
}

var (
	kubeconfig string
	logger     *zap.Logger
	testEnv    *envtest.Environment
	envLock    sync.Mutex
)

var _ = BeforeSuite(func() {
	By("configuring a logger")
	logger = zapc.NewRaw(
		zapc.UseDevMode(true),
		zapc.ConsoleEncoder(func(enc *zapcore.EncoderConfig) {
			enc.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}),
		zapc.WriteTo(GinkgoWriter),
	)
	ctrl.SetLogger(zapr.NewLogger(logger))
	logger = logger.Named("test-suite")
})

var _ = AfterSuite(func() {
	if et := testEnv; et != nil {
		By("shutting down test environment")
		Expect(et.Stop()).To(Succeed())
	}
})

var _ = Describe("CLI", func() {

	var app *cobra.Command
	BeforeEach(func() {
		app = cmd.New()
		app.Version = "testing"
	})

	It("prints help message", func(ctx context.Context) {
		var out bytes.Buffer
		app.SetOut(&out)
		app.SetErr(&out)
		app.SetArgs([]string{"--help"})
		Expect(app.ExecuteContext(ctx)).To(Succeed())
		Expect(out.String()).To(HavePrefix("Usage:\n  konfirm [flags]"))
	})

	It("prints the version", func(ctx context.Context) {
		var out bytes.Buffer
		app.SetOut(&out)
		app.SetErr(&out)
		app.SetArgs([]string{"--version"})
		Expect(app.ExecuteContext(ctx)).To(Succeed())
		Expect(out.String()).To(Equal("konfirm version testing\n"))
	})

	Context("with API server", func() {

		BeforeEach(func() {

			// Only set up envtest once, and only if a spec requires it
			envLock.Lock()
			defer envLock.Unlock()
			if testEnv != nil {
				return
			}

			By("creating a test environment")
			testEnv = &envtest.Environment{
				CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
				ErrorIfCRDPathMissing: true,
			}
			ts := time.Now()
			client, err := testEnv.Start()
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
			logger.Info("started test environment", zap.Duration("duration", time.Now().Sub(ts)))

			By("generating a kubeconfig file")
			kubeconfig = path.Join(GinkgoT().TempDir(), "kubeconfig")
			kconfig := *clientcmdapi.NewConfig()
			key := "envtest"
			kconfig.Clusters[key] = &clientcmdapi.Cluster{
				Server:                   client.Host,
				CertificateAuthorityData: client.CAData,
			}
			kconfig.AuthInfos[key] = &clientcmdapi.AuthInfo{
				ClientCertificateData: client.CertData,
				ClientKeyData:         client.KeyData,
			}
			kconfig.Contexts[key] = &clientcmdapi.Context{
				Cluster:  key,
				AuthInfo: key,
			}
			kconfig.CurrentContext = key
			Expect(clientcmd.WriteToFile(kconfig, kubeconfig)).To(Succeed())
			logger.Info("wrote kubeconfig file", zap.String("path", kubeconfig))
		})

		It("starts", func(ctx context.Context) {

			app.SetArgs([]string{"--kubeconfig", kubeconfig})

			sctx, stop := context.WithCancel(ctx)
			defer stop()
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				By("executing the konfirm command")
				Expect(app.ExecuteContext(sctx)).To(Succeed())
			}()

			time.Sleep(time.Second)
			stop()
			wg.Wait()
		})
	})
})
