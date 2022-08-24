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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
	"time"
)

const (
	testRunIndexKey               = ".metadata.controller"
	TestSuiteControllerFinalizer  = konfirm.GroupName + "/testsuite-controller"
	TestSuiteNeedsRunCondition    = "NeedsRun"
	TestSuiteRunStartedCondition  = "RunStarted"
	TestSuiteErrorCondition       = "HasError"
	testSuiteControllerLoggerName = "testsuite-controller"
)

// TestSuiteReconciler reconciles a TestSuite object
type TestSuiteReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ErrRequeueDelay time.Duration
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

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)
	logger.Debug("starting test suite reconciliation")

	// Get the TestSuite
	logger.Trace("getting test suite")
	var testSuite konfirm.TestSuite
	if err := r.Get(ctx, req.NamespacedName, &testSuite); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug("test suite no longer exists")
			return ctrl.Result{}, nil
		}
		logger.Info("error getting test suite")
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: r.ErrRequeueDelay,
		}, err
	}
	logger.Trace("retrieved test suite")

	// Get any controlled Test Runs
	logger.Trace("getting test runs")
	var testRuns konfirm.TestRunList
	if err := r.List(ctx, &testRuns, client.InNamespace(req.Namespace), client.MatchingFields{testIndexKey: req.Name}); err != nil {
		logger.Debug("error getting test runs")
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: r.ErrRequeueDelay,
		}, err
	}
	logger.Debug("retrieved controlled test runs")

	// Handle deleted test suites
	if testSuite.DeletionTimestamp != nil {
		if len(testRuns.Items) > 0 {
			logger.Trace("deleting test runs")
			if _, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects()); err != nil {
				logger.Info("error getting test runs")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}, err
			}
			logger.Debug("deleted test runs")
		} else {
			logger.Trace("removing finalizer if necessary")
			if patched, err := cleanUp(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); err != nil {
				logger.Info("error removing finalizer")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}, err
			} else if patched {
				logger.Debug("removed finalizer")
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the Finalizer is set
	logger.Trace("ensuring finalizer exists")
	if patched, err := addFinalizer(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); err != nil {
		logger.Info("error adding finalizer")
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: r.ErrRequeueDelay,
		}, err
	} else if patched {
		logger.Debug("added finalizer")
	}

	// Handle deleted test runs
	{
		// The basic strategy here is to iterate over test runs from beginning to end looking
		// for deleted runs. If a deleted run is encountered, the finalizer is removed. We then
		// look from end to beginning for a not-deleted run, removing finalizers, from deleted
		// runs as we go, moving the highest index not-deleted test run into the lowest deleted
		// index. For test suites, this is an unnecessarily optimized strategy. However, test runs
		// will need a similar mechanism, so it make sense to have a single optimized algorithm.
		removeFinalizer := func(testRun *konfirm.TestRun) (err error) {
			logger.Trace("removing finalizer from deleted test run", "testRun", testRun.Name)
			if _, err = removeFinalizer(ctx, r.Client, TestSuiteControllerFinalizer, testRun); err == nil {
				logger.Debug("removed finalizer from deleted test run", "testRun", testRun.Name)
			} else {
				logger.Info("error removing finalizer from deleted test run", "testRun", testRun.Name)
			}
			return
		}
		for i, j := 0, len(testRuns.Items); i < j; i++ {
			if testRuns.Items[i].DeletionTimestamp != nil {
				// Remove the finalizer from Items[i]
				if err := removeFinalizer(&testRuns.Items[i]); err != nil {
					return ctrl.Result{
						Requeue:      true,
						RequeueAfter: r.ErrRequeueDelay,
					}, err
				}
				// Now attempt to replace Items[i] from the end of the slice
				for j--; i < j; j-- {
					if testRuns.Items[j].DeletionTimestamp == nil {
						// Success, move this test tun to Items[i] and continue iterating up
						testRuns.Items[i] = testRuns.Items[j]
						break
					} else {
						// This test tun is also deleted, remove it and continue iterating down
						if err := removeFinalizer(&testRuns.Items[i]); err != nil {
							return ctrl.Result{
								Requeue:      true,
								RequeueAfter: r.ErrRequeueDelay,
							}, err
						}
					}
				}
				testRuns.Items = testRuns.Items[:j]
			}
		}
	}

	// Sort TestRuns by creation time
	sort.Slice(testRuns.Items, func(i, j int) bool {
		return testRuns.Items[i].CreationTimestamp.Before(&testRuns.Items[j].CreationTimestamp)
	})

	// Ensure the correct phase
	if !testSuite.Status.Phase.IsRunning() {

		// If any TestRuns are running, the TestSuite should be running and focused on the oldest TestRun
		for i := range testRuns.Items {
			if !testRuns.Items[i].Status.Phase.IsFinal() {
				orig := testSuite.DeepCopy()
				testSuite.Status.Phase = konfirm.TestSuiteRunning
				testSuite.Status.CurrentTestRun = testRuns.Items[i].Name
				logger.Trace("setting test suite as Running")
				if err := r.Client.Status().Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
					logger.Info("error setting test suite as Pending")
					return ctrl.Result{
						Requeue:      true,
						RequeueAfter: r.ErrRequeueDelay,
					}, err
				}
				logger.Debug("set test suite as Pending")
			}
		}

		// If no phase is set, default to Pending
		if testSuite.Status.Phase == "" {
			// If TestSuite status is empty, make pending
			orig := testSuite.DeepCopy()
			testSuite.Status.Phase = konfirm.TestSuitePending
			logger.Trace("setting test suite as Pending")
			if err := r.Client.Status().Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
				logger.Info("error setting test suite as Pending")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: r.ErrRequeueDelay,
				}, err
			}
			logger.Debug("set test suite as Pending")
		}
	}

	// Phase-specific logic
	if testSuite.Status.Phase.IsRunning() {
		return r.isRunning(ctx, &testSuite, &testRuns)
	} else {
		return r.notRunning(ctx, &testSuite, &testRuns)
	}
}

