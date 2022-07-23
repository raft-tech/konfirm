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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestTemplate struct {
	Name string             `json:"name"`
	Test v1.PodTemplateSpec `json:"test"`
}

// TestSuiteSpec defines the desired state of TestSuite
type TestSuiteSpec struct {
	RetentionPolicy TestRetainPolicy `json:"retentionPolicy,omitempty"`
	Tests           []TestTemplate   `json:"tests"`
}

//+kubebuilder:object:root=true

// TestSuite is the Schema for the testsuites API
type TestSuite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestSuiteSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// TestSuiteList contains a list of TestSuite
type TestSuiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestSuite `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestSuite{}, &TestSuiteList{})
}
