/*
Copyright 2026.

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

// AdvisorySpec defines the desired state of Advisory.
type AdvisorySpec struct {
	// The name of an approver. Setting this is used to trigger the Approval flow.
	Approver string `json:"approver,omitempty"`
}

// AdvisoryStatus defines the observed state of Advisory.
type AdvisoryStatus struct {
	Advisory     string       `json:"advisory,omitempty"`
	Explaination string       `json:"explaination,omitempty"`
	Proposal     string       `json:"proposal,omitempty"`
	Updated      *metav1.Time `json:"advisoryupdatedtime,omitempty"`
	Approved     *metav1.Time `json:"approvaltime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Advisory is the Schema for the advisories API.
type Advisory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AdvisorySpec   `json:"spec,omitempty"`
	Status AdvisoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AdvisoryList contains a list of Advisory.
type AdvisoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Advisory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Advisory{}, &AdvisoryList{})
}
