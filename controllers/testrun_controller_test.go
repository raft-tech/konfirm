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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TestRun Controller", func() {

	const timeout = "500ms"

	var ctx context.Context
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		if ns, err := generateNamespace(); err == nil {
			namespace = ns
			Expect(k8sClient.Create(ctx, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			})).NotTo(HaveOccurred())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	When("a TestRun is created", func() {

		var testRun *konfirm.TestRun

		BeforeEach(func() {
			testRun = &konfirm.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a-testrun",
					Namespace: namespace,
				},
				Spec: konfirm.TestRunSpec{
					Tests: []konfirm.TestTemplate{
						{
							Description: "test",
							Template: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:  "test",
											Image: "konfirm/mock:v1",
										},
									},
								},
							},
						},
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, testRun)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, testRun)
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
		})

		It("it progresses to Running", func() {
			Eventually(func() (phase konfirm.TestRunPhase, err error) {
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun); err == nil {
					phase = testRun.Status.Phase
				}
				return
			}, timeout).Should(Equal(konfirm.TestRunRunning))
			Expect(testRun.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "RunStarted"),
				HaveField("Status", metav1.ConditionTrue),
				HaveField("Reason", "TestRunStarted"),
			)))
			Expect(testRun.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "RunCompleted"),
				HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "TestRunInProgress"),
			)))
		})

		When("the pod(s) succeed", func() {

			JustBeforeEach(func() {
				Eventually(func() (ok bool, err error) {
					var tests []konfirm.Test
					if tests, err = getTests(ctx, testRun); err != nil || len(tests) == 0 {
						return
					}
					for i := range tests {
						var pods []v1.Pod
						if pods, err = getPods(ctx, &tests[i]); err != nil || len(pods) == 0 {
							return
						}
						for j := range pods {
							if err = succeed(ctx, &pods[j]); err != nil {
								return
							}
						}
					}
					ok = true
					return
				}, timeout).Should(BeTrue())
			})

			It("it passes", func() {
				Eventually(func() (konfirm.TestRunPhase, error) {
					return testRun.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun)
				}, timeout).Should(Equal(konfirm.TestRunPassed))
				Expect(testRun.Status.Conditions).To(ContainElement(And(
					HaveField("Type", "RunStarted"),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", "TestRunStarted"),
				)))
				Expect(testRun.Status.Conditions).To(ContainElement(And(
					HaveField("Type", "RunCompleted"),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", "TestRunPassed"),
				)))
			})

			It("it deletes the tests", func() {
				Eventually(func() ([]konfirm.Test, error) {
					return getTests(ctx, testRun)
				}, timeout).Should(BeEmpty())
			})

			When("the retention policy is Always", func() {

				BeforeEach(func() {
					testRun.Spec.RetentionPolicy = konfirm.RetainAlways
				})

				It("it retains the test(s)", func() {
					Consistently(func() ([]konfirm.Test, error) {
						return getTests(ctx, testRun)
					}, timeout).Should(HaveLen(len(testRun.Spec.Tests)))
				})
			})

			When("the retention policy is Never", func() {

				BeforeEach(func() {
					testRun.Spec.RetentionPolicy = konfirm.RetainNever
				})

				It("it deletes the tests", func() {
					Eventually(func() ([]konfirm.Test, error) {
						return getTests(ctx, testRun)
					}, timeout).Should(BeEmpty())
				})
			})

			When("the retention policy is On Failure", func() {

				BeforeEach(func() {
					testRun.Spec.RetentionPolicy = konfirm.RetainOnFailure
				})

				It("it deletes the tests", func() {
					Eventually(func() ([]konfirm.Test, error) {
						return getTests(ctx, testRun)
					}, timeout).Should(BeEmpty())
				})
			})
		})

		When("the pod(s) fail", func() {

			JustBeforeEach(func() {
				Eventually(func() (ok bool, err error) {
					var tests []konfirm.Test
					if tests, err = getTests(ctx, testRun); err != nil || len(tests) == 0 {
						return
					}
					for i := range tests {
						var pods []v1.Pod
						if pods, err = getPods(ctx, &tests[i]); err != nil || len(pods) == 0 {
							return
						}
						for j := range pods {
							if err = fail(ctx, &pods[j]); err != nil {
								return
							}
						}
					}
					ok = true
					return
				}, timeout).Should(BeTrue())
			})

			It("it fails", func() {
				Eventually(func() (konfirm.TestRunPhase, error) {
					return testRun.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun)
				}, timeout).Should(Equal(konfirm.TestRunFailed))
				Expect(testRun.Status.Conditions).To(ContainElement(And(
					HaveField("Type", "RunStarted"),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", "TestRunStarted"),
				)))
				Expect(testRun.Status.Conditions).To(ContainElement(And(
					HaveField("Type", "RunCompleted"),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", "TestRunFailed"),
				)))
			})

			It("it retains the tests", func() {
				Consistently(func() ([]konfirm.Test, error) {
					return getTests(ctx, testRun)
				}, timeout).Should(HaveLen(len(testRun.Spec.Tests)))
			})

			When("the retention policy is Always", func() {

				BeforeEach(func() {
					testRun.Spec.RetentionPolicy = konfirm.RetainAlways
				})

				It("it retains the test(s)", func() {
					Consistently(func() ([]konfirm.Test, error) {
						return getTests(ctx, testRun)
					}, timeout).Should(HaveLen(len(testRun.Spec.Tests)))
				})
			})

			When("the retention policy is Never", func() {

				BeforeEach(func() {
					testRun.Spec.RetentionPolicy = konfirm.RetainNever
				})

				It("it deletes the tests", func() {
					Eventually(func() ([]konfirm.Test, error) {
						return getTests(ctx, testRun)
					}, timeout).Should(BeEmpty())
				})
			})

			When("the retention policy is On Failure", func() {

				BeforeEach(func() {
					testRun.Spec.RetentionPolicy = konfirm.RetainOnFailure
				})

				It("it retains the tests", func() {
					Consistently(func() ([]konfirm.Test, error) {
						return getTests(ctx, testRun)
					}, timeout).Should(HaveLen(len(testRun.Spec.Tests)))
				})
			})
		})
	})
})
