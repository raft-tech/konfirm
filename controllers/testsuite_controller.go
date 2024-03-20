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
	"sort"
	"strings"
	"time"

	"github.com/raft-tech/konfirm/internal/setup"
	"github.com/raft-tech/konfirm/internal/utils"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/builder"

	"github.com/prometheus/client_golang/prometheus"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/internal/impersonate"
	"github.com/raft-tech/konfirm/logging"
	"github.com/robfig/cron/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	testRunIndexKey                    = ".metadata.controller"
	TestSuiteControllerFinalizer       = konfirm.GroupName + "/testsuite-controller"
	TestSuiteScheduleAnnotation        = konfirm.GroupName + "/active-schedule"
	TestSuiteSetUpAnnotation           = konfirm.GroupName + "/setup-helm-release"
	TestSuiteLastHelmReleaseAnnotation = konfirm.GroupName + "/last-helm-release"
	TestSuiteHelmTriggerLabel          = konfirm.GroupName + "/helm-trigger"
	TestSuiteNeedsRunCondition         = "NeedsRun"
	TestSuiteIsStartingCondition       = "IsStarting"
	TestSuiteRunStartedCondition       = "RunStarted"
	TestSuiteHasScheduleCondition      = "HasSchedule"
	TestSuiteSetUpCondition            = "SetUpCompleted"
	TestSuiteTearDownCondition         = "TearDownCompleted"
	TestSuiteErrorCondition            = "HasError"
	testSuiteControllerLoggerName      = "testsuite-controller"
)

var (
	ErrUnhandledErrorDuringSetUp error = errors.New("an unexpected error occurred")
)

var (
	suitePassing = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: "testsuites",
		Name:      "passing",
		Help:      "Test Suites are passing",
	}, []string{"test_namespace", "test_suite"})
	suiteFailing = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: "testsuites",
		Name:      "failing",
		Help:      "Test Suites are failing",
	}, []string{"test_namespace", "test_suite"})
	suiteRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: "testsuites",
		Name:      "runs",
		Help:      "Number of times Test Suites have run",
	}, []string{"test_namespace", "test_suite"})
)

func init() {
	metrics.Registry.MustRegister(suitePassing, suiteFailing, suiteRuns)
}

// TestSuiteReconciler reconciles a TestSuite object
type TestSuiteReconciler struct {
	impersonate.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ErrRequeueDelay time.Duration
	CronParser      func(string) (cron.Schedule, error)
	Clock           clock.PassiveClock
	DataDir         string
}

