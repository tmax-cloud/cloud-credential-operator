/*


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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	CloudCredentialStatusTypeAwaiting = "Awaiting"
	CloudCredentialStatusTypeCreated  = "Created"
	CloudCredentialStatusTypeError    = "Error"
	CloudCredentialStatusTypeDeleted  = "CloudCredential Deleted"
)

// CloudCredentialSpec defines the desired state of CloudCredential
type CloudCredentialSpec struct {
	Provider    string `json:"provider"`
	AccessKeyID string `json:"accesskeyid"`
	AccessKey   string `json:"accesskey"`
	Region      string `json:"region,omitempty"`
}

// CloudCredentialStatus defines the observed state of CloudCredential
type CloudCredentialStatus struct {
	// Message shows log when the status changed in last
	Message string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
	// Reason shows why the status changed in last
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
	// LastTransitionTime shows the time when the status changed in last
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// +kubebuilder:validation:Enum=Awaiting;Created;Error;CloudCredential Deleted;
	// Status shows the present status of the CloudCredential
	Status string `json:"status,omitempty" protobuf:"bytes,4,opt,name=status"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.reason`
// CloudCredential is the Schema for the cloudcredentials API
type CloudCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudCredentialSpec   `json:"spec,omitempty"`
	Status CloudCredentialStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudCredentialList contains a list of CloudCredential
type CloudCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudCredential{}, &CloudCredentialList{})
}
