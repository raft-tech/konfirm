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
	"errors"
	"fmt"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	testRunIndexKey                       = ".metadata.controller"
	TestSuiteControllerFinalizer          = konfirm.GroupName + "/testsuite-controller"
	TestSuiteRunStartedCondition          = "RunStarted"
	TestSuiteHasPreviousTestRunsCondition = "HasPreviousTestRun"
	TestSuiteErrorCondition               = "HasError"
	testSuiteControllerLoggerName         = "testsuite-controller"
)

// TestSuiteReconciler reconciles a TestSuite object
type TestSuiteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites,verbs=get;list;watch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites/trigger;testsuites/status,verbs=get;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testruns,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestSuiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// FIXME requeue all error returns

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)
	logger.Debug("starting test suite reconciliation")

	// Get the TestSuite
	logger.Trace("getting test suite")
	var testSuite konfirm.TestSuite
	if err := r.Get(ctx, req.NamespacedName, &testSuite); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug("test suite no longer exists")
		} else {
			logger.Info("error getting test suite")
		}
		return ctrl.Result{}, err
	}
	logger.Trace("retrieved test suite")

	// If TestSuite status is empty, make pending
	if testSuite.Status.Phase == "" {
		orig := testSuite.DeepCopy()
		testSuite.Status.Phase = konfirm.TestSuitePending
		logger.Trace("patching test suite phase to Pending")
		if err := r.Client.Status().Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
			logger.Info("error patching status")
			return ctrl.Result{}, err
		}
		logger.Debug("test suite Pending")
	}

	// Get any controlled Test Runs
	logger.Trace("getting test runs")
	var testRuns konfirm.TestRunList
	if err := r.List(ctx, &testRuns, client.InNamespace(req.Namespace), client.MatchingFields{testIndexKey: req.Name}); err != nil {
		logger.Debug("error getting test runs")
		return ctrl.Result{}, err
	}
	logger.Debug("retrieved controlled test runs")

	// If controlled Tests exists, the finalizer must be set
	if n := len(testRuns.Items); n > 0 {
		logger.Trace("ensuring finalizer exists")
		if patched, err := addFinalizer(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); patched && err == nil {
			logger.Debug("added finalizer")
		} else if err != nil {
			logger.Info("error adding finalizer")
			return ctrl.Result{}, err
		}
	} else if !testSuite.Status.Phase.IsRunning() {
		// No controlled Tests, so the finalizer should be cleared if not running
		// TODO This should be moved to the ready and error states
		logger.Trace("removing finalizer if necessary")
		if patched, err := removeFinalizer(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); patched && err == nil {
			logger.Debug("removed finalizer")
		} else if err != nil {
			if apierrors.IsNotFound(err) {
				err = nil
				logger.Debug("test suite no longer exists")
			} else {
				logger.Info("error removing finalizer")
			}
			return ctrl.Result{}, err
		}
	}

	// Handle deleted test suites
	if testSuite.DeletionTimestamp != nil {
		if len(testRuns.Items) > 0 {
			logger.Trace("deleting test runs")
			if _, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects()); err != nil {
				logger.Info("error getting tests")
				return ctrl.Result{}, err
			}
			logger.Debug("deleted test runs")
		} else {
			logger.Trace("removing finalizer if necessary")
			if patched, err := cleanUp(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); err != nil {
				logger.Info("error removing finalizer")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Minute,
				}, err
			} else if patched {
				logger.Debug("removed finalizer")
			}
			logger.Debug("test suite deleted, nothing to do")
		}
		return ctrl.Result{}, nil
	}

	// Handled changed Tests
	for i := range testRuns.Items {
		if !reflect.DeepEqual(testRuns.Items[i].Spec.Tests, testSuite.Spec.Tests) {
			logger := logger.WithValues("testRun", testRuns.Items[i].Name)
			logger.Trace("cleaning up out-of-date test run")
			if _, err := cleanUp(ctx, r.Client, TestSuiteControllerFinalizer, &testRuns.Items[i]); err != nil {
				logger.Info("error cleaning up out-of-date test run")
				return ctrl.Result{}, err
			}
			logger.Debug("out-of-date test run removed")
		}
	}

	// Phase-specific logic
	switch testSuite.Status.Phase {

	case konfirm.TestSuitePending:
		return r.isPending(ctx, &testSuite)

	case konfirm.TestSuiteReady:
		return r.isReady(ctx, &testSuite)

	case konfirm.TestSuiteRunning:
		return r.isRunning(ctx, &testSuite, &testRuns)

	case konfirm.TestSuiteError:
		return r.isError(ctx, &testSuite, &testRuns)

	default:
		return ctrl.Result{}, errors.New(fmt.Sprintf("unrecognized phase: %s", testSuite.Status.Phase))
	}
}

func (r *TestSuiteReconciler) isPending(ctx context.Context, testSuite *konfirm.TestSuite) (ctrl.Result, error) {
	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)
	// TODO: If setUp is not nil, ensure it is possible
	orig := testSuite.DeepCopy()
	testSuite.Status.Phase = konfirm.TestSuiteReady
	logger.Trace("patching test suite phase to Ready")
	err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig))
	if err == nil {
		logger.Info("test suite Ready")
	} else {
		logger.Info("error setting test suite phase to Ready")
	}
	return ctrl.Result{}, err
}

