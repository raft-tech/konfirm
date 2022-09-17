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

// TestSuitePhase describes the phase a TestSuite is currently in
type TestSuitePhase string

const (
	TestSuitePending TestSuitePhase = "Pending"
	TestSuiteReady   TestSuitePhase = "Ready"
	TestSuiteRunning TestSuitePhase = "Running"
	TestSuiteError   TestSuitePhase = "Error"
)

// IsPending returns true if TestSuitePhase is "Pending"
func (p TestSuitePhase) IsPending() bool {
	return p == TestSuitePending
}

// IsReady returns true if TestSuitePhase is "Ready"
func (p TestSuitePhase) IsReady() bool {
	return p == TestSuiteReady
}

// IsRunning returns true if TestSuitePhase is "Running"
func (p TestSuitePhase) IsRunning() bool {
	return p == TestSuiteRunning
}

// IsError returns true if TestSuitePhase is "Error"
func (p TestSuitePhase) IsError() bool {
	return p == TestSuitePending
}

// TestSuiteHelmTrigger describes a Helm release that will trigger a TestSuite
type TestSuiteHelmTrigger struct {
	Release string `json:"release,omitempty"`
}

// TestSuiteTriggers describes when a TestSuite should run
type TestSuiteTriggers struct {

	// Schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"cron,omitempty"`

	// HelmRelease to watch for upgrades/install
	HelmRelease string `json:"helmRelease,omitempty"`
}

// TestSuiteHelmSetUp describes a Secret-embedded
type TestSuiteHelmSetUp struct {
	SecretName string `json:"secret,omitempty"`
	ChartKey   string `json:"chartKey,omitempty"`
	ValuesKey  string `json:"valuesKey,omitempty"`
}

// TestSuiteSetUp describes any setup that should occur before the Tests are run
type TestSuiteSetUp struct {
	Helm TestSuiteHelmSetUp `json:"helm,omitempty"`
}

// TestSuiteSpec defines the desired state of TestSuite
type TestSuiteSpec struct {

	// +kubebuilder:default=3
	// +kubebuilder:validation:Maximum=255
	// +kubebuilder:validation:Minimum=0
	HistoryLimit uint8 `json:"historyLimit,omitempty"`

	SetUp TestSuiteSetUp `json:"setUp,omitempty"`

	// +kubebuilder:validation:Required
	Template TestRunSpec `json:"template"`

	When TestSuiteTriggers `json:"when,omitempty"`
}

// TestSuiteTrigger describes the trigger state of TestSuite
type TestSuiteTrigger struct {

	// +kubebuilder:default:false
	// NeedsRun is true if the TestSuite should be triggered
	NeedsRun bool `json:"needsRun"`
}

// TestSuiteStatus describes the observed state of TestSuite
type TestSuiteStatus struct {

	// Current Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +kubebuilder:default=Pending
	// Phase (Pending, Ready, Running, Error)
	Phase TestSuitePhase `json:"phase,omitempty"`

	CurrentTestRun string `json:"currentTestRun,omitempty"`
}

// TestSuite is the Schema for the testsuites API
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=ts
// +kubebuilder:subresource:trigger
// +kubebuilder:subresource:status
type TestSuite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestSuiteSpec    `json:"spec,omitempty"`
	Trigger           TestSuiteTrigger `json:"trigger,omitempty"`
	Status            TestSuiteStatus  `json:"status,omitempty"`
}

// TestSuiteList contains a list of TestSuite
// +kubebuilder:object:root=true
type TestSuiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestSuite `json:"items"`
}

func (l *TestSuiteList) GetObjects() []client.Object {
	var objs = make([]client.Object, len(l.Items))
	for i := range l.Items {
		objs[i] = &l.Items[i]
	}
	return objs
}

func init() {
	SchemeBuilder.Register(&TestSuite{}, &TestSuiteList{})
}
