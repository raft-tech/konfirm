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
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	metricNamespace = "konfirm"
)

// getCondition returns the specified condition if it exists in the provided slice.
// Deprecated: use meta.FindStatusCondition instead.
func getCondition(condition string, from []metav1.Condition) (*metav1.Condition, bool) {
	c := meta.FindStatusCondition(from, condition)
	return c, c != nil
}

// hasCondition returns true if the specified Condition exists in the provided
// slice and the status matches the provided value.
func hasCondition(condition string, matching metav1.ConditionStatus, from []metav1.Condition) bool {
	matched := false
	if c, ok := getCondition(condition, from); ok {
		matched = c.Status == matching
	}
	return matched
}

// addFinalizer adds the specified finalizer to the provided object
// if necessary. If the specified finalizer is not present in the object, the
// returned boolean value will be true. If an error occurs while patching the
// provided object, it will be returned.
func addFinalizer(ctx context.Context, kClient client.Client, finalizer string, obj client.Object) (bool, error) {
	var err error
	patch := true
	for _, f := range obj.GetFinalizers() {
		if f == finalizer {
			patch = false
		}
	}
	if patch {
		orig := obj.DeepCopyObject().(client.Object)
		obj.SetFinalizers(append(obj.GetFinalizers(), finalizer))
		err = kClient.Patch(ctx, obj, client.MergeFrom(orig))
	}
	return patch, err
}

// removeFinalizer removes the specified finalizer from the provided object
// if necessary. If the specified finalizer is present in the object, the
// returned boolean value will be true. If an error occurs while patching the
// provided object, it will be returned.
func removeFinalizer(ctx context.Context, kClient client.Client, finalizer string, obj client.Object) (bool, error) {
	var err error
	patch := false
	var newFinalizers []string
	for _, f := range obj.GetFinalizers() {
		if f == finalizer {
			patch = true
		} else {
			newFinalizers = append(newFinalizers, f)
		}
	}
	if patch {
		orig := obj.DeepCopyObject().(client.Object)
		obj.SetFinalizers(newFinalizers)
		err = kClient.Patch(ctx, obj, client.MergeFrom(orig))
	}
	return patch, err
}

// cleanUp deletes the provided object and, if necessary, removes the specified
// finalizer. If changes are made (object deleted and/or finalizer removed),
// the returned boolean will be true. If an error occurs, it will be returned.
func cleanUp(ctx context.Context, kClient client.Client, finalizer string, obj client.Object) (bool, error) {
	modified := false
	if obj.GetDeletionTimestamp() == nil {
		if err := kClient.Delete(ctx, obj); err == nil {
			modified = true
		} else {
			return false, client.IgnoreNotFound(err)
		}
	}
	if patched, err := removeFinalizer(ctx, kClient, finalizer, obj); err != nil {
		return modified, err
	} else {
		modified = modified || patched
		return modified, client.IgnoreNotFound(err)
	}
}

// cleanUpAll deletes the provided objects and, if necessary, removes the specified
// finalizer. If changes are made (objects deleted and/or finalizers removed),
// the returned boolean will be true. If any errors, they will be returned in an
// ErrorList.
func cleanUpAll(ctx context.Context, kClient client.Client, finalizer string, objs []client.Object) (bool, error) {
	modified := false
	errs := ErrorList{}
	for _, o := range objs {
		m, err := cleanUp(ctx, kClient, finalizer, o)
		modified = modified || m
		if err != nil {
			errs.Append(err)
		}
	}
	return modified, errs.Error()
}

func computeTestRunHash(testRun *konfirm.TestRunSpec) string {
	hasher := fnv.New32a()
	printer := spew.ConfigState{
		DisableMethods: true,
		Indent:         " ",
		SpewKeys:       true,
		SortKeys:       true,
	}
	_, _ = printer.Fprintf(hasher, "%#v", testRun)
	digest := fmt.Sprint(hasher.Sum32())
	return rand.SafeEncodeString(digest)
}

// HelmReleaseMeta describes a Helm Release.
type HelmReleaseMeta struct {
	types.NamespacedName
	Version       int
	VersionString string
	Status        string
}

func (hrm *HelmReleaseMeta) LabelValue() string {
	if hrm == nil {
		return ""
	}
	val := hrm.Name
	if hrm.Namespace != "" {
		val += "." + hrm.Namespace
	}
	return val
}

// ParseHelmReleaseSecret generates HelmReleaseMeta based on the
// provided v1.Secret. If the provided Secret does not appear to be a valid Helm
// release, the return boolean will be false; otherwise it will be true.
func ParseHelmReleaseSecret(secret *v1.Secret) (release *HelmReleaseMeta, ok bool) {

	if secret == nil || secret.Type != HelmSecretType {
		return
	} else {
		ok = true
		release = &HelmReleaseMeta{}
		release.Namespace = secret.Namespace
	}

	if release.Name, ok = secret.Labels["name"]; !ok {
		return
	}

	if release.VersionString, ok = secret.Labels["version"]; !ok {
		return
	}

	if release.Status, ok = secret.Labels["status"]; !ok {
		return
	}

	if v, err := strconv.Atoi(release.VersionString); err == nil {
		release.Version = v
	} else {
		ok = false
		return
	}

	return
}

// MakeNamespacedName parses the provided string to generate
// a types.NamespacedName. If a namespace is not explicitly defined, the
// Namespace on the returned object will be empty.
func MakeNamespacedName(str string) (n types.NamespacedName) {
	if pos := strings.Index(str, string(types.Separator)); pos != -1 && pos+1 < len(str) {
		n.Namespace = str[:pos]
		n.Name = str[pos+1:]
	} else {
		n.Name = str
	}
	return
}

// GetBackOff calculates the next backoff duration given the time since
// backoff started. The backoff rate is exponential, starting at 5 seconds and
// leveling at 5 minutes.
func GetBackOff(since time.Duration) (b time.Duration) {
	if s := since.Seconds(); s < 5.0 {
		b = 5.0 * time.Second
	} else if s < 160.0 {
		i := math.Floor(math.Log2(s/5.0) + 1)                 // How many backoff iterations have occurred
		b = time.Duration(math.Pow(2.0, i)*5.0) * time.Second // Calculate next backoff period
	} else {
		b = 5.0 * time.Minute
	}
	return
}
