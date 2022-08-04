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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getCondition returns the specified condition if it exists in the provided slice.
func getCondition(condition string, from []metav1.Condition) (*metav1.Condition, bool) {
	for _, c := range from {
		if c.Type == condition {
			return &c, true
		}
	}
	return nil, false
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
