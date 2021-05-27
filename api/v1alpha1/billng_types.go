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

// BillngSpec defines the desired state of Billng
type BillngSpec struct {
	Provider    string `json:"provider"`
	CredentialName string `json:"credentialname"`
}

// BillngStatus defines the observed state of Billng
type BillngStatus struct {
	// Message shows log when the status changed in last
	Message string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
	// Reason shows why the status changed in last
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
	// LastTransitionTime shows the time when the status changed in last
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Billng is the Schema for the billngs API
type Billng struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BillngSpec   `json:"spec,omitempty"`
	Status BillngStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BillngList contains a list of Billng
type BillngList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Billng `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Billng{}, &BillngList{})
}
