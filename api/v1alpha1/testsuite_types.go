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

// +kubebuilder:validate:enum=Waiting;Running
// TestSuitePhase describes the phase a TestSuite is currently in
type TestSuitePhase string

const (
	TestSuiteWaiting TestSuitePhase = "Waiting"
	TestSuiteRunning TestSuitePhase = "Running"
)

// IsRunning returns true if TestSuitePhase is "Running"
func (p TestSuitePhase) IsRunning() bool {
	return p == TestSuiteRunning
}

// TestSuiteTriggers describes when a TestSuite should run
type TestSuiteTriggers struct {

	//+kubebuilder:validation:MinLength=0
	// Schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"cron,omitempty"`
}

// TestTemplate describes a templated Test
type TestTemplate struct {
	Name string             `json:"name"`
	Test v1.PodTemplateSpec `json:"test"`
}

// TestSuiteSpec defines the desired state of TestSuite
type TestSuiteSpec struct {
	RetentionPolicy TestRetainPolicy  `json:"retentionPolicy,omitempty"`
	Triggers        TestSuiteTriggers `json:"triggers,omitempty"`
	Tests           []TestTemplate    `json:"tests"`
}

// TestSuiteTrigger describes the trigger state of TestSuite
type TestSuiteTrigger struct {

	// +kubebuilder:default:false
	NeedsRun bool `json:"needsRun"`
}

// TestSuiteStatus describes the observed state of TestSuite
type TestSuiteStatus struct {

	// Current Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase (Pending, Running, Succeeded, Failed, or Unknown)
	Phase TestSuitePhase `json:"phase,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:trigger
//+kubebuilder:subresource:status

// TestSuite is the Schema for the testsuites API
type TestSuite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestSuiteSpec    `json:"spec,omitempty"`
	Trigger           TestSuiteTrigger `json:"trigger,omitempty"`
	Status            TestSuiteStatus  `json:"status,omitempty"`
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
