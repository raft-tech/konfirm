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
	"github.com/raft-tech/konfirm/logging"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"strings"
	"time"

	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
)

const (
	helmSecretType               = "helm.sh/release.v1"
	HelmTriggerLabel             = konfirm.GroupName + "/helm-trigger"
	HelmTriggerVersionAnnotation = konfirm.GroupName + "/current-helm-trigger-version"
)

// HelmTriggerReconciler reconciles a HelmTrigger object
type HelmTriggerReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	ErrRequeueDelay time.Duration
}

//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=helmtriggers,verbs=get;list;watch
//+kubebuilder:rbac:groups=konfirm.goraft.tech,resources=helmtriggers/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *HelmTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := logging.FromContextWithName(ctx, "helmtrigger-controller")
	logger.Debug("starting reconciliation")

	// Retrieve the Helm Trigger
	logger.Trace("getting helm-trigger")
	trigger := konfirm.HelmTrigger{}
	if err := r.Get(ctx, req.NamespacedName, &trigger); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Trace("helm-trigger does not exist")
			return ctrl.Result{}, err
		}
		logger.Error(err, "error retrieving helm-trigger")
		return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
	}
	logger.Debug("retrieved helm-trigger")

	// Retrieve the current release
	logger.Trace("listing deployed releases")
	releases := v1.SecretList{}
	if err := r.List(
		ctx,
		&releases,
		client.InNamespace(req.Namespace),
		client.MatchingFields(map[string]string{"type": helmSecretType}),
		client.MatchingLabels(map[string]string{"name": req.Name, "status": "deployed"})); err != nil {
		logger.Error(err, "error listing Helm secrets")
		return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
	}
	logger.Debug("listed deployed releases")

	// Determine the current release version
	lastSeen := trigger.Status.CurrentVersion.Int()
	current := lastSeen
	for i := range releases.Items {
		if vs, ok := releases.Items[i].Labels["version"]; ok {
			if v, err := strconv.Atoi(vs); err == nil && v > current {
				current = v
			}
		}
	}

	// Trigger if necessary
	if current != lastSeen {
		currentVersion := konfirm.ReleaseVersion(current)
		logger = logger.WithValues("currentVersion", currentVersion, "previousVersion", konfirm.ReleaseVersion(lastSeen))
		withLabels := client.MatchingLabels(map[string]string{HelmTriggerLabel: req.NamespacedName.String()})
		suites := konfirm.TestSuiteList{}
		if l := len(trigger.Spec.ExportTo); l == 1 && trigger.Spec.ExportTo[0] == "*" {
			logger.Trace("listing all related test-suites")
			if err := r.List(ctx, &suites, withLabels); err != nil {
				logger.Error(err, "error listing related test-suites")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("triggering related test-suites in all namespaces")
			if err := r.patchAll(ctx, &suites, currentVersion.String()); err != nil {
				logger.Error(err, "error triggering related test-suites")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Info("triggered all related test-suites")
		} else {
			logger.Trace("listing related test-suites in the same namespace")
			if err := r.List(ctx, &suites, client.InNamespace(req.Namespace), withLabels); err != nil {
				logger.Error(err, "error listing related test-suites")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Debug("triggering related test-suites in the same namespace")
			if err := r.patchAll(ctx, &suites, currentVersion.String()); err != nil {
				logger.Error(err, "error triggering related test-suites")
				return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
			}
			logger.Info("triggered related test-suites in the same namespace")
			for i := range trigger.Spec.ExportTo {
				logger := logger.WithValues("testNamespace", trigger.Spec.ExportTo[i])
				logger.Trace("listing related test-suites in additional namespace")
				if err := r.List(ctx, &suites, client.InNamespace(req.Namespace), withLabels); err != nil {
					logger.Error(err, "error listing related test-suites in additional namespace")
					return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
				}
				logger.Debug("triggering related test-suites in additional namespace")
				if err := r.patchAll(ctx, &suites, currentVersion.String()); err != nil {
					logger.Error(err, "error triggering related test-suites")
					return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
				}
				logger.Info("triggered related test-suites in additional namespace")
			}
		}

		// Patch HelmTrigger Status to current version
		orig := trigger.DeepCopy()
		newVersion := konfirm.ReleaseVersion(current)
		trigger.Status.CurrentVersion = &newVersion
		logger.Trace("patching helm-trigger status")
		if err := r.Status().Patch(ctx, &trigger, client.MergeFrom(orig)); err != nil {
			logger.Error(err, "error patching helm-trigger status")
			return ctrl.Result{RequeueAfter: r.ErrRequeueDelay}, nil
		}
		logger.Debug("patched helm-trigger status")
	}

	return ctrl.Result{}, nil
}

func (r *HelmTriggerReconciler) patchAll(ctx context.Context, suites *konfirm.TestSuiteList, releaseVersion string) error {
	errors := ErrorList{}
	for i := range suites.Items {
		ts := &suites.Items[i]
		if ts.Annotations[HelmTriggerVersionAnnotation] != releaseVersion {
			orig := ts.DeepCopy()
			ts.Annotations[HelmTriggerVersionAnnotation] = releaseVersion
			if err := r.Patch(ctx, ts, client.MergeFrom(orig)); err != nil {
				errors.Append(err)
			}
		}
	}
	return errors.Error()
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelmTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&konfirm.HelmTrigger{}).
		Watches(&source.Kind{Type: &v1.Secret{}}, &enqueueForHelmTrigger{}, builder.WithPredicates(&isHelmRelease{})).
		Complete(r)
}

type isHelmRelease struct{}

func (h isHelmRelease) Create(e event.CreateEvent) bool {
	return h.isHelmRelease(e.Object)
}

func (h isHelmRelease) Delete(e event.DeleteEvent) bool {
	return h.isHelmRelease(e.Object)
}

func (h isHelmRelease) Update(e event.UpdateEvent) bool {
	return h.isHelmRelease(e.ObjectNew)
}

func (h isHelmRelease) Generic(e event.GenericEvent) bool {
	return h.isHelmRelease(e.Object)
}

func (_ isHelmRelease) isHelmRelease(rawObj client.Object) bool {
	if secret, ok := rawObj.(*v1.Secret); ok {
		return secret.Type == helmSecretType
	} else {
		return false
	}
}

type enqueueForHelmTrigger struct{}

func (h *enqueueForHelmTrigger) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	if req, ok := h.generateRequest(e.Object); ok {
		q.Add(req)
	}
}

func (h *enqueueForHelmTrigger) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if req, ok := h.generateRequest(e.ObjectNew); ok {
		q.Add(req)
	}
}

func (h *enqueueForHelmTrigger) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if req, ok := h.generateRequest(e.Object); ok {
		q.Add(req)
	}
}

func (h *enqueueForHelmTrigger) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	if req, ok := h.generateRequest(e.Object); ok {
		q.Add(req)
	}
}

func (_ *enqueueForHelmTrigger) generateRequest(obj client.Object) (req ctrl.Request, ok bool) {
	if s := strings.SplitN(obj.GetName(), ".", 6); len(s) == 6 && s[4] != "" {
		req.Namespace = obj.GetNamespace()
		req.Name = s[4]
		ok = true
	}
	return
}