func (r *TestSuiteReconciler) isReady(ctx context.Context, testSuite *konfirm.TestSuite) (ctrl.Result, error) {
	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)

	var patch client.Patch
	var trigger string
	var message string
	switch {
	case testSuite.Trigger.NeedsRun:
		logger.Trace("test suite was manually triggered")
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

		// First reset trigger
		logger.Trace("resetting trigger", "trigger", trigger)
		if err = r.Client.Patch(ctx, testSuite, patch); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Debug("test suite no longer exists")
				return ctrl.Result{}, nil
			} else {
				logger.Info("error resetting trigger", "trigger", trigger)
				return ctrl.Result{}, err
			}
		}
		logger.Debug("trigger reset", "trigger", trigger)

		// Then update status
		orig := testSuite.DeepCopy()
		testSuite.Status.Phase = konfirm.TestSuiteRunning
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteRunStartedCondition,
			Status:             "False",
			ObservedGeneration: testSuite.Generation,
			Reason:             trigger,
			Message:            message,
		})
		logger.Trace("patching test suite status to Running")
		switch err = r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); true {
		case apierrors.IsNotFound(err):
			logger.Debug("test suite no longer exists")
			err = nil
		case err != nil:
			logger.Info("error setting test suit to Running")
		case err == nil:
			logger.Info("test suite Running")
		}
	} else {

	}
	return ctrl.Result{}, err
}

func (r *TestSuiteReconciler) isRunning(ctx context.Context, testSuite *konfirm.TestSuite, testRuns *konfirm.TestRunList) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)

	// Ensure test run can start
	if e, ok := getCondition(TestSuiteRunStartedCondition, testSuite.Status.Conditions); !ok {
		logger.Debug("test suite is running but has no Running condition")
		orig := testSuite.DeepCopy()
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteRunStartedCondition,
			Status:             "False",
			ObservedGeneration: testSuite.Generation,
			Reason:             "Unknown",
			Message:            "test suite was in Running phase but had no RunningCondition",
		})
		if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err == nil {
			logger.Info("running condition set")
		} else {
			logger.Info("error setting running condition")
			return ctrl.Result{}, err
		}
	} else if e.Status == "False" {

		// Ensure all previous testRuns are removed
		if n := len(testRuns.Items); n > 0 {
			logger.Trace("removing previous test run")
			change, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects())
			if err == nil {
				if change {
					logger.Info("removed previous test run")
				} else {
					logger.Info("waiting on previous test run to be removed")
				}
			} else {
				logger.Info("error removing previous test run")
			}
			return ctrl.Result{}, err
		}

		// TODO Ensure Set Up is done

		// Mark test as ready
		orig := testSuite.DeepCopy()
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteRunStartedCondition,
			Status:             "True",
			ObservedGeneration: testSuite.Generation,
			Reason:             "PreRunComplete",
			Message:            "All prerun tasks have completed",
		})
		if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err == nil {
			logger.Info("test run started")
			r.Recorder.Event(testSuite, "Normal", "RunStarted", "Test run is starting")
		} else {
			logger.Info("error starting test run")
			return ctrl.Result{}, err
		}
	}

	// Start a test run
	var testRun *konfirm.TestRun
	switch l := len(testRuns.Items); true {
	case l > 1:
		logger.Trace("clearing excess test runs")
		if _, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects()); err != nil {
			logger.Info("error removing excess test runs")
			return ctrl.Result{}, err
		}
		logger.Info("multiple test runs found, cleared to restart")
	case l == 1:
		testRun = &testRuns.Items[0]
	default:
		yes := true
		testRun = &konfirm.TestRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: testSuite.Name + "-",
				Namespace:    testSuite.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         konfirm.GroupVersion.String(),
						Kind:               "TestSuite",
						Name:               testSuite.Name,
						UID:                testSuite.UID,
						Controller:         &yes,
						BlockOwnerDeletion: &yes,
					},
				},
				Finalizers: []string{
					TestSuiteControllerFinalizer,
				},
			},
			Spec: konfirm.TestRunSpec{
				RetentionPolicy: testSuite.Spec.RetentionPolicy,
				Tests:           testSuite.Spec.Tests,
			},
		}
		logger.Trace("creating test run")
		err := r.Client.Create(ctx, testRun)
		if err == nil {
			logger.Info("test run created")
		} else {
			logger.Info("error creating test run")
		}
		return ctrl.Result{}, err
	}

	// TODO Handle finished test runs

	return ctrl.Result{}, nil
}

func (r *TestSuiteReconciler) isError(ctx context.Context, testSuite *konfirm.TestSuite, testRuns *konfirm.TestRunList) (ctrl.Result, error) {
	_ = logging.FromContextWithName(ctx, testSuiteControllerLoggerName)
	panic("not implemented")
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestSuiteReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Set up an indexer to reconcile on changes to controlled tests
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &konfirm.TestRun{}, testRunIndexKey, func(rawObj client.Object) []string {
		// Get the test and owner
		test := rawObj.(*konfirm.TestRun)
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
		Owns(&konfirm.TestRun{}).
		Complete(r)
}
