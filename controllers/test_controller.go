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
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	podIndexKey             = ".metadata.controller"
	PodCreatedCondition     = "PodCreated"
	TestCompletedCondition  = "TestCompleted"
	TestStartingEvent       = "PodCreated"
	TestPassedEvent         = "TestPassed"
	TestFailedEvent         = "TestFailed"
	TestErrorEvent          = "ErrorCreatingPod"
	TestControllerFinalizer = konfirm.GroupName + "/test-controller"
)

// TestReconciler reconciles a Test object
type TestReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ErrRequeueDelay time.Duration
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests,verbs=get;list;watch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests/status,verbs=get;patch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="",resources=events,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {

	logger := logging.FromContextWithName(ctx, "test-controller")
	logger.Debug("starting test reconciliation")

	// Retrieve the subject Test
	logger.Trace("getting test")
	var test konfirm.Test
	if err = r.Get(ctx, req.NamespacedName, &test); err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Info("error getting test")
			res = ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}
			return
		}
		logger.Debug("test no longer exists")
		return
	}
	logger.Trace("retrieved test")

	// Retrieve any controlled pods
	logger.Trace("getting pods")
	var pods v1.PodList
	if err = r.List(ctx, &pods, client.InNamespace(req.Namespace), client.MatchingFields{podIndexKey: req.Name}); err != nil {
		logger.Info("error getting pods")
		res = ctrl.Result{
			Requeue:      true,
			RequeueAfter: r.ErrRequeueDelay,
		}
		return
	}
	logger.Debug("retrieved controlled pods")

	// If deleted, clean up
	if test.DeletionTimestamp != nil {
		logger.Info("test is being deleted")
		if l := len(pods.Items); l > 0 {
			logger.Trace("deleting pods")
			errs := ErrorList{}
			for i := range pods.Items {
				if _, e := cleanUp(ctx, r.Client, TestControllerFinalizer, &pods.Items[i]); e != nil {
					errs.Append(e)
				}
			}
			if errs.HasError() {
				logger.Info("one or more errors occurred while cleaning up managed pod(s)")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}, errs.Error()
			}
			logger.Info("pod deleted and finalizer removed")
		}
		logger.Trace("removing finalizer")
		if patched, err := removeFinalizer(ctx, r.Client, TestControllerFinalizer, &test); err != nil {
			if err = client.IgnoreNotFound(err); err != nil {
				logger.Info("error removing finalizer")
			}
		} else if patched {
			logger.Info("finalizer removed")
		}
		return
	}

	// Ensure the finalizer is set
	logger.Trace("ensuring finalizer is set")
	if patched, e := addFinalizer(ctx, r.Client, TestControllerFinalizer, &test); e != nil {
		if err = client.IgnoreNotFound(e); err != nil {
			logger.Info("error setting finalizer")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		}
		logger.Debug("test no longer exists")
		return
	} else if patched {
		logger.Debug("added finalizer")
	}

	// If Test status is empty, make Pending
	if test.Status.Phase == "" {
		logger.Trace("setting test as Pending")
		orig := test.DeepCopy()
		test.Status.Phase = konfirm.TestPending
		if err = r.Client.Status().Patch(ctx, &test, client.MergeFrom(orig)); err != nil {
			logger.Info("error setting test as Pending")
			res = ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}
			return
		}
		logger.Debug("test set to Pending", "phase", test.Status.Phase)
	}

	// Handle the current phase
	if test.Status.Phase.IsFinal() {
		res, err = r.isComplete(ctx, &test, pods.Items)
	} else {
		res, err = r.isRunning(ctx, &test, pods.Items)
	}

	return
}