type testSuiteTrigger struct {
	Reason  string
	Message string
	Patch   client.Patch
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testsuites/trigger;testsuites/status,verbs=get;patch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=testruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *TestSuiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName).WithValues("namespace", req.Namespace, "testSuite", req.Name)
	logger.Debug("starting test suite reconciliation")

	// Get the TestSuite
	logger.Trace("getting test suite")
	var testSuite konfirm.TestSuite
	if err := r.Get(ctx, req.NamespacedName, &testSuite); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug("test suite no longer exists")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "error getting test suite")
		return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
	}
	logger.Trace("retrieved test suite")

	// Ensure the namespaced counters are initialized
	suiteRuns.WithLabelValues(testSuite.Namespace, testSuite.Name)
	suitePassing.WithLabelValues(testSuite.Namespace, testSuite.Name)
	suiteFailing.WithLabelValues(testSuite.Namespace, testSuite.Name)

	// Get any controlled Test Runs
	logger.Trace("getting test runs")
	var testRuns konfirm.TestRunList
	if err := r.List(ctx, &testRuns, client.InNamespace(req.Namespace), client.MatchingFields{testIndexKey: req.Name}); err != nil {
		logger.Error(err, "error getting test runs")
		return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
	}
	logger.Debug("retrieved controlled test runs")

	// Handle deleted test suites
	if testSuite.DeletionTimestamp != nil {
		if len(testRuns.Items) > 0 {
			logger.Trace("deleting test runs")
			if _, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects()); err != nil {
				logger.Error(err, "error getting test runs")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("deleted test runs")
		} else {
			logger.Trace("removing finalizer if necessary")
			if patched, err := cleanUp(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); err != nil {
				logger.Error(err, "error removing finalizer")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			} else if patched {
				logger.Debug("removed finalizer")
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the Finalizer is set
	logger.Trace("ensuring finalizer exists")
	if patched, err := addFinalizer(ctx, r.Client, TestSuiteControllerFinalizer, &testSuite); err != nil {
		logger.Error(err, "error adding finalizer")
		return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
	} else if patched {
		logger.Debug("added finalizer")
	}

	// Handle errors

	// Handle changes to scheduling
	if sched := testSuite.Spec.When.Schedule; sched != "" {
		if sched != testSuite.Annotations[TestSuiteScheduleAnnotation] {
			if testSuite.Status.NextRun != nil {
				orig := testSuite.DeepCopy()
				testSuite.Status.NextRun = nil
				logger.Trace("clearing next run")
				if err := r.Status().Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
					logger.Error(err, "error clearing next run")
				}
				logger.Debug("cleared next run")
			}
		}
	} else {

		orig := testSuite.DeepCopy()

		if _, ok := testSuite.Annotations[TestSuiteScheduleAnnotation]; ok {
			delete(testSuite.Annotations, TestSuiteScheduleAnnotation)
			logger.Trace("removing active schedule")
			if err := r.Client.Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error removing active schedule")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("removed active schedule")
		}

		testSuite.Status.NextRun = nil
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteHasScheduleCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: testSuite.Generation,
			Reason:             "ScheduleNotDefined",
			Message:            "schedule is not defined",
		})
		logger.Trace("removing schedule")
		if err := r.Status().Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
			logger.Error(err, "error removing schedule")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		logger.Debug("removed schedule")
	}

	// Handle changes to Helm triggers
	if release := testSuite.Spec.When.HelmRelease; release != "" {
		if strings.Index(release, ".") == -1 {
			release = testSuite.Namespace + "." + release
		}
		if testSuite.ObjectMeta.Labels[TestSuiteHelmTriggerLabel] != release {
			orig := testSuite.DeepCopy()
			if testSuite.Labels == nil {
				testSuite.Labels = make(map[string]string)
			}
			testSuite.Labels[TestSuiteHelmTriggerLabel] = release
			delete(testSuite.Annotations, "TestSuiteLastHelmReleaseAnnotation")
			logger.Trace("patching helm metadata")
			if err := r.Patch(ctx, &testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error patching helm metadata")
			}
			logger.Debug("patched helm metadata")
		}
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
				logger.Error(err, "error removing finalizer from deleted test run", "testRun", testRun.Name)
			}
			return
		}
		for i, j := 0, len(testRuns.Items); i < j; i++ {
			if testRuns.Items[i].DeletionTimestamp != nil {
				// Remove the finalizer from Items[i]
				if err := removeFinalizer(&testRuns.Items[i]); err != nil {
					return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
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
							return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
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
					logger.Error(err, "error setting test suite as Pending")
					return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
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
				logger.Error(err, "error setting test suite as Pending")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("set test suite as Pending")
		}
	}

	// Phase-specific logic
	if p := testSuite.Status.Phase; p.IsRunning() || p.IsStarting() || p.IsError() {
		return r.isRunning(ctx, &testSuite, &testRuns)
	} else {
		return r.notRunning(ctx, &testSuite, &testRuns)
	}
}

