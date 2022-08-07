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
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TestSuite Controller", func() {

	const timeout = "500ms"

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
		}
	})

	AfterEach(func() {

		// All pods are gone
		Eventually(func() ([]v1.Pod, error) {
			var pods v1.PodList
			if err := k8sClient.List(ctx, &pods); err == nil {
				return pods.Items, nil
			} else {
				return nil, err
			}
		}, timeout).Should(BeEmpty())

		// All tests are gone
		Eventually(func() ([]konfirm.Test, error) {
			var tests konfirm.TestList
			if err := k8sClient.List(ctx, &tests); err == nil {
				return tests.Items, nil
			} else {
				return nil, err
			}
		}, timeout).Should(BeEmpty())

		// All test runs are gone
		Eventually(func() ([]konfirm.TestRun, error) {
			var testRuns konfirm.TestRunList
			if err := k8sClient.List(ctx, &testRuns); err == nil {
				return testRuns.Items, nil
			} else {
				return nil, err
			}
		}, timeout).Should(BeEmpty())

		// All test suites are gone
		Eventually(func() ([]konfirm.TestSuite, error) {
			testsuites := konfirm.TestSuiteList{}
			return testsuites.Items, k8sClient.List(ctx, &testsuites)
		}, timeout).Should(BeEmpty(), "not all TestSuites were removed")
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

	When("a test suite exists", func() {

		BeforeEach(func() {
			// Create a test suite and let it reach Ready
			Expect(k8sClient.Create(ctx, testSuite)).NotTo(HaveOccurred())
			Eventually(func() (konfirm.TestSuitePhase, error) {
				return testSuite.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
			}, timeout).Should(Equal(konfirm.TestSuiteReady))
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, testSuite)).NotTo(HaveOccurred())
		})

		When("and it is triggered", func() {

			BeforeEach(func() {
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

		When("and a previous test runs exists", func() {

			var testRun *konfirm.TestRun

			BeforeEach(func() {

				// Add a test run
				yes := true
				testRun = &konfirm.TestRun{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: testSuite.Name + "-",
						Namespace:    testSuite.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         konfirm.GroupVersion.String(),
								Kind:               "TestSuite",
								Name:               testSuite.Name,
								UID:                testSuite.UID,
								Controller:         &yes,
								BlockOwnerDeletion: &yes,
							},
						},
					},
					Spec: konfirm.TestRunSpec{
						Tests: testSuite.Spec.Tests,
					},
				}
				Expect(k8sClient.Create(ctx, testRun)).NotTo(HaveOccurred())

				// Finish the test
				Eventually(func() (bool, error) {

					var test *konfirm.Test
					var tests konfirm.TestList
					if err := k8sClient.List(ctx, &tests, client.InNamespace(testRun.Namespace)); err == nil {
						for i := range tests.Items {
							for j := range tests.Items[i].OwnerReferences {
								if o := &tests.Items[i].OwnerReferences[j]; o.UID == testRun.UID {
									test = &tests.Items[i]
									break
								}
							}
							if test != nil {
								break
							}
						}
						if test == nil {
							return false, nil
						}
					} else {
						return false, err
					}

					var pods v1.PodList
					if err := k8sClient.List(ctx, &pods, client.InNamespace(test.Namespace)); err == nil {
						for i := range pods.Items {
							for j := range pods.Items[i].OwnerReferences {
								if o := &pods.Items[i].OwnerReferences[j]; o.UID == test.UID {
									pod := &pods.Items[i]
									orig := pod.DeepCopy()
									pod.Status.Phase = v1.PodSucceeded
									return true, k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))
								}
							}
						}
					} else {
						return false, err
					}

					return false, nil
				}, timeout).Should(BeTrue())
			})

			When("a new run is triggered", func() {

				BeforeEach(func() {
					orig := testSuite.DeepCopy()
					testSuite.Trigger = konfirm.TestSuiteTrigger{NeedsRun: true}
					Expect(k8sClient.Patch(ctx, testSuite, client.MergeFrom(orig))).NotTo(HaveOccurred())
				})

				It("the previous run is removed", func() {
					Eventually(func() bool {
						ok := false
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), &konfirm.TestRun{}); err != nil {
							ok = apierrors.IsNotFound(err)
						}
						return ok
					}, timeout).Should(BeTrue())
				})
			})
		})
	})

	//It("an associated test should be run", func() {
	//
	//	var test *konfirm.Test
	//	Eventually(func() bool {
	//		ok := false
	//		if tests, err := getTests(ctx, testSuite); err == nil && len(tests) == 1 {
	//			test = &tests[0]
	//			ok = true
	//		}
	//		return ok
	//	}, timeout).Should(BeTrue())
	//
	//	Eventually(func() bool {
	//		ok := false
	//		if pods, e := getPods(ctx, test); e == nil {
	//			for _, p := range pods {
	//				orig := p.DeepCopy()
	//				p.Status.Phase = v1.PodSucceeded
	//				if e := k8sClient.Patch(ctx, &p, client.MergeFrom(orig)); e != nil {
	//					ok = false
	//					break
	//				}
	//				ok = true
	//			}
	//		}
	//		return ok
	//	}, timeout).Should(BeTrue())
	//})

})