func (r *TestReconciler) isRunning(ctx context.Context, test *konfirm.Test, pods []v1.Pod) (res ctrl.Result, err error) {

	logger := logging.FromContextWithName(ctx, "test-controller")

	// Determine which pod will be used to determine the test results
	var pod *v1.Pod
	if l := len(pods); l > 0 {

		// Remove unfinished, deleted pods from the list
		var newPods []*v1.Pod
		errs := ErrorList{}
		for i := range pods {
			p := &pods[i]
			if p.DeletionTimestamp != nil {
				logger.Trace("removing finalizer from pod", "pod", p.Name)
				if patched, err := removeFinalizer(ctx, r.Client, TestControllerFinalizer, p); err != nil {
					if err = client.IgnoreNotFound(err); err != nil {
						logger.Info("error removing finalizer from pod", "pod", p.Name)
						errs.Append(err)
					}
				} else if patched {
					logger.Info("removed finalizer from deleted pod", "pod", p.Name)
				}
				// If the pod completed before being deleted, keep it in the list
				if phase := p.Status.Phase; phase == v1.PodSucceeded || phase == v1.PodFailed {
					newPods = append(newPods, p)
				}
			} else {
				newPods = append(newPods, p)
			}
		}

		// If an error occurred, return now
		if errs.HasError() {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, errs.Error()
		}

		// Reduce the list of controlled pods to no more than 1
		if l := len(newPods); l > 0 {
			pod = newPods[0]
			errs = ErrorList{}
			for i := 1; i < l; i++ {
				if newPods[i].DeletionTimestamp == nil && pod.DeletionTimestamp != nil {
					pod = newPods[i]
				} else {
					logger.Debug("removing extraneous pod", "pod", newPods[i].Name)
					if _, err := cleanUp(ctx, r.Client, TestControllerFinalizer, newPods[i]); err != nil {
						if err = client.IgnoreNotFound(err); err != nil {
							logger.Info("error removing extraneous pod", "pod", newPods[i].Name)
							errs.Append(err)
						}
					} else {
						logger.Debug("removed extraneous pod", "pod", newPods[i].Name)
					}
				}
			}
			if errs.HasError() {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}, errs.Error()
			}
		}
	}

	// If a usable pod does not exist, create one
	if pod == nil {
		yes := true
		pod = &v1.Pod{
			ObjectMeta: test.Spec.Template.ObjectMeta,
			Spec:       test.Spec.Template.Spec,
		}
		pod.ObjectMeta.Name = ""
		pod.ObjectMeta.Namespace = test.Namespace
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
		if err = r.Create(ctx, pod); err != nil {
			logger.Info("error creating pod")
			r.Recorder.Event(test, "Warning", TestErrorEvent, "An error occurred while creating a test pod")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		} else {
			logger.Info("test is starting", "pod", pod.Name)
		}
	}

	switch pod.Status.Phase {

	case v1.PodPending:
		if test.Status.Phase != konfirm.TestStarting {
			logger.Trace("updating test status")
			orig := test.DeepCopy()
			test.Status.Phase = konfirm.TestStarting
			meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
				Type:               PodCreatedCondition,
				Status:             "True",
				ObservedGeneration: test.Generation,
				Reason:             "PodCreated",
				Message:            fmt.Sprintf("Created pod %s", pod.Name),
			})
			meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
				Type:               TestCompletedCondition,
				Status:             "False",
				ObservedGeneration: test.Generation,
				Reason:             "PodNotCompleted",
				Message:            fmt.Sprintf("Pod %s is pending", pod.Name),
			})
			if err = r.Status().Patch(ctx, test, client.MergeFrom(orig)); err != nil {
				logger.Info("error setting test as Starting")
				res = ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}
			}
			logger.Info("test is Starting")
			r.Recorder.Eventf(test, "Normal", TestStartingEvent, "Created pod %s", pod.Name)
		}

	case v1.PodRunning:
		if test.Status.Phase != konfirm.TestRunning {
			logger.Trace("updating test status")
			orig := test.DeepCopy()
			test.Status.Phase = konfirm.TestRunning
			meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
				Type:               PodCreatedCondition,
				Status:             "True",
				ObservedGeneration: test.Generation,
				Reason:             "PodCreated",
				Message:            fmt.Sprintf("Created pod %s", pod.Name),
			})
			meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
				Type:               TestCompletedCondition,
				Status:             "False",
				ObservedGeneration: test.Generation,
				Reason:             "PodNotCompleted",
				Message:            fmt.Sprintf("Pod %s is running", pod.Name),
			})
			if err = r.Status().Patch(ctx, test, client.MergeFrom(orig)); err != nil {
				logger.Info("error setting test as Running")
				res = ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}
			} else {
				logger.Info("test is Running")
			}
		}

	case v1.PodSucceeded:
		orig := test.DeepCopy()
		test.Status.Phase = konfirm.TestPassed
		test.Status.Messages = make(map[string]string)
		for i := range pod.Status.ContainerStatuses {
			if state := pod.Status.ContainerStatuses[i].State.Terminated; state != nil {
				test.Status.Messages[pod.Status.ContainerStatuses[i].Name] = state.Message
			}
		}
		meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
			Type:               TestCompletedCondition,
			Status:             "True",
			ObservedGeneration: test.Generation,
			Reason:             "PodSucceeded",
			Message:            fmt.Sprintf("Pod %s completed successfully", pod.Name),
		})
		if err = r.Client.Status().Patch(ctx, test, client.MergeFrom(orig)); err != nil {
			logger.Info("error setting test as Passed")
			res = ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}
		} else {
			logger.Info("test Passed")
			r.Recorder.Eventf(test, "Normal", TestPassedEvent, "Pod %s succeeded", pod.Name)
		}

	case v1.PodFailed:
		orig := test.DeepCopy()
		test.Status.Phase = konfirm.TestFailed
		test.Status.Messages = make(map[string]string)
		for i := range pod.Status.ContainerStatuses {
			if state := pod.Status.ContainerStatuses[i].State.Terminated; state != nil {
				test.Status.Messages[pod.Status.ContainerStatuses[i].Name] = state.Message
			}
		}
		meta.SetStatusCondition(&test.Status.Conditions, metav1.Condition{
			Type:               TestCompletedCondition,
			Status:             "True",
			ObservedGeneration: test.Generation,
			Reason:             "PodFailed",
			Message:            fmt.Sprintf("Pod %s failed", pod.Name),
		})
		if err = r.Client.Status().Patch(ctx, test, client.MergeFrom(orig)); err != nil {
			logger.Info("error setting test as Failed")
			res = ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}
		} else {
			logger.Info("test Failed")
			r.Recorder.Eventf(test, "Normal", TestFailedEvent, "Pod %s failed", pod.Name)
		}
	}

	return
}