func (r *TestSuiteReconciler) notRunning(ctx context.Context, testSuite *konfirm.TestSuite, testRuns *konfirm.TestRunList) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName)

	// Make ready to start
	if testSuite.Status.Phase == konfirm.TestSuitePending {
		orig := testSuite.DeepCopy()
		testSuite.Status.Phase = konfirm.TestSuiteReady
		logger.Trace("setting test suite as Ready")
		if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
			logger.Error(err, "error setting test suite as Ready")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
	}

	// Enforce historyLimit
	if l, m := len(testRuns.Items), int(testSuite.Spec.HistoryLimit); l > m {

		// Clean up
		offset := l - m
		logger.Trace("removing old test runs")
		if _, err := cleanUpAll(ctx, r.Client, TestSuiteControllerFinalizer, testRuns.GetObjects()[:offset]); err != nil {
			logger.Error(err, "error removing old test runs")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		testRuns.Items = testRuns.Items[offset:]
		logger.Debug("old test runs removed")
	}

	// Ensure active schedule is set
	var schedule cron.Schedule
	if when := testSuite.Spec.When.Schedule; when != "" {

		// Annotate with the current schedule
		if testSuite.Annotations[TestSuiteScheduleAnnotation] != when {
			orig := testSuite.DeepCopy()
			if testSuite.Annotations == nil {
				testSuite.Annotations = make(map[string]string)
			}
			testSuite.Annotations[TestSuiteScheduleAnnotation] = testSuite.Spec.When.Schedule
			logger.Trace("setting active schedule")
			if err := r.Client.Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error setting active schedule")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("set active schedule")
		}

		// Parse the schedule and set next run
		orig := testSuite.DeepCopy()
		if sched, err := r.CronParser(when); err == nil {
			schedule = sched
			if testSuite.Status.NextRun == nil {
				next := metav1.NewTime(schedule.Next(r.Clock.Now()))
				testSuite.Status.NextRun = &next
				meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
					Type:               TestSuiteHasScheduleCondition,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: testSuite.Generation,
					Reason:             "ScheduleSet",
					Message:            "schedule is set",
				})
				logger.Trace("setting next run", "nextRun", next.String())
				if err = r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
					logger.Error(err, "error setting next run")
					return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
				}
				logger.Debug("set next run", "nextRun", next.String())
			}
		} else {
			// Schedule is defined but not valid
			testSuite.Status.NextRun = nil
			meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
				Type:               TestSuiteHasScheduleCondition,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: testSuite.Generation,
				Reason:             "InvalidSchedule",
				Message:            "schedule is not a valid cron value",
			})
			logger.Trace("setting schedule as invalid")
			if err = r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error setting schedule as invalid")
			} else {
				logger.Debug("schedule set as invalid")
			}
		}
	}

	// Determine the current Helm release
	var currentHelmRelase *HelmReleaseMeta
	if releaseName := testSuite.Spec.When.HelmRelease; releaseName != "" {

		release := types.NamespacedName{
			Namespace: testSuite.Namespace,
			Name:      releaseName,
		}

		// If the release is in another namespace, it must be exported to the test suite's namespace to be observed
		ok := true
		if policyName := MakeNamespacedName(releaseName); policyName.Namespace != "" {
			policy := konfirm.HelmPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName.Name,
					Namespace: policyName.Namespace,
				},
			}
			if err := r.Get(ctx, client.ObjectKeyFromObject(&policy), &policy); err == nil {
				ok = slices.Contains(policy.Spec.ExportTo, testSuite.Namespace)
			} else {
				if err = client.IgnoreNotFound(err); err != nil {
					logger.Error(err, "error retrieving helm policy")
					return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
				} else {
					logger.Info("ignoring unexported helm release trigger from another namespace")
					ok = false
				}
			}
		}

		if ok {
			logger.Trace("listing matching helm releases")
			releases := v1.SecretList{}
			//matchingFields := client.MatchingFields(map[string]string{"type": HelmSecretType})
			matchingLabels := client.MatchingLabels(map[string]string{"owner": "Helm", "name": release.Name, "status": "deployed"})
			if err := r.List(ctx, &releases, client.InNamespace(release.Namespace), matchingLabels); err != nil {
				logger.Error(err, "error listing Helm release secrets")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("retrieved matching helm releases")
			for i := range releases.Items {
				if parsed, ok := ParseHelmReleaseSecret(&releases.Items[i]); ok {
					if currentHelmRelase == nil || currentHelmRelase.Version < parsed.Version {
						currentHelmRelase = parsed
					}
				}
			}
		}
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
			Patch:   client.MergeFrom(testSuite.DeepCopy()),
		}
		testSuite.Trigger.NeedsRun = false
		logger.Trace("resetting manual trigger")
		if err := r.Client.Patch(ctx, testSuite, trigger.Patch); err != nil {
			logger.Error(err, "error resetting manual trigger")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		logger.Debug("reset manual trigger")

	case currentHelmRelase != nil && currentHelmRelase.VersionString != testSuite.Annotations[TestSuiteLastHelmReleaseAnnotation]:
		logger.Trace("test suite was triggered by a Helm release")
		trigger = &testSuiteTrigger{
			Reason:  "Helm",
			Message: "Test suite was triggered by a Helm release",
			Patch:   client.MergeFrom(testSuite.DeepCopy()),
		}
		if testSuite.Annotations == nil {
			testSuite.Annotations = make(map[string]string)
		}
		testSuite.Annotations[TestSuiteLastHelmReleaseAnnotation] = testSuite.Annotations[currentHelmRelase.VersionString]
		logger.Trace("annotating last helm release")
		if err := r.Patch(ctx, testSuite, trigger.Patch); err != nil {
			logger.Error(err, "error setting helm release annotation")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		logger.Debug("annotated last helm release")

	case testSuite.Status.NextRun != nil:
		if until := testSuite.Status.NextRun.Sub(r.Clock.Now()); until > 0 {
			logger.Debug("requeuing for next scheduled run")
			return ctrl.Result{RequeueAfter: until}, nil
		} else if until < -10*time.Second {
			orig := testSuite.DeepCopy()
			next := metav1.NewTime(schedule.Next(r.Clock.Now()))
			testSuite.Status.NextRun = &next
			logger.Trace("setting next run")
			if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error setting next run")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("set next run")
			logger.Info("missed scheduled run, requeueing")
			return ctrl.Result{RequeueAfter: next.Sub(r.Clock.Now())}, nil
		} else {
			logger.Trace("test run triggered by schedule")
			trigger = &testSuiteTrigger{
				Reason:  "Scheduled",
				Message: "test run triggered by schedule '" + testSuite.Spec.When.Schedule + "'",
				Patch:   client.MergeFrom(testSuite.DeepCopy()),
			}
		}

	default:
		logger.Info("reconcile completed")
		return ctrl.Result{}, nil
	}

	// Trigger the test run
	testSuite.Status.Phase = konfirm.TestSuiteStarting
	testSuite.Status.CurrentTestRun = ""
	meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
		Type:               TestSuiteNeedsRunCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: testSuite.Generation,
		Reason:             trigger.Reason,
		Message:            trigger.Message,
	})
	logger.Trace("setting test suite as Starting")
	if err := r.Status().Patch(ctx, testSuite, trigger.Patch); err != nil {
		logger.Error(err, "error setting test suite as Starting")
		return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
	}
	logger.Debug("test suite set as Starting")
	r.Recorder.Event(testSuite, "Normal", "TestSuiteTriggered", trigger.Message)

	return ctrl.Result{}, nil
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
	} else { // Test Suite was triggered but no Test Run exists yet

		// Perform SetUp if defined
		if ok, err := r.doSetUp(ctx, testSuite); err != nil {

			// If set up encountered an unexpected error, then retry later
			if errors.Is(err, ErrUnhandledErrorDuringSetUp) {
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}

			// If set up fails, set status to Error
			if testSuite.Status.Phase != konfirm.TestSuiteError {
				orig := testSuite.DeepCopy()
				testSuite.Status.Phase = konfirm.TestSuiteError
				meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
					Type:               TestSuiteErrorCondition,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: testSuite.Generation,
					Reason:             "SetUpFailed",
					Message:            err.Error(),
				})
				logger.Trace("setting Error status")
				if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err == nil {
					logger.Debug("set Error status")
				}
			} else {
				// Exponential back off up to 5 sec to 5 min
				if c, ok := getCondition(TestSuiteErrorCondition, testSuite.Status.Conditions); ok && c.Message == err.Error() {
					since := time.Now().Sub(c.LastTransitionTime.Time).Seconds()
					var backoff time.Duration
					switch {
					case since < 5.0:
						backoff = 5 * time.Second
					case since < 10.0:
						backoff = 10 * time.Second
					case since < 20.0:
						backoff = 20 * time.Second
					case since < 40.0:
						backoff = 40 * time.Second
					case since < 80.0:
						backoff = 80 * time.Second
					case since < 160.0:
						backoff = 160 * time.Second
					default:
						backoff = 300 * time.Second
					}
					logger.Info("backing off due to error state", "retryAfter", fmt.Sprintf("%.0fs", backoff.Seconds()))
					return ctrl.Result{RequeueAfter: backoff}, nil
				} else {
					orig := testSuite.DeepCopy()
					meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
						Type:               TestSuiteErrorCondition,
						Status:             metav1.ConditionTrue,
						ObservedGeneration: testSuite.Generation,
						Reason:             "SetUpFailed",
						Message:            err.Error(),
					})
					logger.Trace("updating Error condition")
					if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
						logger.Error(err, "error updating TestSuite error condition")
						return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
					}
					logger.Debug("updated Error condition")
					return ctrl.Result{}, nil
				}
			}
		} else if !ok {
			// Reconciliation will be triggered when set up completes, requeue for 5 min later to catch race conditions
			return ctrl.Result{RequeueAfter: 300 * time.Second}, nil
		}

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
				RunAs:           testSuite.Spec.RunAs,
				Tests:           testSuite.Spec.Template.Tests,
			},
		}
		if err := r.Client.Create(ctx, currentRun); err != nil {
			logger.Error(err, "error creating test run")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
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
			Status:             metav1.ConditionFalse,
			ObservedGeneration: testSuite.Generation,
			Reason:             conditionReason,
			Message:            conditionMessage,
		})
		meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
			Type:               TestSuiteRunStartedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: testSuite.Generation,
			Reason:             conditionReason,
			Message:            conditionMessage,
		})
		if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
			logger.Error(err, "error setting status")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		logger.Debug("update test suite status")
		return ctrl.Result{}, nil
	}

	// Handle finished test runs
	if phase := currentRun.Status.Phase; phase == konfirm.TestRunPassed || phase == konfirm.TestRunFailed {

		if c := meta.FindStatusCondition(testSuite.Status.Conditions, TestSuiteRunStartedCondition); c == nil || c.Status == metav1.ConditionTrue {

			// Update test suite status
			logger.Trace("updating RunStarted condition")
			orig := testSuite.DeepCopy()
			testSuite.Status.CurrentTestRun = ""
			meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
				Type:               TestSuiteRunStartedCondition,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: testSuite.Generation,
				Reason:             "TestRunCompleted",
				Message:            fmt.Sprintf("test run %s completed", currentRun.Name),
			})
			if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error setting status")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("updated RunStarted condition")
			suiteRuns.WithLabelValues(testSuite.Namespace, testSuite.Name).Inc()

			// Record an event and update metrics
			if phase == konfirm.TestRunPassed {
				suitePassing.WithLabelValues(testSuite.Namespace, testSuite.Name).Set(1)
				suiteFailing.WithLabelValues(testSuite.Namespace, testSuite.Name).Set(0)
				r.Recorder.Eventf(testSuite, "Normal", "TestRunPassed", "test run %s passed", currentRun.Name)
			} else {
				suiteFailing.WithLabelValues(testSuite.Namespace, testSuite.Name).Set(1)
				suitePassing.WithLabelValues(testSuite.Namespace, testSuite.Name).Set(0)
				r.Recorder.Eventf(testSuite, "Warning", "TestRunFailed", "test run %s failed", currentRun.Name)
			}
		}

		if done, err := r.doTearDown(ctx, testSuite); err != nil {
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		} else if !done {
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}

		logger.Trace("updating status")
		orig := testSuite.DeepCopy()
		testSuite.Status.Phase = konfirm.TestSuiteReady
		if err := r.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
			logger.Error(err, "error setting status")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		logger.Debug("updated status")
	}

	return ctrl.Result{}, nil
}

