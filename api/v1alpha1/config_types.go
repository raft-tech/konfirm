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
	config "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

//+kubebuilder:object:root=true

// Config is the Schema for the configs API
type Config struct {
	metav1.TypeMeta `json:",inline"`
	config.ControllerManagerConfigurationSpec

	// IgnoredNamespaces if specified causes the controller to ignore resources
	// in the named namespaces.
	IgnoredNamespaces []string `json:"ignoredNamespaces,omitempty"`

	// WatchedNamespaces if specified causes the controller to ignore resources
	// in any namespace not explicitly specified.
	//
	// To restrict the controller to a single namespace, use CacheNamespace.
	WatchedNamespaces []string `json:"watchedNamespaces,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Config{})
}
