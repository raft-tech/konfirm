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

package controllers

import (
	"context"
	konfirmv1alpha1 "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RunningCondition        = "Running"
	PassedCondition         = "Passed"
	podIndexKey             = ".metadata.controller"
	TestStartedEvent        = "PodCreated"
	TestErrorEvent          = "ErrorCreatingPod"
	TestPassedEvent         = "TestPassed"
	TestFailedEvent         = "TestFailed"
	TestControllerFinalizer = konfirmv1alpha1.GroupName + "/test"
)

// TestReconciler reconciles a Test object
type TestReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests,verbs=get;list;watch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/finalizers,verbs=patch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := logging.FromContext(ctx, "test", req.NamespacedName)
	logger.Debug().Info("starting test reconciliation")

	// Retrieve the subject Test
	logger.Trace().Info("getting test")
	var test konfirmv1alpha1.Test
	if err := r.Get(ctx, req.NamespacedName, &test); err != nil {
		err = client.IgnoreNotFound(err)
		if err != nil {
			logger.Error(err, "error getting test")
		} else {
			logger.Trace().Info("test does not exist")
		}
		return ctrl.Result{}, err
	}
	logger.Trace().Info("retrieved test")

	// Retrieve any pods
	logger.Trace().Info("getting pods")
	var pods v1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(req.Namespace), client.MatchingFields{podIndexKey: req.Name}); err != nil {
		logger.Error(err, "error getting pods")
		return ctrl.Result{}, err
	}

	// If test is being deleted clean up pods and return
	if test.DeletionTimestamp != nil {
		logger.Trace().Info("deleting pods")
		err := r.deleteTestPods(ctx, pods.Items)
		if err == nil {
			logger.Info("deleted pods")
		} else {
			logger.Error(err, "error deleting pod(s)")
		}
		return ctrl.Result{}, err
	}

	// Create a Pod if needed
	if count := len(pods.Items); count == 0 {
		logger.Trace().Info("no pods found")
		if pod, err := r.createTestPod(ctx, req, &test); err == nil {
			logger.Info("pod created")
			r.Recorder.Eventf(&test, "Normal", TestStartedEvent, "created pod %s/%s", pod.Namespace, pod.Name)
		} else {
			logger.Error(err, "error creating pod")
			r.Recorder.Event(&test, "Warning", TestErrorEvent, "error creating pod")
			// TODO: If timeout is implemented, set a requeue after
			return ctrl.Result{}, err
		}
	} else {
		logger.Trace().Info("retrieved pods", "count", count)
	}

	// Update the test status as needed
	phase := konfirmv1alpha1.TestPhaseUnknown
	var messages []string
	for _, pod := range pods.Items {
		// If more than one pod exists, we'll use the results of the first observed to have completed
		switch pod.Status.Phase {
		case v1.PodRunning:
			// If a pod is running, the test is running
			if !phase.IsFinal() {
				phase = konfirmv1alpha1.TestRunning
			}
		case v1.PodSucceeded:
			phase = konfirmv1alpha1.TestSucceeded
		case v1.PodFailed:
			phase = konfirmv1alpha1.TestFailed
		}
		// If the current pod either Succeeded or Failed, retrieve termination messages and exit the loop
		if phase.IsFinal() {
			for _, status := range pod.Status.ContainerStatuses {
				if state := status.State.Terminated; state != nil {
					messages = append(messages, state.Message)
				}
			}
			break
		}
	}
	if phase != konfirmv1alpha1.TestPhaseUnknown && phase != test.Status.Phase {
		logger.Info("updating status", "newPhase", phase)
		if err := r.Client.Status().Update(ctx, &test); err != nil {
			logger.Error(err, "error updating status")
			return ctrl.Result{}, err
		}
		// TODO Update conditions
		switch phase {
		case konfirmv1alpha1.TestRunning:
			break
		case konfirmv1alpha1.TestSucceeded:
			r.Recorder.Event(&test, "Normal", TestPassedEvent, "Test passed")
		case konfirmv1alpha1.TestFailed:
			r.Recorder.Event(&test, "Warning", TestFailedEvent, "Test failed")
		}
		// TODO Apply retention policy
	}

	return ctrl.Result{}, nil
}

func (r *TestReconciler) createTestPod(ctx context.Context, req ctrl.Request, test *konfirmv1alpha1.Test) (*v1.Pod, error) {
	yes := true
	pod := v1.Pod{
		ObjectMeta: test.Spec.Template.ObjectMeta,
		Spec:       test.Spec.Template.Spec,
	}
	pod.ObjectMeta.Name = ""
	pod.ObjectMeta.Namespace = req.Namespace
	pod.ObjectMeta.GenerateName = test.Name + "-"
	pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         test.APIVersion,
			Kind:               test.Kind,
			Name:               test.Name,
			UID:                test.UID,
			Controller:         &yes,
			BlockOwnerDeletion: &yes,
		},
	}
	pod.ObjectMeta.Finalizers = []string{TestControllerFinalizer}
	return &pod, r.Create(ctx, &pod)
}

func (r *TestReconciler) deleteTestPods(ctx context.Context, pods []v1.Pod) error {
	errs := ErrorList{}
	for _, pod := range pods {

		// Patch the pod to remove the konfirm finalizer, preserving any additional finalizers
		orig := pod.DeepCopy()
		pod.ObjectMeta.Finalizers = []string{}
		for _, f := range orig.Finalizers {
			if f != TestControllerFinalizer {
				pod.ObjectMeta.Finalizers = append(pod.ObjectMeta.Finalizers, f)
			}
		}
		if err := r.Patch(ctx, &pod, client.MergeFrom(orig)); err != nil {
			errs.Append(err)
		}

		// Delete the pod
		if pod.DeletionTimestamp == nil {
			if err := r.Delete(ctx, &pod); err != nil {
				errs.Append(err)
			}
		}
	}
	return errs.Error()
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Set up an indexer to reconcile on changes to controlled pods
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Pod{}, podIndexKey, func(rawObj client.Object) []string {
		// Get the pod and owner
		pod := rawObj.(*v1.Pod)
		owner := metav1.GetControllerOf(pod)
		if owner == nil {
			return nil
		}
		// Return the owner if it's a test
		if owner.APIVersion == konfirmv1alpha1.GroupVersion.String() && owner.Kind == "Test" {
			return []string{owner.Name}
		} else {
			return nil
		}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&konfirmv1alpha1.Test{}).
		Owns(&v1.Pod{}).
		Complete(r)
}
