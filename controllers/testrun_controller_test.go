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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TestRun Controller", func() {

	const timeout = "200ms"

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
							Description: "test1",
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
						}, {
							Description: "test2",
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
			Expect(k8sClient.Create(ctx, testRun)).NotTo(HaveOccurred())
		})

		It("it progresses to Running", func() {
			Eventually(func() (phase konfirm.TestRunPhase, err error) {
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun); err == nil {
					phase = testRun.Status.Phase
				}
				return
			}, timeout).Should(Equal(konfirm.TestRunRunning))
		})

		When("the pod(s) succeed", func() {

			BeforeEach(func() {
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

			It("it passes", func() {
				Eventually(func() (konfirm.TestRunPhase, error) {
					return testRun.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun)
				}, timeout).Should(Equal(konfirm.TestRunPassed))
			})
		})

		When("the pod(s) fail", func() {

			BeforeEach(func() {
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
									pod.Status.Phase = v1.PodFailed
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

			It("it fails", func() {
				Eventually(func() (konfirm.TestRunPhase, error) {
					return testRun.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun)
				}, timeout).Should(Equal(konfirm.TestRunFailed))
			})
		})
	})
})
