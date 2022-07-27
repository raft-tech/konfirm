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
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testIndexKey                 = ".metadata.controller"
	TestSuiteControllerFinalizer = konfirm.GroupName + "/testsuite-controller"
	TestSuiteRunningCondition    = "Running"
)

// TestSuiteReconciler reconciles a TestSuite object
type TestSuiteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites/trigger;testsuites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestSuiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := logging.FromContext(ctx, "testSuite", req.NamespacedName)
	logger.Debug().Info("starting test suite reconciliation")

	// Get the TestSuite
	logger.Trace().Info("getting test suite")
	var testSuite konfirm.TestSuite
	if err := r.Get(ctx, req.NamespacedName, &testSuite); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug().Info("test suite no longer exists")
		} else {
			logger.Debug().Info("error getting test suite")
		}
		return ctrl.Result{}, err
	}
	logger.Trace().Info("retrieved test suite")

	// Get any controlled Tests
	logger.Trace().Info("getting tests")
	var tests konfirm.TestList
	if err := r.List(ctx, &tests, client.InNamespace(req.Namespace), client.MatchingFields{testIndexKey: req.Name}); err != nil {
		logger.Debug().Info("error getting tests")
		return ctrl.Result{}, err
	}
	logger.Debug().Info("retrieved controlled tests")

	// Handle housekeeping not related to running/analyzing tests
	switch {

	// Test Suite is deleted but Tests exist
	case testSuite.DeletionTimestamp != nil && len(tests.Items) > 0:
		logger.Trace().Info("deleting tests")
		if err := r.deleteTests(ctx, tests.Items); err != nil {
			logger.Info("error getting tests")
			return ctrl.Result{}, err
		}
		logger.Debug().Info("deleted tests")

	// Test Suite is either deleted or not running with no Tests, remove finalizer
	case testSuite.DeletionTimestamp != nil || !testSuite.Status.Phase.IsRunning():
		hasFinalizer := false
		var finalizers []string
		for _, f := range testSuite.Finalizers {
			if f != TestControllerFinalizer {
				finalizers = append(finalizers, f)
				hasFinalizer = true
			}
		}
		if hasFinalizer {
			logger.Trace().Info("removing finalizer")
			orig := testSuite.DeepCopy()
			testSuite.Finalizers = finalizers
			err := r.Client.Patch(ctx, &testSuite, client.MergeFrom(orig))
			if err == nil {
				logger.Debug().Info("removed finalizer")
			} else {
				logger.Info("error removing finalizer")
			}
			return ctrl.Result{}, err
		}

		// No finalizer, fallthrough
		fallthrough

	case testSuite.Status.Phase.IsPending():
		// TODO: If setUp is not nil, ensure it is possible
		orig := testSuite.DeepCopy()
		testSuite.Status.Phase = konfirm.TestSuiteReady
		logger.Trace().Info("patching test suite phase to Ready")
		err := r.Status().Patch(ctx, &testSuite, client.MergeFrom(orig))
		if err == nil {
			logger.Info("test suite Ready")
		} else {
			logger.Info("error setting test suite phase to Ready")
		}
		return ctrl.Result{}, err

	case testSuite.Status.Phase.IsError():
		// TODO: TestSuite is in Error

	}

	// Trigger as needed
	if testSuite.Status.Phase.IsReady() {

		var patch client.Patch
		var trigger string
		var message string
		switch {
		case testSuite.Trigger.NeedsRun:
			logger.Trace().Info("test suite was manually triggered")
			patch = client.MergeFrom(testSuite.DeepCopy())
			trigger = "Manual"
			message = "TestSuite was manually triggered"
			testSuite.Trigger.NeedsRun = false

			// TODO: Scheduled run

			// TODO: Helm run

		}

		// Patch if needed and return
		var err error
		if patch != nil {
			testSuite.Status.Phase = konfirm.TestSuiteRunning
			meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
				Type:               TestSuiteRunningCondition,
				Status:             "True",
				ObservedGeneration: testSuite.Generation,
				Reason:             trigger,
				Message:            message,
			})
			err = r.Patch(ctx, &testSuite, patch)
			if logger := logger.WithValues("trigger", trigger); err == nil {
				logger.Info("running test suite")
			} else {
				logger.Info("error running test suite")
			}
		}
		return ctrl.Result{}, err
	}

	// Test and analyze results
	if testSuite.Status.Phase.IsRunning() {
		// Ensure all previous tests are removed
		// ensure all tests run to completion
		// Analyze results
	}

	return ctrl.Result{}, nil
}

func (r *TestSuiteReconciler) deleteTests(ctx context.Context, tests []konfirm.Test) error {
	errs := ErrorList{}
	return errs.Error()
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestSuiteReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Set up an indexer to reconcile on changes to controlled tests
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &konfirm.Test{}, testIndexKey, func(rawObj client.Object) []string {
		// Get the test and owner
		test := rawObj.(*konfirm.Test)
		owner := metav1.GetControllerOf(test)
		if owner == nil {
			return nil
		}
		// Return the owner if it's a TestSuite
		if owner.APIVersion == konfirm.GroupVersion.String() && owner.Kind == "TestSuite" {
			return []string{owner.Name}
		} else {
			return nil
		}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&konfirm.TestSuite{}).
		Owns(&konfirm.Test{}).
		Complete(r)
}