type testSuiteTrigger struct {
	Reason  string
	Message string
	Patch   client.Patch
}

func (r *TestSuiteReconciler) notRunning(ctx context.Context, testSuite *konfirm.TestSuite, testRuns *konfirm.TestRunList) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)

	// TODO If setUp is specified, ensure it is possible
	if testSuite.Status.Phase == konfirm.TestSuitePending {
		orig := testSuite.DeepCopy()
		testSuite.Status.Phase = konfirm.TestSuiteReady
		logger.Trace("setting test suite as Ready")
		if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
			logger.Info("error setting test suite as Ready")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		}
	}

	// Enforce historyLimit
	if l, m := len(testRuns.Items), int(testSuite.Spec.HistoryLimit); l > m {

		// Clean up
		offset := l - m
		logger.Trace("removing old test runs")
		if _, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects()[offset:]); err != nil {
			logger.Info("error removing old test runs")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		}
		testRuns.Items = testRuns.Items[:offset]
		logger.Debug("old test runs removed")
	}

	// Trigger a test run, if needed
	var trigger *testSuiteTrigger
	switch {

	// Manual trigger occur when the Trigger subresource is modified to NeedsRun == True
	case testSuite.Trigger.NeedsRun:
		logger.Trace("test suite was manually triggered")
		trigger = &testSuiteTrigger{
			Reason:  "Manual",
			Message: "Test suite was manually triggered",
			Patch:   client.MergeFrom(testSuite),
		}
		testSuite.Trigger.NeedsRun = false
		logger.Trace("resetting manual trigger")
		if err := r.Client.Patch(ctx, testSuite, trigger.Patch); err != nil {
			logger.Info("error resetting manual trigger")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		}
		logger.Debug("reset manual trigger")

	case r.needsScheduledRun(ctx, testSuite):
		panic("not implemented")

	case r.needsHelmRun(ctx, testSuite):
		panic("not implemented")

	default:
		return ctrl.Result{}, nil
	}

	// Trigger the test run
	testSuite.Status.Phase = konfirm.TestSuiteRunning
	testSuite.Status.CurrentTestRun = ""
	meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
		Type:               TestSuiteNeedsRunCondition,
		Status:             "True",
		ObservedGeneration: testSuite.Generation,
		Reason:             trigger.Reason,
		Message:            trigger.Message,
	})
	logger.Trace("setting test suite as Running")
	if err := r.Client.Status().Patch(ctx, testSuite, trigger.Patch); err != nil {
		logger.Info("error setting test suite as Running")
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: r.ErrRequeueDelay,
		}, err
	}
	logger.Debug("test suite set as Running")
	r.Recorder.Event(testSuite, "Normal", trigger.Reason, trigger.Message)

	return ctrl.Result{}, nil
}

