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
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getTestRuns is a helper function for retrieving TestRuns associated with a specified TestSuite
func getTestRuns(ctx context.Context, t *konfirm.TestSuite) ([]konfirm.TestRun, error) {
	var testRuns []konfirm.TestRun
	var err error
	var testRunList konfirm.TestRunList
	if err = k8sClient.List(ctx, &testRunList, client.InNamespace(t.Namespace)); err == nil {
		for _, t := range testRunList.Items {
			for _, o := range t.GetOwnerReferences() {
				if o.APIVersion == konfirm.GroupVersion.String() &&
					o.Kind == "TestSuite" &&
					o.Name == t.Name {
					testRuns = append(testRuns, t)
				}
			}
		}
	}
	return testRuns, err
}

// getTests is a helper function for retrieving Tests associated with a specified TestRun
func getTests(ctx context.Context, t *konfirm.TestRun) ([]konfirm.Test, error) {
	var tests []konfirm.Test
	var err error
	var testList konfirm.TestList
	if err = k8sClient.List(ctx, &testList, client.InNamespace(t.Namespace)); err == nil {
		for _, t := range testList.Items {
			for _, o := range t.GetOwnerReferences() {
				if o.APIVersion == konfirm.GroupVersion.String() &&
					o.Kind == "TestRun" &&
					o.Name == t.Name {
					tests = append(tests, t)
				}
			}
		}
	}
	return tests, err
}

// getPods is a helper function for retrieving Pods associated with a specific Test
func getPods(ctx context.Context, test *konfirm.Test) ([]v1.Pod, error) {
	var pods []v1.Pod
	var err error
	var podList v1.PodList
	if err = k8sClient.List(ctx, &podList, client.InNamespace(test.Namespace)); err == nil {
		for _, p := range podList.Items {
			for _, o := range p.GetOwnerReferences() {
				if o.APIVersion == konfirm.GroupVersion.String() &&
					o.Kind == "Test" &&
					o.Name == test.Name {
					pods = append(pods, p)
				}
			}
		}
	}
	return pods, err
}