// doSetUp performs any required setup and returns true when all set up
// is complete. An error is returned if set up cannot be performed.
func (r *TestSuiteReconciler) doSetUp(ctx context.Context, testSuite *konfirm.TestSuite) (bool, error) {

	config := &testSuite.Spec.SetUp.Helm

	// Return early if no set up
	if config.SecretName == "" {
		return true, nil
	}

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName, "namespace", testSuite.Namespace, "testSuite", testSuite.Name)

	// Check if teardown is in progress
	if c := meta.FindStatusCondition(testSuite.Status.Conditions, TestSuiteTearDownCondition); c != nil {
		if c.Status == metav1.ConditionFalse {
			return false, nil
		}
	}

	// Check if set up is complete
	if c := meta.FindStatusCondition(testSuite.Status.Conditions, TestSuiteSetUpCondition); c != nil {
		return c.Status == metav1.ConditionTrue, nil
	}

	// Get and validate the Helm secret
	var hparams setup.HelmReleaseParams
	{
		logger := logger.WithValues("secret", testSuite.Spec.SetUp.Helm.SecretName)
		logger.Trace("getting setup secret")
		setupParams := v1.Secret{}
		if e := r.Client.Get(ctx, client.ObjectKey{
			Namespace: testSuite.Namespace,
			Name:      config.SecretName,
		}, &setupParams); e != nil {
			if e = client.IgnoreNotFound(e); e != nil {
				logger.Error(e, "error getting secret")
				e = utils.WrapError(e, "an error occurred fetching the specified secret")
			} else {
				msg := "specified setup secret does not exist"
				logger.Info(msg)
				e = utils.WrapError(ErrResourceNotFound, msg)
			}
			return false, e
		}

		hparams = setup.HelmReleaseParams{
			ReleaseName: testSuite.Name,
			Chart: setup.ChartRef{
				URL:      string(setupParams.Data["chart"]),
				Username: string(setupParams.Data["username"]),
				Password: string(setupParams.Data["password"]),
			},
			Values: make(map[string]any),
		}

		if v, ok := setupParams.Data["values"]; ok && len(v) > 0 {
			if err := yaml.Unmarshal(v, &hparams.Values); err != nil {
				return false, utils.WrapfError(err, "YAML decoding of Helm values failed")
			}
		}
	}

	// Set up the helm client
	var helm setup.HelmClient
	if hc, err := r.getHelmClient(ctx, testSuite.Namespace, testSuite.Spec.RunAs); err == nil {
		helm = hc
	} else {
		return false, err
	}

	// Add the SetUp condition and remove any existing error or teardown condition
	orig := testSuite.DeepCopy()
	testSuite.Status.Phase = konfirm.TestSuiteStarting
	meta.RemoveStatusCondition(&testSuite.Status.Conditions, TestSuiteErrorCondition)
	meta.RemoveStatusCondition(&testSuite.Status.Conditions, TestSuiteTearDownCondition)
	meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
		Type:               TestSuiteSetUpCondition,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: testSuite.Generation,
		Reason:             "SetUpInProgress",
		Message:            "Set up is currently in progress",
	})
	logger.Trace("updating SetUp condition")
	if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
		logger.Error(err, "error updating TestSuite setup condition")
		return false, ErrUnhandledErrorDuringSetUp
	}
	logger.Debug("updated SetUp condition")

	go func(ctx context.Context) {

		// TODO handle unexpected failures

		// Perform the release
		if rel, err := helm.Install(ctx, hparams); err == nil {
			logger.Info("set up completed")
			orig := testSuite.DeepCopy()
			testSuite.Status.Phase = konfirm.TestSuiteRunning
			meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
				Type:               TestSuiteSetUpCondition,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: testSuite.Generation,
				Reason:             "SetUpCompleted",
				Message:            fmt.Sprintf("Helm release %s successfully deployed", rel.Name),
			})
			logger.Trace("updating TestSuite status")
			if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error updating TestSuite status after failed set up")
			}
			r.Recorder.Event(testSuite, "Normal", "SetUpCompleted", "Helm set up successfully completed")
		} else {
			// When an error occurs, there is not much we can do other than stop the current run and consider it failed
			logger.Error(utils.UnwrapAndJoin(err), "set up failed with error")
			orig := testSuite.DeepCopy()
			testSuite.Status.Phase = konfirm.TestSuiteReady
			meta.RemoveStatusCondition(&testSuite.Status.Conditions, TestSuiteSetUpCondition)
			logger.Trace("updating TestSuite status")
			if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error updating TestSuite status after failed set up")
			}
			logger.Debug("updated TestSuite status")
			suiteFailing.WithLabelValues(testSuite.Namespace, testSuite.Name).Set(1)
			suitePassing.WithLabelValues(testSuite.Namespace, testSuite.Name).Set(0)
			r.Recorder.Event(testSuite, "Warning", "SetUpFailed", "Helm set up failed")
			r.Recorder.Event(testSuite, "Warning", "TestRunFailed", "test set up failed")
		}
	}(context.WithoutCancel(ctx))

	return false, nil
}

