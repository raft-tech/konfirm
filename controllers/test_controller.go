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
	"fmt"
	konfirmv1alpha1 "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodCreatedCondition     = "PodCreated"
	TestCompletedCondition  = "TestCompleted"
	podIndexKey             = ".metadata.controller"
	TestStartingEvent       = "PodCreated"
	TestRunningEvent        = "PodRunning"
	TestPassedEvent         = "TestPassed"
	TestFailedEvent         = "TestFailed"
	TestErrorEvent          = "ErrorCreatingPod"
	TestControllerFinalizer = konfirmv1alpha1.GroupName + "/test-controller"
)

// TestReconciler reconciles a Test object
type TestReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/finalizers,verbs=patch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="",resources=events,verbs=create

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
	logger = logger.WithValues("generation", test.Generation)
	logger.Trace().Info("retrieved test")

	// Retrieve any controlled pods
	logger.Trace().Info("getting pods")
	var pods v1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(req.Namespace), client.MatchingFields{podIndexKey: req.Name}); err != nil {
		logger.Error(err, "error getting pods")
		return ctrl.Result{}, err
	}

	// If test is being deleted clean up pods and return
	if test.DeletionTimestamp != nil {

	}

	// Retrieve or create the pod
	var pod *v1.Pod
	switch l := len(pods.Items); true {
	case test.DeletionTimestamp != nil && l > 0:
		logger.Trace().Info("deleting pods")
		err := r.deleteTestPods(ctx, pods.Items)
		if err == nil {
			logger.Info("pods deleted")
		} else {
			logger.Error(err, "error deleting pod(s)")
		}
		return ctrl.Result{}, err
	case l == 1:
		pod = &pods.Items[0]
	case l > 1:
		logger.V(-1).Info("more than one pod owned by test", "count", l)
		// Clean up and start over
		if !test.Status.Phase.IsFinal() {
			err := r.deleteTestPods(ctx, pods.Items)
			if err != nil {
				logger.Error(err, "error cleaning up extraneous pods")
			}
			return ctrl.Result{}, err
		}
	case test.Status.Phase.IsFinal() || test.DeletionTimestamp != nil:
		orig := test.DeepCopy()
		test.Finalizers = []string{}
		for _, f := range orig.GetFinalizers() {
			if f != TestControllerFinalizer {
				test.Finalizers = append(test.Finalizers, f)
			}
		}
		logger.Trace().Info("patching finalizer")
		err := r.Client.Patch(ctx, &test, client.MergeFrom(orig))
		if err == nil {
			logger.Debug().Info("finalizer removed")
		} else {
			logger.Error(err, "error removing Test finalizer")
		}
		return ctrl.Result{}, nil
	default:
		// Test needs Pod. If finalizer is set, create pods. Otherwise add finalizer
		hasFinalizer := false
		for _, f := range test.GetFinalizers() {
			if f == TestControllerFinalizer {
				hasFinalizer = true
				break
			}
		}
		if !hasFinalizer {
			// Do not create pods until Test has konfirm finalizer
			orig := test.DeepCopy()
			test.Finalizers = append(test.Finalizers, TestControllerFinalizer)
			err := r.Client.Patch(ctx, &test, client.MergeFrom(orig))
			if err == nil {
				logger.Debug().Info("added test-controller finalizer")
			} else {
				logger.Error(err, "error adding test-controller finalizer")
			}
			return ctrl.Result{}, err
		}
		logger.Trace().Info("creating pod")
		if p, err := r.createTestPod(ctx, req, &test); err == nil {
			logger.Info("pod created", "pod", client.ObjectKeyFromObject(p).String())
			pod = p
		} else {
			logger.Error(err, "error creating pod")
			r.Recorder.Event(&test, "Warning", TestErrorEvent, "error creating pod")
			return ctrl.Result{}, err
		}
	}

	// Update the test status as needed
	orig := test.DeepCopy()
	test.Status.Phase.FromPodPhase(pod.Status.Phase)
	meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
		Type:               PodCreatedCondition,
		Status:             "True",
		ObservedGeneration: test.Generation,
		Reason:             "PodCreated",
		Message:            fmt.Sprintf("pod %s/%s was created", pod.Namespace, pod.Name),
	})
	if test.Status.Phase.IsFinal() {
		meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
			Type:               TestCompletedCondition,
			Status:             "True",
			ObservedGeneration: test.Generation,
			Reason:             "PodComplete",
			Message:            fmt.Sprintf("pod %s/%s has completed", pod.Namespace, pod.Name),
		})
		test.Status.Messages = make(map[string]string)
		for _, status := range pod.Status.ContainerStatuses {
			if state := status.State.Terminated; state != nil {
				test.Status.Messages[status.Name] = state.Message
			}
		}
	} else {
		meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
			Type:               TestCompletedCondition,
			Status:             "False",
			ObservedGeneration: test.Generation,
			Reason:             "PodNotComplete",
			Message:            fmt.Sprintf("pod %s/%s has not completed", pod.Namespace, pod.Name),
		})
	}

	logger.Trace().Info("patching status")
	err := r.Client.Status().Patch(ctx, &test, client.MergeFrom(orig))
	if err == nil {
		logger.Info("status patched", "phase", test.Status.Phase)
	} else {
		logger.Error(err, "error patching status")
	}

	// Record the appropriate event
	if orig.Status.Phase != test.Status.Phase {
		switch test.Status.Phase {
		case konfirmv1alpha1.TestStarting:
			r.Recorder.Eventf(&test, "Normal", TestStartingEvent, "pod %s/%s is pending", pod.Namespace, pod.Name)
		case konfirmv1alpha1.TestRunning:
			r.Recorder.Eventf(&test, "Normal", TestRunningEvent, "pod %s/%s is running", pod.Namespace, pod.Name)
		case konfirmv1alpha1.TestPassed:
			r.Recorder.Eventf(&test, "Normal", TestPassedEvent, "pod %s/%s succeeded", pod.Namespace, pod.Name)
		case konfirmv1alpha1.TestFailed:
			r.Recorder.Eventf(&test, "Warning", TestFailedEvent, "pod %s/%s failed", pod.Namespace, pod.Name)
		}
	}

	// Handle retention by policy
	if test.Status.Phase.IsFinal() {
		switch test.Spec.RetentionPolicy {
		case konfirmv1alpha1.RetainOnFailure:
			if test.Status.Phase == konfirmv1alpha1.TestFailed {
				logger.Info("retaining test pods due to RetainOnFailure")
				break
			}
			fallthrough
		case konfirmv1alpha1.RetainNever:
			if err := r.deleteTestPods(ctx, pods.Items); err == nil {
				logger.Info("pods removed based on retention policy")
			} else {
				logger.Error(err, "error cleaning up pods after test completion")
			}
		case konfirmv1alpha1.RetainAlways:
			logger.Info("retaining test pods due to RetainAlways")
		}
	}

	return ctrl.Result{}, err
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
	pod.Spec.RestartPolicy = v1.RestartPolicyNever
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
		if err := r.Patch(ctx, &pod, client.MergeFrom(orig)); err == nil {
			// Delete the pod
			if pod.DeletionTimestamp == nil {
				if err := r.Delete(ctx, &pod); err != nil {
					errs.Append(err)
				}
			}
		} else {
			errs.Append(err)
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
