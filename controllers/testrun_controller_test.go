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

	const timeout = "100ms"

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	When("a TestRun is created", func() {

		var testRun *konfirm.TestRun

		BeforeEach(func() {
			testRun = &konfirm.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a-testrun",
					Namespace: "default",
				},
				Spec: konfirm.TestRunSpec{
					Tests: []konfirm.TestTemplate{
						{
							Description: "a-test",
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

		AfterEach(func() {

			// No pods left
			Eventually(func() ([]v1.Pod, error) {
				var pods v1.PodList
				if err := k8sClient.List(ctx, &pods); err == nil {
					return pods.Items, nil
				} else {
					return nil, err
				}
			}, timeout).Should(BeEmpty())

			// No tests left
			Eventually(func() ([]konfirm.Test, error) {
				var tests konfirm.TestList
				if err := k8sClient.List(ctx, &tests); err == nil {
					return tests.Items, nil
				} else {
					return nil, err
				}
			}, timeout).Should(BeEmpty())

			// No test runs left
			Eventually(func() ([]konfirm.TestRun, error) {
				var testRuns konfirm.TestRunList
				if err := k8sClient.List(ctx, &testRuns); err == nil {
					return testRuns.Items, nil
				} else {
					return nil, err
				}
			}, timeout).Should(BeEmpty())
		})

		It("it progresses to Starting", func() {
			Eventually(func() (phase konfirm.TestRunPhase, err error) {
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun); err == nil {
					phase = testRun.Status.Phase
				}
				return
			}, timeout).Should(Equal(konfirm.TestRunStarting))
			Expect(k8sClient.Delete(ctx, testRun)).NotTo(HaveOccurred())
		})
	})
})
