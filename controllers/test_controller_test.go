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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	konfirmv1alpha1 "github.com/raft-tech/konfirm/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Test Controller", func() {

	const (
		timeout              = "100ms"
		ownerDeleteFinalizer = "konfirm.go-raft.tech/fake"
	)

	var (
		ctx  context.Context
		test *konfirmv1alpha1.Test
	)

	// Pretest set up
	BeforeEach(func() {
		ctx = context.Background()
		test = &konfirmv1alpha1.Test{
			TypeMeta: metav1.TypeMeta{
				APIVersion: konfirmv1alpha1.GroupVersion.String(),
				Kind:       "Test",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a-test",
				Namespace: "default",
				// FIXME: Actually confirm this behavior exists in K8s
				Finalizers: []string{ownerDeleteFinalizer}, // Mock K8s' native blockOwnerDeletion behavior
			},
			Spec: konfirmv1alpha1.TestSpec{
				RetentionPolicy: konfirmv1alpha1.RetainOnFailure,
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "a-test-container",
								Image: "a-test-image",
							},
						},
					},
				},
			},
		}
	})

	AfterEach(func() {
		Expect(func() error { // Remove the fake finalizer, which mocks blockOwnerDeletion behavior
			orig := test.DeepCopy()
			test.Finalizers = []string{}
			for _, f := range orig.GetFinalizers() {
				if f != ownerDeleteFinalizer {
					test.Finalizers = append(test.Finalizers, f)
				}
			}
			return k8sClient.Patch(ctx, test, client.MergeFrom(orig))
		}()).NotTo(HaveOccurred())
	})

	// Pods and Tests should be cleaned up
	AfterEach(func() {

		// All pods are gone
		Eventually(func() ([]v1.Pod, error) {
			pods := v1.PodList{}
			return pods.Items, k8sClient.List(ctx, &pods)
		}, timeout).Should(BeEmpty(), "not all Pods were removed")

		// All tests are gone
		Eventually(func() ([]konfirmv1alpha1.Test, error) {
			tests := konfirmv1alpha1.TestList{}
			return tests.Items, k8sClient.List(ctx, &tests)
		}, timeout).Should(BeEmpty(), "not all Tests were removed")
	})

	When("a test is created or deleted", func() {
		It("an associated pod should be created/deleted", func() {

			var podKey client.ObjectKey

			// Create ...
			Expect(k8sClient.Create(ctx, test)).NotTo(HaveOccurred())
			Eventually(func() bool {
				pods := &v1.PodList{}
				if err := k8sClient.List(ctx, pods, &client.ListOptions{Namespace: test.Namespace}); err == nil {
					for _, p := range pods.Items {
						for _, o := range p.GetOwnerReferences() {
							if o.Kind == "Test" && o.Name == test.Name {
								podKey = client.ObjectKeyFromObject(&p)
								return o.Controller != nil && *o.Controller && *o.BlockOwnerDeletion
							}
						}
					}
				}
				return false
			}, timeout).Should(BeTrue())

			// Delete ...
			Expect(k8sClient.Delete(ctx, test)).NotTo(HaveOccurred())
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, podKey, &v1.Pod{}))
			}, timeout).Should(BeTrue())
		})
	})

	Context("a Test exists", func() {

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, test)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, test)).NotTo(HaveOccurred())
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), &konfirmv1alpha1.Test{}))
			}, timeout).Should(BeTrue())
		})
	})
})
