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
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

// HelmTriggerSpec defines the desired state of HelmTrigger
type HelmTriggerSpec struct {
	ExportTo []string `json:"exportTo,omitempty"`
}

type ReleaseVersion int

func (rv *ReleaseVersion) String() string {
	if rv == nil {
		return ""
	} else {
		return fmt.Sprintf("v%d", rv)
	}
}

func (rv *ReleaseVersion) Int() int {
	if rv == nil {
		return 0
	} else {
		return int(*rv)
	}
}

func (rv *ReleaseVersion) MarshalText() (text []byte, err error) {
	return []byte(rv.String()), nil
}

func (rv *ReleaseVersion) UnmarshalText(text []byte) error {
	if l := len(text); l == 0 {
		*rv = 0
	} else if l == 1 {
		return errors.New("invalid format")
	} else if i, err := strconv.Atoi(string(text[0:])); err == nil {
		*rv = ReleaseVersion(i)
	} else {
		return err
	}
	return nil
}

// HelmTriggerStatus defines the observed state of HelmTrigger
type HelmTriggerStatus struct {
	CurrentVersion *ReleaseVersion `json:"currentVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HelmTrigger is the Schema for the helmtriggers API
type HelmTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmTriggerSpec   `json:"spec,omitempty"`
	Status HelmTriggerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HelmTriggerList contains a list of HelmTrigger
type HelmTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmTrigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HelmTrigger{}, &HelmTriggerList{})
}
