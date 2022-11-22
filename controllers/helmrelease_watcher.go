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
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	HelmSecretType = "helm.sh/release.v1"
)

// IsHelmRelease is a predicate.Predicate implementation that matches only Helm
// Release Secrets.
type IsHelmRelease struct{}

func (h IsHelmRelease) Create(e event.CreateEvent) bool {
	return h.isHelmRelease(e.Object)
}

func (h IsHelmRelease) Delete(e event.DeleteEvent) bool {
	return h.isHelmRelease(e.Object)
}

func (h IsHelmRelease) Update(e event.UpdateEvent) bool {
	return h.isHelmRelease(e.ObjectNew)
}

func (h IsHelmRelease) Generic(e event.GenericEvent) bool {
	return h.isHelmRelease(e.Object)
}

func (_ IsHelmRelease) isHelmRelease(rawObj client.Object) bool {
	if secret, ok := rawObj.(*v1.Secret); ok {
		return secret.Type == HelmSecretType
	} else {
		return false
	}
}

// EnqueueForHelmTrigger is a handler.Handler implementation for enqueuing
// TestSuites with a Helm trigger when a matching Helm Release is deployed.
// It only responds to Create and Update events.
type EnqueueForHelmTrigger struct {
	client.Client
	Context context.Context
}

func (h *EnqueueForHelmTrigger) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.handle(e.Object, q)
}

func (h *EnqueueForHelmTrigger) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueForHelmTrigger) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueForHelmTrigger) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

// handle responds to change to a Helm Release secret by enqueuing associated
// TestSuites for reconciliation. TestSuites outside the Helm Release's own
// namespace must match an explicitly defined HelmPolicy (named for the release)
// to be enqueued for reconciliation.
func (h *EnqueueForHelmTrigger) handle(obj client.Object, q workqueue.RateLimitingInterface) {

	logger := logging.FromContextWithName(h.Context, "helmrelease-watcher", "namespace", obj.GetNamespace(), "secret", obj.GetName())

	// Parse the Helm release secret
	var release *HelmReleaseMeta
	if r, ok := ParseHelmReleaseSecret(obj.(*v1.Secret)); ok {
		release = r
		logger = logger.WithValues("helmRelease", release.Name, "helmReleaseVersion", release.Version)
	} else {
		logger.Error(errors.New("required Helm labels are missing"), "secret is not a valid Helm release")
		return
	}
	logger.Info("processing new Helm release")

	// Enqueue triggered TestSuites in the same namespace
	logger.Trace("listing subscribed test suites")
	testSuites := konfirm.TestSuiteList{}
	releaseLabelMatcher := client.MatchingLabels(map[string]string{TestSuiteHelmTriggerLabel: release.LabelValue()})
	if err := h.List(h.Context, &testSuites, client.InNamespace(release.Namespace), releaseLabelMatcher); err != nil {
		logger.Error(err, "error listing subscribed test suites")
		return
	}
	logger.Debug("enqueuing reconciliation of subscribed test suites")
	for i := range testSuites.Items {
		ts := &testSuites.Items[i]
		req := ctrl.Request{}
		req.Namespace = ts.Namespace
		req.Name = ts.Name
		logger := logger.WithValues("testNamespace", req.Namespace, "testSuite", req.Name)
		logger.Trace("enqueuing test suite reconciliation")
		q.AddRateLimited(req)
		logger.Info("test suite enqueued for reconciliation")
	}

	// Check for a helm policy
	policy := konfirm.HelmPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: release.Namespace,
			Name:      release.Name,
		},
	}
	logger.Trace("checking for an associated helm policy")
	if err := h.Get(h.Context, client.ObjectKeyFromObject(&policy), &policy); err != nil {
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Debug("release is not covered by a helm policy")
		} else {
			logger.Error(err, "error retrieving release's helm policy")
		}
		return
	}
	logger.Debug("retrieved associated helm policy")

	// Enqueue subscribed test suites in other namespaces
	reqs := make(chan ctrl.Request)
	if len(policy.Spec.ExportTo) == 1 && policy.Spec.ExportTo[0] == "*" {
		go func() {
			defer close(reqs)
			logger.Trace("listing subscribed tests suites in all namespaces")
			testSuites = konfirm.TestSuiteList{}
			if err := h.List(h.Context, &testSuites, releaseLabelMatcher); err != nil {
				logger.Error(err, "error listing subscribed test suites in all other namespaces")
				return
			}
			logger.Debug("listed subscribed tests suites in all namespaces")
			var req ctrl.Request
			for i := range testSuites.Items {
				// Ignore the release's own namespace, since that already happened
				if testSuites.Items[i].Namespace == release.Namespace {
					continue
				}
				req = ctrl.Request{}
				req.Namespace = testSuites.Items[i].Namespace
				req.Name = testSuites.Items[i].Name
				reqs <- req
			}
		}()
	} else {
		go func() {
			defer close(reqs)
			logger.Trace("listing subscribed tests suites in extra namespaces")
			for i := range policy.Spec.ExportTo {
				namespace := policy.Spec.ExportTo[i]
				testSuites = konfirm.TestSuiteList{}
				if err := h.List(h.Context, &testSuites, client.InNamespace(namespace), releaseLabelMatcher); err != nil {
					logger.Error(err, "error listing subscribed test suites in extra namespace", "extraNamespace", namespace)
					return
				}
				var req ctrl.Request
				for j := range testSuites.Items {
					req = ctrl.Request{}
					req.Namespace = testSuites.Items[j].Namespace
					req.Name = testSuites.Items[j].Name
					reqs <- req
				}
			}
		}()
	}
	logger.Debug("enqueuing reconciliation of subscribed test suites in additional namespaces")
	for r := range reqs {
		logger := logger.WithValues("testNamespace", r.Namespace, "testSuite", r.Name)
		logger.Trace("enqueuing test suite reconciliation")
		q.AddRateLimited(r)
		logger.Info("test suite enqueued for reconciliation")
	}
}
