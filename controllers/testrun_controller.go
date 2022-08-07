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
	"github.com/raft-tech/konfirm/logging"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"strconv"
	"time"

	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testIndexKey                = ".metadata.controller"
	TestRunControllerFinalizer  = konfirm.GroupName + "/testrun-controller"
	TestRunStartedCondition     = "RunStarted"
	TestRunCompletedCondition   = "RunCompleted"
	testRunControllerLoggerName = "testrun-controller"
)

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testruns,verbs=get;list;watch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testruns/status,verbs=get;patch;update
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testruns/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=tests,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testRunControllerLoggerName)
	logger.Debug("starting test-run reconciliation")

	// Get the TestRun
	logger.Trace("getting test run")
	var testRun konfirm.TestRun
	if err := r.Get(ctx, req.NamespacedName, &testRun); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug("test run no longer exists")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "error getting test run")
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Minute,
		}, err
	}
	logger.Trace("retrieved test run")

	// Unless a test run is completed, it MUST have the testrun-controller finalizer
	if !testRun.Status.Phase.IsFinal() && testRun.DeletionTimestamp == nil {
		logger.Trace("ensuring finalizer exists")
		if patched, err := addFinalizer(ctx, r.Client, TestRunControllerFinalizer, &testRun); err != nil {
			if err = client.IgnoreNotFound(err); err == nil {
				logger.Debug("test run no longer exists")
				return ctrl.Result{}, nil
			}
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Minute,
			}, err
		} else if patched {
			logger.Debug("added finalizer")
		}
	}

	// If TestRun status is empty, make Starting
	if testRun.Status.Phase == "" {
		orig := testRun.DeepCopy()
		testRun.Status.Phase = konfirm.TestRunStarting
		logger.Trace("patching test run phase to Starting")
		if err := r.Client.Status().Patch(ctx, &testRun, client.MergeFrom(orig)); err != nil {
			if err := client.IgnoreNotFound(err); err == nil {
				logger.Debug("test run no longer exists")
				return ctrl.Result{}, nil
			}
			logger.Error(err, "error patching status")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Minute,
			}, err
		}
		logger.Debug("test run Starting")
	}

	// Get any controlled Tests
	logger.Trace("getting tests")
	var tests konfirm.TestList
	if err := r.List(ctx, &tests, client.InNamespace(req.Namespace), client.MatchingFields{testIndexKey: req.Name}); err != nil {
		logger.Debug("error getting tests")
		return ctrl.Result{}, err
	}
	logger.Debug("retrieved controlled tests")

	// Handle deleted test runs
	if testRun.DeletionTimestamp != nil {
		if len(tests.Items) > 0 {
			logger.Trace("deleting tests")
			if _, err := cleanUpAll(ctx, r.Client, TestRunControllerFinalizer, tests.GetObjects()); err != nil {
				logger.Error(err, "error removing tests")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Minute,
				}, err
			}
			logger.Debug("deleted tests")
		} else {
			logger.Trace("removing finalizer if present")
			if patched, err := removeFinalizer(ctx, r.Client, TestRunControllerFinalizer, &testRun); err != nil {
				logger.Error(err, "error removing finalizer")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Minute,
				}, err
			} else if patched {
				logger.Debug("finalizer removed")
			} else {
				logger.Debug("nothing to do, test run is deleted")
			}
		}
		return ctrl.Result{}, nil
	}

	// Phase-specific logic
	if phase := testRun.Status.Phase; phase.IsFinal() {
		return r.isComplete(ctx, &testRun, tests.Items)
	} else {
		return r.isRunning(ctx, &testRun, tests.Items)
	}
}

