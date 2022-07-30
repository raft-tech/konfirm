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

	const timeout = "100ms"

	var (
		ctx       context.Context
		testSuite *konfirm.TestSuite
	)

	BeforeEach(func() {
		ctx = context.Background()
		testSuite = &konfirm.TestSuite{
			TypeMeta: metav1.TypeMeta{
				APIVersion: konfirm.GroupVersion.String(),
				Kind:       "TestSuite",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a-test-suite",
				Namespace: "default",
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
			pods := v1.PodList{}
			return pods.Items, k8sClient.List(ctx, &pods)
		}, timeout).Should(BeEmpty(), "not all Pods were removed")

		// All tests are gone
		Eventually(func() ([]konfirm.Test, error) {
			tests := konfirm.TestList{}
			return tests.Items, k8sClient.List(ctx, &tests)
		}, timeout).Should(BeEmpty(), "not all Tests were removed")

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

		It("reaches the Ready state", func() {
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

		When("previous tests where run", func() {

			var test *konfirm.Test

			BeforeEach(func() {

				// Add a test
				yes := true
				test = &konfirm.Test{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: testSuite.Name + "-akasd8a-",
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
					Spec: konfirm.TestSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Name:  "main",
										Image: "konfirm/mock:v1",
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, test)).NotTo(HaveOccurred())

				// Progress the pod to succeeded
				Eventually(func() (v1.PodPhase, error) {
					var phase v1.PodPhase
					var err error
					var pods v1.PodList
					if err = k8sClient.List(ctx, &pods, client.InNamespace(test.Namespace)); err == nil {
						for _, p := range pods.Items {
							for _, o := range p.GetOwnerReferences() {
								if o.Name == test.Name {
									orig := p.DeepCopy()
									p.Status.Phase = v1.PodSucceeded
									if err = k8sClient.Status().Patch(ctx, &p, client.MergeFrom(orig)); err == nil {
										phase = p.Status.Phase
									}
								}
							}
						}
					}
					return phase, err
				}, timeout).Should(Equal(v1.PodSucceeded))

				// Let the test progress to Passed
				Eventually(func() (konfirm.TestPhase, error) {
					return test.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test)
				}, timeout).Should(Equal(konfirm.TestPassed))
			})

			When("a new run is triggered", func() {

				BeforeEach(func() {
					orig := testSuite.DeepCopy()
					testSuite.Trigger = konfirm.TestSuiteTrigger{NeedsRun: true}
					Expect(k8sClient.Patch(ctx, testSuite, client.MergeFrom(orig))).NotTo(HaveOccurred())
				})

				It("it removes previous tests before new tests are run", func() {
					Eventually(func() bool {
						ok := false
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(test), &konfirm.Test{}); err != nil {
							ok = apierrors.IsNotFound(err)
						}
						return ok
					}, "3s").Should(BeTrue())
				})
			})
		})

		When("the test suite is triggered", func() {

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
