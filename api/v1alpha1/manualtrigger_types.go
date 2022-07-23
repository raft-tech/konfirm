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
)

// +kubebuilder:validation:Enum=Pending;Watching;Running
type ManualTriggerPhase string

const (
	ManualTriggerPending  ManualTriggerPhase = "Pending"
	ManualTriggerWatching ManualTriggerPhase = "Watching"
	ManualTriggerRunning  ManualTriggerPhase = "Running"
)

func (p ManualTriggerPhase) IsWatching() bool {
	return p == ManualTriggerWatching
}

func (p ManualTriggerPhase) IsRunning() bool {
	return p == ManualTriggerRunning
}

// ManualTriggerSpec defines the desired state of ManualTrigger
type ManualTriggerSpec struct {

	// +kubebuilder:validation:Required
	Suite string `json:"suite"`
}

// ManualTriggerStatus defines the observed state of ManualTrigger
type ManualTriggerStatus struct {

	// Current Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase (Pending, Running, Succeeded, Failed, or Unknown)
	Phase ManualTriggerPhase `json:"phase,omitempty"`
}

// ManualTriggerTrigger defines whether a run is desired
type ManualTriggerTrigger struct {

	// +kubebuilder:default:false
	NeedsRun bool `json:"needsRun"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:trigger

// ManualTrigger is the Schema for the manualtriggers API
type ManualTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec    ManualTriggerSpec    `json:"spec,omitempty"`
	Status  ManualTriggerStatus  `json:"status,omitempty"`
	Trigger ManualTriggerTrigger `json:"trigger"`
}

//+kubebuilder:object:root=true

// ManualTriggerList contains a list of ManualTrigger
type ManualTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManualTrigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManualTrigger{}, &ManualTriggerList{})
}