func (r *TestSuiteReconciler) needsScheduledRun(ctx context.Context, testSuite *konfirm.TestSuite) bool {
	// TODO handle needsScheduledRun
	return false
}

func (r *TestSuiteReconciler) needsHelmRun(ctx context.Context, testSuite *konfirm.TestSuite) bool {
	// TODO handle needsHelmRun
	return false
}

func (r *TestSuiteReconciler) isRunning(ctx context.Context, testSuite *konfirm.TestSuite, testRuns *konfirm.TestRunList) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)

	// Ensure TestRun exists
	var currentRun *konfirm.TestRun
	if name := testSuite.Status.CurrentTestRun; name != "" {
		for i, j := 0, len(testRuns.Items); currentRun == nil && i < j; i++ {
			if testRuns.Items[i].Name == name {
				currentRun = &testRuns.Items[i]
			}
		}
		// If a current run was expected but no longer exists, restart reconcile loop
		if currentRun == nil {
			logger.Info("expected test run is missing", "testRun", name)
			return ctrl.Result{
				Requeue: true,
			}, errors.New("current test run is missing")
		}
	} else {

		// Test Suite was triggered but no Test Run exists yet

		// TODO: Perform setUp if defined

		// Create the test run
		logger.Trace("creating test run")
		yes := true
		currentRun = &konfirm.TestRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: testSuite.Name + "-",
				Namespace:    testSuite.Namespace,
				Annotations: map[string]string{
					konfirm.GroupName + "/template": computeTestRunHash(&testSuite.Spec.Template),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         testSuite.APIVersion,
						Kind:               testSuite.Kind,
						Name:               testSuite.Name,
						UID:                testSuite.UID,
						Controller:         &yes,
						BlockOwnerDeletion: &yes,
					},
				},
				Finalizers: []string{TestSuiteControllerFinalizer},
			},
			Spec: konfirm.TestRunSpec{
				RetentionPolicy: testSuite.Spec.Template.RetentionPolicy,
				Tests:           testSuite.Spec.Template.Tests,
			},
		}
		if err := r.Client.Create(ctx, currentRun); err != nil {
			logger.Info("error creating test run")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		}
		logger.Info("created test run", "testRun", currentRun.Name)
		r.Recorder.Eventf(testSuite, "Normal", "StartedTestRun", "Created test run %s", currentRun.Name)

		// Patch status
		logger.Trace("updating test suite status")
		orig := testSuite.DeepCopy()
		testSuite.Status.CurrentTestRun = currentRun.Name
		conditionReason := "TestRunStarted"
		conditionMessage := fmt.Sprintf("created test run %s", currentRun.Name)
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteNeedsRunCondition,
			Status:             "False",
			ObservedGeneration: testSuite.Generation,
			Reason:             conditionReason,
			Message:            conditionMessage,
		})
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteRunStartedCondition,
			Status:             "True",
			ObservedGeneration: testSuite.Generation,
			Reason:             conditionReason,
			Message:            conditionMessage,
		})
		if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
			logger.Info("error setting status")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: r.ErrRequeueDelay,
			}, err
		}
		logger.Debug("update test suite status")
		return ctrl.Result{}, nil
	}

	// TODO: Get TestRun tatus

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
