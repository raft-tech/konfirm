//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Test) DeepCopyInto(out *Test) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Test.
func (in *Test) DeepCopy() *Test {
	if in == nil {
		return nil
	}
	out := new(Test)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Test) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestList) DeepCopyInto(out *TestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Test, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestList.
func (in *TestList) DeepCopy() *TestList {
	if in == nil {
		return nil
	}
	out := new(TestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestResult) DeepCopyInto(out *TestResult) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestResult.
func (in *TestResult) DeepCopy() *TestResult {
	if in == nil {
		return nil
	}
	out := new(TestResult)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestRun) DeepCopyInto(out *TestRun) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestRun.
func (in *TestRun) DeepCopy() *TestRun {
	if in == nil {
		return nil
	}
	out := new(TestRun)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TestRun) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestRunList) DeepCopyInto(out *TestRunList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TestRun, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestRunList.
func (in *TestRunList) DeepCopy() *TestRunList {
	if in == nil {
		return nil
	}
	out := new(TestRunList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TestRunList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestRunSpec) DeepCopyInto(out *TestRunSpec) {
	*out = *in
	if in.Tests != nil {
		in, out := &in.Tests, &out.Tests
		*out = make([]TestTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestRunSpec.
func (in *TestRunSpec) DeepCopy() *TestRunSpec {
	if in == nil {
		return nil
	}
	out := new(TestRunSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestRunStatus) DeepCopyInto(out *TestRunStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Results != nil {
		in, out := &in.Results, &out.Results
		*out = make([]TestResult, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestRunStatus.
func (in *TestRunStatus) DeepCopy() *TestRunStatus {
	if in == nil {
		return nil
	}
	out := new(TestRunStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSpec) DeepCopyInto(out *TestSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSpec.
func (in *TestSpec) DeepCopy() *TestSpec {
	if in == nil {
		return nil
	}
	out := new(TestSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestStatus) DeepCopyInto(out *TestStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Messages != nil {
		in, out := &in.Messages, &out.Messages
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestStatus.
func (in *TestStatus) DeepCopy() *TestStatus {
	if in == nil {
		return nil
	}
	out := new(TestStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuite) DeepCopyInto(out *TestSuite) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Trigger = in.Trigger
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuite.
func (in *TestSuite) DeepCopy() *TestSuite {
	if in == nil {
		return nil
	}
	out := new(TestSuite)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TestSuite) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteHelmSetUp) DeepCopyInto(out *TestSuiteHelmSetUp) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteHelmSetUp.
func (in *TestSuiteHelmSetUp) DeepCopy() *TestSuiteHelmSetUp {
	if in == nil {
		return nil
	}
	out := new(TestSuiteHelmSetUp)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteHelmTrigger) DeepCopyInto(out *TestSuiteHelmTrigger) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteHelmTrigger.
func (in *TestSuiteHelmTrigger) DeepCopy() *TestSuiteHelmTrigger {
	if in == nil {
		return nil
	}
	out := new(TestSuiteHelmTrigger)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteList) DeepCopyInto(out *TestSuiteList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TestSuite, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteList.
func (in *TestSuiteList) DeepCopy() *TestSuiteList {
	if in == nil {
		return nil
	}
	out := new(TestSuiteList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TestSuiteList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteSetUp) DeepCopyInto(out *TestSuiteSetUp) {
	*out = *in
	out.Helm = in.Helm
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteSetUp.
func (in *TestSuiteSetUp) DeepCopy() *TestSuiteSetUp {
	if in == nil {
		return nil
	}
	out := new(TestSuiteSetUp)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteSpec) DeepCopyInto(out *TestSuiteSpec) {
	*out = *in
	out.SetUp = in.SetUp
	in.Template.DeepCopyInto(&out.Template)
	out.When = in.When
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteSpec.
func (in *TestSuiteSpec) DeepCopy() *TestSuiteSpec {
	if in == nil {
		return nil
	}
	out := new(TestSuiteSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteStatus) DeepCopyInto(out *TestSuiteStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteStatus.
func (in *TestSuiteStatus) DeepCopy() *TestSuiteStatus {
	if in == nil {
		return nil
	}
	out := new(TestSuiteStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteTrigger) DeepCopyInto(out *TestSuiteTrigger) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteTrigger.
func (in *TestSuiteTrigger) DeepCopy() *TestSuiteTrigger {
	if in == nil {
		return nil
	}
	out := new(TestSuiteTrigger)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestSuiteTriggers) DeepCopyInto(out *TestSuiteTriggers) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestSuiteTriggers.
func (in *TestSuiteTriggers) DeepCopy() *TestSuiteTriggers {
	if in == nil {
		return nil
	}
	out := new(TestSuiteTriggers)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestTemplate) DeepCopyInto(out *TestTemplate) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestTemplate.
func (in *TestTemplate) DeepCopy() *TestTemplate {
	if in == nil {
		return nil
	}
	out := new(TestTemplate)
	in.DeepCopyInto(out)
	return out
}
