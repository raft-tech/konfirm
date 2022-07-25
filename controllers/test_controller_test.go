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
	"github.com/raft-tech/konfirm/controllers"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Test Controller", func() {

	const (
		timeout = "500ms"
	)

	var (
		ctx  context.Context
		test *konfirmv1alpha1.Test
	)

	// Helper function for retrieving a Test's pods
	var getPods = func() ([]v1.Pod, error) {
		var pods []v1.Pod
		var err error
		var podList v1.PodList
		if err = k8sClient.List(ctx, &podList, client.InNamespace(test.Namespace)); err == nil {
			for _, p := range podList.Items {
				for _, o := range p.GetOwnerReferences() {
					if o.APIVersion == konfirmv1alpha1.GroupVersion.String() &&
						o.Kind == "Test" &&
						o.Name == test.Name {
						pods = append(pods, p)
					}
				}
			}
		}
		return pods, err
	}

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
								return o.Controller != nil &&
									*o.Controller &&
									*o.BlockOwnerDeletion &&
									p.Spec.RestartPolicy == v1.RestartPolicyNever
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

	Context("a Test was created", func() {

		// Create the Test and, optionally, set the associated Pod's phase
		var podPhase = v1.PodPending
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, test)).NotTo(HaveOccurred())
			if podPhase != v1.PodPending {
				Eventually(func() bool {
					ok := false
					if pods, err := getPods(); err == nil && len(pods) == 1 {
						pod := &pods[0]
						orig := pod.DeepCopy()
						pod.Status.Phase = podPhase
						ok = k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig)) == nil
					}
					return ok
				}, timeout).Should(BeTrue())
			}
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, test)).NotTo(HaveOccurred())
		})

		It("should reach the Starting phase", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
				g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestStarting), "test did not reach the Starting phase")
				g.Expect(test.Status.Conditions).To(And(
					HaveValue(And(
						HaveField("Type", controllers.PodCreatedCondition),
						HaveField("Status", "True"),
						HaveField("Reason", "PodCreated"),
					)),
					HaveValue(And(
						HaveField("Type", controllers.TestCompletedCondition),
						HaveField("Status", "False"),
						HaveField("Reason", "PodNotCompleted"),
					)),
				), "have the expected conditions")
			}, timeout)
		})

		When("a Test's pod is Running", func() {

			BeforeEach(func() {
				podPhase = v1.PodRunning
			})

			It("should reach the Running phase", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
					g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestRunning), "test did not reach the Running phase")
					g.Expect(test.Status.Conditions).To(And(
						HaveValue(And(
							HaveField("Type", controllers.PodCreatedCondition),
							HaveField("Status", "True"),
							HaveField("Reason", "PodCreated"),
						)),
						HaveValue(And(
							HaveField("Type", controllers.TestCompletedCondition),
							HaveField("Status", "False"),
							HaveField("Reason", "PodNotCompleted"),
						)),
					), "have the expected conditions")
				}, timeout)
			})
		})

		Context("container statuses are set", func() {

			var (
				containerStatuses = []v1.ContainerStatus{
					{
						Name: "main",
						State: v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode: 1,
								Message:  "Of all the things I've lost I miss my mind the most. --Ozzy Osbourne",
							},
						},
					},
				}
			)

			JustBeforeEach(func() {
				Eventually(func() bool {
					ok := false
					if pods, err := getPods(); err == nil && len(pods) == 1 {
						pod := &pods[0]
						orig := pod.DeepCopy()
						pod.Status.ContainerStatuses = containerStatuses
						ok = k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig)) == nil
					}
					return ok
				}).Should(BeTrue())
			})

			When("a Test's pod Succeeded", func() {

				BeforeEach(func() {
					podPhase = v1.PodSucceeded
				})

				It("it should progress the Test to Passed", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
						g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestPassed), "test did not reach the Passed phase")
						g.Expect(test.Status.Messages).To(Equal(map[string]string{
							"main": "Of all the things I've lost I miss my mind the most. --Ozzy Osbourne",
						}))
						g.Expect(test.Status.Conditions).To(And(
							HaveValue(And(
								HaveField("Type", controllers.PodCreatedCondition),
								HaveField("Status", "True"),
								HaveField("Reason", "PodCreated"),
							)),
							HaveValue(And(
								HaveField("Type", controllers.TestCompletedCondition),
								HaveField("Status", "True"),
								HaveField("Reason", "PodCompleted"),
							)),
						), "have the expected conditions")
					}, timeout)
				})

				It("it should delete the pods", func() {
					Eventually(getPods).Should(BeEmpty())
				})

				When("retention policy is Never", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirmv1alpha1.RetainNever
					})

					It("it should delete the pods", func() {
						Eventually(getPods).Should(BeEmpty())
					})
				})

				When("retention policy is Always", func() {
					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirmv1alpha1.RetainNever
					})

					It("it should retain the pod", func() {
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
							g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestPassed), "test did not reach the Passed phase")
							g.Expect(getPods()).To(HaveLen(1))
						}, timeout)
					})
				})
			})

			When("a Test's pod Failed", func() {

				BeforeEach(func() {
					podPhase = v1.PodFailed
				})

				It("it should progress the Test to Failed", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
						g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestFailed), "test did not reach the Failed phase")
						g.Expect(test.Status.Messages).To(Equal(map[string]string{
							"main": "Of all the things I've lost I miss my mind the most. --Ozzy Osbourne",
						}))
						g.Expect(test.Status.Conditions).To(And(
							HaveValue(And(
								HaveField("Type", controllers.PodCreatedCondition),
								HaveField("Status", "True"),
								HaveField("Reason", "PodCreated"),
							)),
							HaveValue(And(
								HaveField("Type", controllers.TestCompletedCondition),
								HaveField("Status", "True"),
								HaveField("Reason", "PodCompleted"),
							)),
						), "have the expected conditions")
					}, timeout)
				})

				It("it should retain the pod", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
						g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestFailed), "test did not reach the Failed phase")
						g.Expect(getPods()).To(HaveLen(1))
					}, timeout)
				})

				When("retention policy is Never", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirmv1alpha1.RetainNever
					})

					It("it should delete the pods", func() {
						Eventually(getPods).Should(BeEmpty())
					})
				})

				When("retention policy is Always", func() {
					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirmv1alpha1.RetainNever
					})

					It("it should retain the pod", func() {
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)).ToNot(HaveOccurred())
							g.Expect(test.Status.Phase).To(Equal(konfirmv1alpha1.TestPassed), "test did not reach the Passed phase")
							g.Expect(getPods()).To(HaveLen(1))
						}, timeout)
					})
				})
			})
		})
	})
})
