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
	"os"
	"path"
	"testing"

	"github.com/go-logr/zapr"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zaptest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestNewConfig(t *testing.T) {

	g := NewWithT(t)
	logger := zapr.NewLogger(zaptest.NewLogger(t))

	// Given
	kconfig := *clientcmdapi.NewConfig()
	kconfig.Clusters["test-cluster"] = &clientcmdapi.Cluster{
		Server: "http://testing",
	}
	kconfig.Contexts["test"] = &clientcmdapi.Context{
		Cluster: "test-cluster",
	}
	kconfig.CurrentContext = "test"

	kubeconfig := path.Join(t.TempDir(), "kubeconfig")
	g.Expect(clientcmd.WriteToFile(kconfig, kubeconfig)).To(Succeed())

	cmd := &cobra.Command{
		Use: "konfirm",
	}
	RegisterFlags(cmd.PersistentFlags())
	g.Expect(cmd.ParseFlags([]string{"--kubeconfig", kubeconfig})).To(Succeed())

	// When
	var config *Config
	var err error
	config, err = NewConfig(cmd, logger)

	// Then
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(config.HealthProbeBindAddress).To(Equal(":8081"))
	g.Expect(config.MetricsBindAddress).To(Equal(":8080"))
	g.Expect(config.LeaderElectID).To(BeEmpty())
	g.Expect(config.LeaderElectEnabled).To(BeFalse())
	pwd, err := os.Getwd()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(config.HelmDir).To(Equal(pwd))
	g.Expect(config.RestConfig.Host).To(Equal(kconfig.Clusters["test-cluster"].Server))
}
