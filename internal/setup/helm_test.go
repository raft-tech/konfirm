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
	"context"
	"net"
	"net/http"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raft-tech/konfirm/internal/setup"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Helm Client", func() {

	var namespace string
	BeforeEach(func(ctx context.Context) {
		if n, e := generateNamespace(); e == nil {
			namespace = n
		} else {
			Expect(e).NotTo(HaveOccurred())
		}
		ns := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	Context("with client", func() {

		var helm setup.HelmClient

		BeforeEach(func(ctx context.Context) {
			var err error
			helm, err = setup.NewHelmClient(setup.HelmClientOptions{
				ReleaseNamespace: namespace,
				RESTMapper:       k8sClient.RESTMapper(),
				RESTConfig:       cfg,
				WorkDir:          GinkgoT().TempDir(),
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with HTTP repository", func() {

			var repository http.Server
			var repositoryAddr string
			var repositoryErr error
			var repositoryWait sync.WaitGroup
			BeforeEach(func() {
				repository = http.Server{
					Handler: http.FileServer(http.Dir("testdata")),
				}
				wg := sync.WaitGroup{}
				wg.Add(1)
				go func() {
					if l, err := net.Listen("tcp", "localhost:0"); err == nil {
						repositoryAddr = "http://" + l.Addr().String()
						wg.Done()
						repositoryWait.Add(1)
						repositoryErr = repository.Serve(l)
						repositoryWait.Done()
					} else {
						repositoryErr = err
					}
				}()
				wg.Wait()
				Expect(repositoryErr).NotTo(HaveOccurred())
			})

			AfterEach(func(ctx context.Context) {
				Expect(repository.Shutdown(ctx)).To(Succeed())
			})

			It("installs releases", func(ctx SpecContext) {

				By("By running a Helm install")
				rel, err := helm.Install(ctx, setup.HelmReleaseParams{
					ReleaseName: "simple-test",
					Values: map[string]any{
						"config": map[string]string{
							"spec": ctx.SpecReport().LeafNodeText,
						},
					},
					Chart: setup.ChartRef{
						URL: repositoryAddr + "/test-0.1.0.tgz",
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(rel.Name).To(Equal("simple-test"))
				Expect(rel.Namespace).To(Equal(namespace))
				Expect(rel.Info.Status).To(Equal(release.StatusDeployed))

				actual := v1.ConfigMap{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: namespace,
					Name:      "simple-test",
				}, &actual)).To(Succeed())

				Expect(actual.Data).To(HaveKeyWithValue("spec", ctx.SpecReport().LeafNodeText))
			})
		})
	})
})