func (r *TestRunReconciler) isRunning(ctx context.Context, testRun *konfirm.TestRun, tests []konfirm.Test) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testRunControllerLoggerName)

	// Map tests
	maxIndex := len(testRun.Spec.Tests) - 1
	mapped := make([]*konfirm.Test, maxIndex+1)
	for _, t := range tests {

		logger := logger.WithValues("test", client.ObjectKeyFromObject(&t))

		var err error
		if l := len(t.GenerateName); l >= 4 {
			idxStr := t.GenerateName[l-2 : l-1]
			if i, e := strconv.Atoi(idxStr); e == nil {
				if i <= maxIndex {
					mapped[i] = &t
				} else {
					err = errors.New("test index greater than max")
				}
			} else {
				err = errors.New("non-integer test index")
			}
		} else {
			err = errors.New("test name does not meet min length")
		}

		if err != nil {
			logger.Error(err, "invalid test name")
			if _, err = cleanUp(ctx, r.Client, TestRunControllerFinalizer, &t); err != nil {
				logger.Error(err, "error cleaning up invalidly named test")
				return ctrl.Result{}, err
			}
		}
	}

	// Ensure each test exists, track completions
	testRun.Status.Results = []konfirm.TestResult{}
	passing := true
	var completions int
	var running int
	for i, test := range mapped {

		logger := logger.WithValues("testIndex", i)

		// Create Test if it does not exist
		if test == nil {
			template := &testRun.Spec.Tests[i]
			yes := true
			test = &konfirm.Test{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-%d-", testRun.Name, i),
					Namespace:    testRun.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         testRun.APIVersion,
							Kind:               testRun.Kind,
							Name:               testRun.Name,
							UID:                testRun.UID,
							Controller:         &yes,
							BlockOwnerDeletion: &yes,
						},
					},
				},
				Spec: konfirm.TestSpec{
					RetentionPolicy: testRun.Spec.RetentionPolicy,
					Template:        template.Template,
				},
			}
			if template.RetentionPolicy != "" {
				test.Spec.RetentionPolicy = template.RetentionPolicy
			}
			logger.Trace("creating test")
			if err := r.Client.Create(ctx, test); err != nil {
				logger.Error(err, "error creating test")
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Minute,
				}, err
			}
			logger.Debug("created test")
		} else {
			// Evaluate test phase
			if test.Status.Phase.IsFinal() {
				completions++
				result := konfirm.TestResult{
					Description: testRun.Spec.Tests[i].Description,
				}
				if test.Status.Phase.IsSuccess() {
					result.Passed = true
				} else {
					passing = false
				}
				testRun.Status.Results = append(testRun.Status.Results, result)
			} else {
				running++
			}
		}
	}

	// Set the current status
	if len(mapped) == completions {
		logger.Info("test run Complete")
		meta.SetStatusCondition(&testRun.Status.Conditions, metav1.Condition{
			Type:               TestRunStartedCondition,
			Status:             "False",
			ObservedGeneration: testRun.Generation,
			Reason:             "TestRunCompleted",
			Message:            "Test run has completed",
		})
		meta.SetStatusCondition(&testRun.Status.Conditions, metav1.Condition{
			Type:               TestRunCompletedCondition,
			Status:             "False",
			ObservedGeneration: testRun.Generation,
			Reason:             "TestRunCompleted",
			Message:            "Test run has completed",
		})
		if passing {
			testRun.Status.Phase = konfirm.TestRunPassed
		} else {
			testRun.Status.Phase = konfirm.TestRunFailed
		}
	} else if testRun.Status.Phase == konfirm.TestRunStarting {
		logger.Info("test run Running")
		testRun.Status.Phase = konfirm.TestRunRunning
		meta.SetStatusCondition(&testRun.Status.Conditions, metav1.Condition{
			Type:               TestRunStartedCondition,
			Status:             "True",
			ObservedGeneration: testRun.Generation,
			Reason:             "TestRunStarted",
			Message:            "Test run has started",
		})
	}

	// Update it!
	logger.Trace("updating status")
	if err := r.Client.Status().Update(ctx, testRun); err == nil {
		logger.Debug("status updated")
	} else {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug("test run no longer exists")
		} else {
			logger.Error(err, "error updating status")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Minute,
			}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) isComplete(ctx context.Context, testRun *konfirm.TestRun, tests []konfirm.Test) (ctrl.Result, error) {
	_ = logging.FromContextWithName(ctx, testSuiteControllerLoggerName)
	// TODO Handle retention policy/clean up
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Set up an indexer to reconcile on changes to controlled tests
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &konfirm.Test{}, testIndexKey, func(rawObj client.Object) []string {
		// Get the test and owner
		test := rawObj.(*konfirm.Test)
		owner := metav1.GetControllerOf(test)
		if owner == nil {
			return nil
		}
		// Return the owner if it's a TestRun
		if owner.APIVersion == konfirm.GroupVersion.String() && owner.Kind == "TestRun" {
			return []string{owner.Name}
		} else {
			return nil
		}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&konfirm.TestRun{}).
		Owns(&konfirm.Test{}).
		Complete(r)
}
