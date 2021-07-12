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

// PreprovisioningImageSpec defines the desired state of PreprovisioningImage
type PreprovisioningImageSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of PreprovisioningImage. Edit PreprovisioningImage_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// PreprovisioningImageStatus defines the observed state of PreprovisioningImage
type PreprovisioningImageStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// PreprovisioningImage is the Schema for the preprovisioningimages API
type PreprovisioningImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreprovisioningImageSpec   `json:"spec,omitempty"`
	Status PreprovisioningImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PreprovisioningImageList contains a list of PreprovisioningImage
type PreprovisioningImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreprovisioningImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreprovisioningImage{}, &PreprovisioningImageList{})
}
