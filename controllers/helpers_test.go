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
	"github.com/raft-tech/konfirm/controllers"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"testing"
	"time"
)

var (
	yes = true
	no  = false
)

// getTestRuns is a helper function for retrieving TestRuns associated with a specified TestSuite
func getTestRuns(ctx context.Context, testSuite *konfirm.TestSuite) ([]konfirm.TestRun, error) {
	var testRuns []konfirm.TestRun
	var err error
	var testRunList konfirm.TestRunList
	if err = k8sClient.List(ctx, &testRunList, client.InNamespace(testSuite.Namespace)); err == nil {
		for i := range testRunList.Items {
			for j := range testRunList.Items[i].OwnerReferences {
				if testRunList.Items[i].OwnerReferences[j].UID == testSuite.UID {
					testRuns = append(testRuns, testRunList.Items[i])
				}
			}
		}
	}
	return testRuns, err
}

// getTests is a helper function for retrieving Tests associated with a specified TestRun
func getTests(ctx context.Context, testRun *konfirm.TestRun) ([]konfirm.Test, error) {
	var tests []konfirm.Test
	var err error
	var testList konfirm.TestList
	if err = k8sClient.List(ctx, &testList, client.InNamespace(testRun.Namespace)); err == nil {
		for i := range testList.Items {
			for j := range testList.Items[i].OwnerReferences {
				if testList.Items[i].OwnerReferences[j].UID == testRun.UID {
					tests = append(tests, testList.Items[i])
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
		for i := range podList.Items {
			for j := range podList.Items[i].OwnerReferences {
				if podList.Items[i].OwnerReferences[j].UID == test.UID {
					pods = append(pods, podList.Items[i])
				}
			}
		}
	}
	return pods, err
}

func succeed(ctx context.Context, pod *v1.Pod) (err error) {
	orig := pod.DeepCopy()
	pod.Status.Phase = v1.PodSucceeded
	pod.Status.ContainerStatuses = make([]v1.ContainerStatus, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		state := v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:    0,
				Reason:      "Container exited",
				Message:     "Success!",
				StartedAt:   metav1.NewTime(pod.CreationTimestamp.Add(time.Millisecond * 1)),
				FinishedAt:  metav1.NewTime(pod.CreationTimestamp.Add(time.Millisecond * 10)),
				ContainerID: strconv.Itoa(i),
			}}
		pod.Status.ContainerStatuses[i] = v1.ContainerStatus{
			Name:                 pod.Spec.Containers[i].Name,
			State:                state,
			LastTerminationState: state,
			Ready:                false,
			RestartCount:         0,
			Image:                pod.Spec.Containers[i].Image,
			Started:              &no,
		}
	}
	err = k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))
	return
}

func fail(ctx context.Context, pod *v1.Pod) (err error) {
	orig := pod.DeepCopy()
	pod.Status.Phase = v1.PodFailed
	pod.Status.ContainerStatuses = make([]v1.ContainerStatus, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		state := v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:    1,
				Reason:      "Container exited",
				Message:     "Failure!",
				StartedAt:   metav1.NewTime(pod.CreationTimestamp.Add(time.Millisecond * 1)),
				FinishedAt:  metav1.NewTime(pod.CreationTimestamp.Add(time.Millisecond * 10)),
				ContainerID: strconv.Itoa(i),
			}}
		pod.Status.ContainerStatuses[i] = v1.ContainerStatus{
			Name:                 pod.Spec.Containers[i].Name,
			State:                state,
			LastTerminationState: state,
			Ready:                false,
			RestartCount:         0,
			Image:                pod.Spec.Containers[i].Image,
			Started:              &yes,
		}
	}
	err = k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))
	return
}

func TestGetBackOff(t *testing.T) {

	tests := []struct {
		input    time.Duration
		expected time.Duration
	}{
		{0, 5 * time.Second},
		{1 * time.Second, 5 * time.Second},
		{5 * time.Second, 10 * time.Second},
		{10 * time.Second, 20 * time.Second},
		{20 * time.Second, 40 * time.Second},
		{40 * time.Second, 80 * time.Second},
		{80 * time.Second, 160 * time.Second},
		{160 * time.Second, 5 * time.Minute},
		{5 * time.Minute, 5 * time.Minute},
	}

	for i := range tests {
		assert.Equal(t, tests[i].expected, controllers.GetBackOff(tests[i].input))
	}
}