// doTearDown performs any required teardown and returns true w
func (r *TestSuiteReconciler) doTearDown(ctx context.Context, testSuite *konfirm.TestSuite) (bool, error) {

	// Check if teardown is in progress
	if c := meta.FindStatusCondition(testSuite.Status.Conditions, TestSuiteTearDownCondition); c != nil {
		return c.Status == metav1.ConditionTrue, nil
	}

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName, "namespace", testSuite.Namespace, "testSuite", testSuite.Name)

	// Update conditions
	orig := testSuite.DeepCopy()
	testSuite.Status.Phase = konfirm.TestSuiteRunning
	meta.RemoveStatusCondition(&testSuite.Status.Conditions, TestSuiteSetUpCondition)
	meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
		Type:               TestSuiteTearDownCondition,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: testSuite.Generation,
		Reason:             "TearDownInProgress",
		Message:            "tear down is currently in progress",
	})
	logger.Trace("updating TestSuite status")
	if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
		logger.Error(err, "error updating TestSuite status after failed set up")
	}

	// Set up the helm client
	var helm setup.HelmClient
	if hc, err := r.getHelmClient(ctx, testSuite.Namespace, testSuite.Spec.RunAs); err == nil {
		helm = hc
	} else {
		return false, err
	}

	// Perform the teardown
	logger.Info("starting teardown")
	go func(ctx context.Context) {

		// TODO handle unexpected failures

		// Perform the release
		if err := helm.Uninstall(ctx, testSuite.Name); err == nil {
			logger.Info("teardown completed")
			orig := testSuite.DeepCopy()
			meta.SetStatusCondition(&testSuite.Status.Conditions, metav1.Condition{
				Type:               TestSuiteTearDownCondition,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: testSuite.Generation,
				Reason:             "TeardownCompleted",
				Message:            fmt.Sprintf("Helm release %s successfully uninstalled", testSuite.Name),
			})
			logger.Trace("updating TestSuite status")
			if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error updating TestSuite status after failed set up")
			}
			r.Recorder.Event(testSuite, "Normal", "SetUpCompleted", "Helm set up successfully completed")
		} else {
			// When an error occurs, there is not much we can do other than record an event and otherwise behave as if it had not failed
			logger.Error(err, "teardown failed with error")
			orig := testSuite.DeepCopy()
			testSuite.Status.Phase = konfirm.TestSuiteReady
			meta.RemoveStatusCondition(&testSuite.Status.Conditions, TestSuiteTearDownCondition)
			logger.Trace("updating TestSuite status")
			if err := r.Client.Status().Patch(ctx, testSuite, client.MergeFrom(orig)); err != nil {
				logger.Error(err, "error updating TestSuite status after failed teardown")
			}
			logger.Debug("updated TestSuite status")
			r.Recorder.Event(testSuite, "Warning", "TeardownFailed", "Helm uninstall failed")
		}
	}(context.WithoutCancel(ctx))

	return false, nil
}

