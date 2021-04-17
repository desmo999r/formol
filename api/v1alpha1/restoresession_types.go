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
	//"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RestoreSessionSpec defines the desired state of RestoreSession
type RestoreSessionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ref string `json:"backupSessionRef"`
}

// RestoreSessionStatus defines the observed state of RestoreSession
type RestoreSessionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	SessionState `json:"state,omitempty"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	Targets []TargetStatus `json:"target,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName="rs"
// +kubebuilder:subresource:status

// RestoreSession is the Schema for the restoresessions API
type RestoreSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreSessionSpec   `json:"spec,omitempty"`
	Status RestoreSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RestoreSessionList contains a list of RestoreSession
type RestoreSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestoreSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestoreSession{}, &RestoreSessionList{})
}
