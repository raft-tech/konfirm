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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	testIndexKey = ".metadata.controller"
)

// ManualTriggerReconciler reconciles a ManualTrigger object
type ManualTriggerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=manualtriggers,verbs=get;list;watch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=manualtriggers/trigger,verbs=get;update;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=manualtriggers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=manualtriggers/finalizers,verbs=update
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites,verbs=get;list;watch;
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the ManualTrigger object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *ManualTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManualTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Set up an indexer to reconcile on changes to controlled tests
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Pod{}, testIndexKey, func(rawObj client.Object) []string {
		// Get the test and owner
		test := rawObj.(*konfirmv1alpha1.Test)
		owner := metav1.GetControllerOf(test)
		if owner == nil {
			return nil
		}
		// Return the owner if it's a ManualTrigger
		if owner.APIVersion == konfirmv1alpha1.GroupVersion.String() && owner.Kind == "ManualTrigger" {
			return []string{owner.Name}
		} else {
			return nil
		}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&konfirmv1alpha1.ManualTrigger{}).
		Owns(&konfirmv1alpha1.Test{}).
		Complete(r)
}
