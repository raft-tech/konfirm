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
	konfirmv1alpha1 "go.goraft.tech/konfirm/api/v1alpha1"
	"go.goraft.tech/konfirm/logging"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RunningCondition = "Running"
	PassedCondition  = "Passed"
	podIndexKey      = ".metadata.controller"
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
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := logging.FromContext(ctx, "test", req.NamespacedName)
	log.Debug().Info("starting test reconciliation")

	// Retrieve the subject Test
	log.Trace().Info("getting test")
	var test konfirmv1alpha1.Test
	if err := r.Get(ctx, req.NamespacedName, &test); err != nil {
		err = client.IgnoreNotFound(err)
		if err != nil {
			log.Error(err, "error getting test")
		} else {
			log.Trace().Info("test does not exist")
		}
		return ctrl.Result{}, err
	}
	log.Trace().Info("retrieved test")

	// Get any child pods
	log.Trace().Info("getting child pods")
	var pods v1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(req.Namespace), client.MatchingFields{podIndexKey: req.Name}); err != nil {
		log.Error(err, "error getting child pods")
		return ctrl.Result{}, err
	}
	if count := len(pods.Items); count == 0 {
		// No pod exists, so create one
		log.Trace().Info("no child pods found")
		// TODO: If timeout is implemented, set a requeue after
		return ctrl.Result{}, r.createPodForTest(ctx, req, &test)
	} else {
		log.Trace().Info("retrieved child pods", "count", count)
	}

	// If Test is already completed, clean up as needed
	if test.Status.Phase.IsFinal() {
		log.Debug().Info("running post-test cleanup")
		return ctrl.Result{}, r.cleanUpTestPods(ctx, req, &test, pods.Items)
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
		log.Info("updating status", "newPhase", phase)
		if err := r.Client.Status().Update(ctx, &test); err != nil {
			log.Error(err, "error updating status")
			return ctrl.Result{}, err
		}
		if phase.IsFinal() {
			return ctrl.Result{}, r.cleanUpTestPods(ctx, req, &test, pods.Items)
		}
	}

	return ctrl.Result{}, nil
}

func (r *TestReconciler) createPodForTest(ctx context.Context, req ctrl.Request, test *konfirmv1alpha1.Test) error {

	logger := logging.FromContext(ctx, "test", req.NamespacedName)
	yes := true

	// Generate the Pod
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
	pod.ObjectMeta.Finalizers = []string{konfirmv1alpha1.GroupName}

	// Create the pod
	logger.Trace().Info("creating child pod")
	if err := r.Create(ctx, &pod); err != nil {
		logger.Error(err, "error creating child pod")
		return err
	}
	logger.Info("child pod created")
	return nil
}

func (r *TestReconciler) cleanUpTestPods(ctx context.Context, req ctrl.Request, test *konfirmv1alpha1.Test, pods []v1.Pod) error {

	logger := logging.FromContext(ctx, "test", req.NamespacedName)

	// Determine if pods should be deleted
	deletePods := false
	if p := test.Spec.RetentionPolicy; p == konfirmv1alpha1.RetainNever ||
		(p == konfirmv1alpha1.RetainOnFailure && test.Status.Phase == konfirmv1alpha1.TestSucceeded) {
		deletePods = true
	}

	// Remove the konfirm finalizer and optionally delete pods
	errs := ErrorList{}
	for _, pod := range pods {

		logger := logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

		// Patch the pod to remove the konfirm finalizer, preserving any additional finalizers
		orig := pod.DeepCopy()
		pod.ObjectMeta.Finalizers = []string{}
		for _, f := range orig.Finalizers {
			if f != konfirmv1alpha1.GroupName {
				pod.ObjectMeta.Finalizers = append(pod.ObjectMeta.Finalizers, f)
			}
		}
		logger.Trace().Info("removing pod finalizer")
		if err := r.Patch(ctx, &pod, client.MergeFrom(orig)); err == nil {
			logger.Debug().Info("finalizer removed")
		} else {
			logger.Error(err, "error removing finalizer")
			errs.Append(err)
		}

		// Optionally delete the pod
		if deletePods && pod.DeletionTimestamp != nil {
			if err := r.Delete(ctx, &pod); err == nil {
				logger.Info("pod deleted")
			} else {
				logger.Error(err, "error deleting pod")
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
