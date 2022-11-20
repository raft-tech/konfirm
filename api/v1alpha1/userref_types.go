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

// UserRefSpec defines the desired state of UserRef
type UserRefSpec struct {

	// UserName sets the username that will be impersonated when this UserRef
	// is used.
	UserName string `json:"username,omitempty"`
}

//+kubebuilder:object:root=true

// UserRef is the Schema for the userrefs API
type UserRef struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec UserRefSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// UserRefList contains a list of UserRef
type UserRefList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserRef `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UserRef{}, &UserRefList{})
}
