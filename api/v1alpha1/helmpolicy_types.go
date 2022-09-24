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

// HelmPolicySpec defines the desired state of HelmPolicy
type HelmPolicySpec struct {

	// ExportTo specifies namespaces permitted to observe a release
	ExportTo []string `json:"exportTo,omitempty"`
}

//+kubebuilder:object:root=true

// HelmPolicy is the Schema for the helmpolicies API
type HelmPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HelmPolicySpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// HelmPolicyList contains a list of HelmPolicy
type HelmPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HelmPolicy{}, &HelmPolicyList{})
}
