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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestRunPhase describes the phase a TestRun is currently in
type TestRunPhase string

const (
	TestRunStarting TestRunPhase = "Starting"
	TestRunRunning  TestRunPhase = "Running"
	TestRunPassed   TestRunPhase = "Passed"
	TestRunFailed   TestRunPhase = "Failed"
)

func (t TestRunPhase) IsFinal() bool {
	return t == TestRunPassed || t == TestRunFailed
}

// TestRunSpec defines the desired state of TestRun
type TestRunSpec struct {

	// +kubebuilder:default=OnFailure
	RetentionPolicy RetainPolicy `json:"retentionPolicy,omitempty"`

	// RunAs is the name of the UserRef the resulting pods will be managed by
	RunAs string `json:"runAs,omitempty"`

	// +kubebuilder:validation:MinItems=1
	Tests []TestTemplate `json:"tests"`
}

// TestResult describes the outcome of a completed Test
type TestResult struct {
	Description string `json:"description,omitempty"`
	Passed      bool   `json:"passed,omitempty"`
	Test        string `json:"test,omitempty"`
}

// TestRunStatus defines the observed state of TestRun
type TestRunStatus struct {

	// Current Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +kubebuilder:default=Pending
	// Phase (Pending, Starting, Passed, Failed)
	Phase TestRunPhase `json:"phase,omitempty"`

	// Message
	Message string `json:"message,omitempty"`

	// Results
	Results []TestResult `json:"results,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=tr
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`

// TestRun is the Schema for the testruns API
type TestRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestRunSpec   `json:"spec,omitempty"`
	Status TestRunStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TestRunList contains a list of TestRun
type TestRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestRun `json:"items"`
}

func (l *TestRunList) GetObjects() []client.Object {
	var objs = make([]client.Object, len(l.Items))
	for i := range l.Items {
		objs[i] = &l.Items[i]
	}
	return objs
}

func init() {
	SchemeBuilder.Register(&TestRun{}, &TestRunList{})
}
