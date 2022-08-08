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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:validation:Enum=Always;Never;OnFailure
type RetainPolicy string

const (
	RetainAlways    RetainPolicy = "Always"
	RetainNever     RetainPolicy = "Never"
	RetainOnFailure RetainPolicy = "OnFailure"
)

// TestTemplate describes a templated Test
type TestTemplate struct {

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Description string `json:"description"`

	RetentionPolicy RetainPolicy `json:"retentionPolicy,omitempty"`

	// +kubebuilder:validation:Required
	Template v1.PodTemplateSpec `json:"template"`
}

// TestSpec defines the desired state of Test
type TestSpec struct {

	// +kubebuilder:default=OnFailure
	// RetentionPolicy specifies how generated resources should be handled after the Test finishes.
	RetentionPolicy RetainPolicy `json:"retentionPolicy,omitempty"`

	// Template is the PodSpecTemplate that will be used to run the test
	Template v1.PodTemplateSpec `json:"template"`
}

// +kubebuilder:validation:Enum=Pending;Starting;Running;Passed;Failed;Unknown
type TestPhase string

func (p *TestPhase) FromPodPhase(phase v1.PodPhase) {
	switch phase {
	case v1.PodPending:
		*p = TestStarting
	case v1.PodRunning:
		*p = TestRunning
	case v1.PodSucceeded:
		*p = TestPassed
	case v1.PodFailed:
		*p = TestFailed
	default:
		*p = TestPending
	}
}

const (
	TestPending      TestPhase = "Pending"
	TestStarting     TestPhase = "Starting"
	TestRunning      TestPhase = "Running"
	TestPassed       TestPhase = "Passed"
	TestFailed       TestPhase = "Failed"
	TestPhaseUnknown TestPhase = "Unknown"
)

func (p TestPhase) IsFinal() bool {
	return p == TestPassed || p == TestFailed
}

func (p TestPhase) IsSuccess() bool {
	return p == TestPassed
}

func (p TestPhase) String() string {
	return string(p)
}

// TestStatus defines the observed state of Test
type TestStatus struct {

	// Current Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase (Pending, Starting, Running, Succeeded, Failed, or Unknown)
	// +kubebuilder:default=Pending
	Phase TestPhase `json:"phase,omitempty"`

	// Messages
	Messages map[string]string `json:"messages,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`

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

func (l *TestList) GetObjects() []client.Object {
	var objs = make([]client.Object, len(l.Items))
	for i := range l.Items {
		objs[i] = &l.Items[i]
	}
	return objs
}

func init() {
	SchemeBuilder.Register(&Test{}, &TestList{})
}
