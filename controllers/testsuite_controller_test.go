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
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TestSuite Controller", func() {

	const timeout = "100ms"

	var (
		ctx       context.Context
		namespace string
		testSuite *konfirm.TestSuite
	)

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

		testSuite = &konfirm.TestSuite{
			TypeMeta: metav1.TypeMeta{
				APIVersion: konfirm.GroupVersion.String(),
				Kind:       "TestSuite",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a-test-suite",
				Namespace: namespace,
			},
			Spec: konfirm.TestSuiteSpec{
				Template: konfirm.TestRunSpec{
					Tests: []konfirm.TestTemplate{
						{
							Description: "a-test",
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
					},
				},
			},
		}
	})

	When("a test suite is created", func() {

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, testSuite)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, testSuite)).NotTo(HaveOccurred())
		})

		It("it reaches the Ready state", func() {
			Eventually(func() (konfirm.TestSuitePhase, error) {
				return testSuite.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
			}, timeout).Should(Equal(konfirm.TestSuiteReady))
		})
	})

	Context("a test suite exists", func() {

		JustBeforeEach(func() {
			// Create a test suite and let it reach Ready
			Expect(k8sClient.Create(ctx, testSuite)).NotTo(HaveOccurred())
			Eventually(func() (konfirm.TestSuitePhase, error) {
				return testSuite.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
			}, timeout).Should(Equal(konfirm.TestSuiteReady))
		})

		When("and it is triggered", func() {

			JustBeforeEach(func() {
				orig := testSuite.DeepCopy()
				testSuite.Trigger = konfirm.TestSuiteTrigger{NeedsRun: true}
				Expect(k8sClient.Patch(ctx, testSuite, client.MergeFrom(orig))).NotTo(HaveOccurred())
			})

			It("it should progress to the Running phase", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
					g.Expect(testSuite.Status.Phase).To(Equal(konfirm.TestSuiteRunning))
					g.Expect(testSuite.Status.Conditions).To(HaveKey(And(
						HaveField("Type", controllers.TestSuiteRunStartedCondition),
						HaveField("Status", "True"),
						HaveField("Reason", "Manual"),
						HaveField("Message", "TestSuite was manually triggered"),
					)))
				})
			})
		})

		Context("historical test runs exist", func() {

			var historicalRuns []konfirm.TestRun

			BeforeEach(func() {
				testSuite.Spec.HistoryLimit = 6
				testRun := konfirm.TestRun{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: testSuite.Name + "-",
						Namespace:    testSuite.Namespace,
						Annotations: map[string]string{
							konfirm.GroupName + "/template": "abcdefg",
						},
						Finalizers: []string{controllers.TestSuiteControllerFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         testSuite.APIVersion,
								Kind:               testSuite.Kind,
								Name:               testSuite.Name,
								UID:                testSuite.UID,
								Controller:         &yes,
								BlockOwnerDeletion: &yes,
							},
						},
					},
					Spec: konfirm.TestRunSpec{
						RetentionPolicy: testSuite.Spec.Template.RetentionPolicy,
						Tests:           testSuite.Spec.Template.Tests,
					},
				}

				var results []konfirm.TestResult
				for i := range testRun.Spec.Tests {
					results = append(results, konfirm.TestResult{
						Description: testRun.Spec.Tests[i].Description,
						Passed:      true,
					})
				}

				for i := 0; i < 6; i++ {
					historicalRuns = append(historicalRuns, testRun)
				}
			})

			JustBeforeEach(func() {
				for i, j := 0, len(historicalRuns); i < j; i++ {

					// Create a test run
					testRun := &historicalRuns[i]
					testRun.OwnerReferences[0].UID = testSuite.UID
					Expect(k8sClient.Create(ctx, testRun)).NotTo(HaveOccurred())

					// Get the associated tests
					var tests []konfirm.Test
					Eventually(func() ([]konfirm.Test, error) {
						var err error
						tests, err = getTests(ctx, testRun)
						return tests, err
					}, timeout).ShouldNot(BeEmpty())

					// Get all the tests
					for i := range tests {

						test := &tests[i]

						// Get the associated pod
						var pod *v1.Pod
						Eventually(func() (*v1.Pod, error) {
							pods, err := getPods(ctx, test)
							if len(pods) > 0 {
								pod = &pods[0]
							}
							return pod, err
						}, timeout).ShouldNot(BeNil())

						// Pass the pod
						orig := pod.DeepCopy()
						pod.Status.Phase = v1.PodSucceeded
						pod.Status.Message = fmt.Sprintf("test-%s passed", test.Name)
						Expect(k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))).NotTo(HaveOccurred())
					}

					// Make sure the test run passes
					Eventually(func() (konfirm.TestRunPhase, error) {
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun); err != nil {
							return "", err
						}
						return testRun.Status.Phase, nil
					}, timeout).Should(Equal(konfirm.TestRunPassed))
				}
			})

			When("historical runs are deleted", func() {

				JustBeforeEach(func() {
					Expect(k8sClient.Delete(ctx, &historicalRuns[1])).NotTo(HaveOccurred())
					Expect(k8sClient.Delete(ctx, &historicalRuns[4])).NotTo(HaveOccurred())
					Expect(k8sClient.Delete(ctx, &historicalRuns[5])).NotTo(HaveOccurred())
				})

				It("removes the finalizer", func() {
					deleted := []int{1, 4, 5}
					for i := range deleted {
						n := deleted[i]
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(&historicalRuns[n]), &konfirm.TestRun{})
						}, timeout).Should(Satisfy(apierrors.IsNotFound), fmt.Sprintf("test run %d was not deleted", n))
					}
				})
			})
		})
	})
})