func (r *TestReconciler) isComplete(ctx context.Context, test *konfirm.Test, pods []v1.Pod) (res ctrl.Result, err error) {

	logger := logging.FromContextWithName(ctx, "test-controller")

	// Remove finalizers from deleted pods
	errs := ErrorList{}
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			logger.Trace("removing finalizer from pod", "pod", pod.Name)
			if patched, err := removeFinalizer(ctx, r.Client, TestControllerFinalizer, pod); err != nil {
				if err = client.IgnoreNotFound(err); err != nil {
					logger.Info("error removing finalizer from pod", "pod", pod.Name)
					errs.Append(err)
				}
			} else if patched {
				logger.Info("removed finalizer from deleted pod", "pod", pod.Name)
			}
		}
	}
	if errs.HasError() {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: r.ErrRequeueDelay,
		}, errs.Error()
	}

	// Apply retention policy
	if policy := test.Spec.RetentionPolicy; policy == konfirm.RetainNever ||
		(policy == konfirm.RetainOnFailure && test.Status.Phase == konfirm.TestPassed) {
		if l := len(pods); l > 0 {
			logger.Trace("applying retention policy")
			errs = ErrorList{}
			for i := range pods {
				if _, e := cleanUp(ctx, r.Client, TestControllerFinalizer, &pods[i]); e != nil {
					errs.Append(e)
				}
			}
			if errs.HasError() {
				logger.Info("one or more errors occurred while cleaning up managed pod(s)")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}, errs.Error()
			}
			logger.Info("pod removed according to retention policy")
		}
	}

	return
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
		if owner.APIVersion == konfirm.GroupVersion.String() && owner.Kind == "Test" {
			return []string{owner.Name}
		} else {
			return nil
		}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&konfirm.Test{}).
		Owns(&v1.Pod{}).
		Complete(r)
}