func (r *TestSuiteReconciler) getHelmClient(ctx context.Context, namespace string, runas string) (setup.HelmClient, error) {

	logger := logging.FromContextWithName(ctx, testSuiteControllerLoggerName, "namespace", namespace)

	// Get impersonator
	var user impersonate.Client
	logger.Trace("initiating impersonation")
	if u, err := r.Impersonate(ctx, namespace, runas); err == nil {
		user = u
	} else {
		if errors.Is(err, impersonate.ErrUserRefNotFound) {
			logger.Debug("UserRef not found", "userRef", runas)
			return nil, errors.New("specified UserRef not found")
		} else {
			logger.Error(err, "unable to run as UserRef", "userRef", runas)
			return nil, ErrUnhandledErrorDuringSetUp
		}
	}
	logger.Debug("impersonation initialized")

	// Create the Helm Client
	if hc, err := setup.NewHelmClient(setup.HelmClientOptions{
		ReleaseNamespace: namespace,
		RESTMapper:       user.RESTMapper(),
		RESTConfig:       user.Config(),
		WorkDir:          r.DataDir,
	}); err == nil {
		return hc, nil
	} else {
		logger.Error(err, "error creating HelmClient")
		return nil, ErrUnhandledErrorDuringSetUp
	}
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

	// Add an indexer to track Secret types for efficient Helm release listing
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Secret{}, "field:type", func(rawObj client.Object) []string {
		secret := rawObj.(*v1.Secret)
		if st := secret.Type; st != "" {
			return []string{string(st)}
		}
		return nil
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&konfirm.TestSuite{}).
		Owns(&konfirm.TestRun{}).
		Watches(&v1.Secret{}, &EnqueueForHelmTrigger{Client: mgr.GetClient()}, builder.WithPredicates(&IsHelmRelease{})).
		Complete(r)
}
