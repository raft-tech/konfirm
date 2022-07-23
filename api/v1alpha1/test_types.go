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

// +kubebuilder:validation:Enum=Always;Never;OnFailure
type TestRetainPolicy string

const (
	RetainAlways    TestRetainPolicy = "Always"
	RetainNever     TestRetainPolicy = "Never"
	RetainOnFailure TestRetainPolicy = "OnFailure"
)

// TestSpec defines the desired state of Test
type TestSpec struct {

	// +kubebuilder:default=OnFailure
	// RetentionPolicy specifies how generated resources should be handled after the Test finishes.
	RetentionPolicy TestRetainPolicy `json:"retentionPolicy,omitempty"`

	// Template is the PodSpecTemplate that will be used to run the test
	Template v1.PodTemplateSpec `json:"template"`
}

// +kubebuilder:validation:Enum=Pending;Running;Passed;Failed;Unknown
type TestPhase string

const (
	TestPending      TestPhase = "Pending"
	TestRunning      TestPhase = "Running"
	TestSucceeded    TestPhase = "Succeeded"
	TestFailed       TestPhase = "Failed"
	TestPhaseUnknown TestPhase = "Unknown"
)

func (p TestPhase) IsFinal() bool {
	return p == TestSucceeded || p == TestFailed
}

func (p TestPhase) String() string {
	return string(p)
}

// TestStatus defines the observed state of Test
type TestStatus struct {

	// Current Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase (Pending, Running, Succeeded, Failed, or Unknown)
	Phase TestPhase `json:"phase,omitempty"`

	// Messages
	Messages []string `json:"messages,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Test is the Schema for the tests API
type Test struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestSpec   `json:"spec,omitempty"`
	Status            TestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TestList contains a list of Test
type TestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Test `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Test{}, &TestList{})
}
