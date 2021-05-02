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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BackupSessionSpec defines the desired state of BackupSession
type BackupSessionSpec struct {
	Ref corev1.ObjectReference `json:"ref"`
}

// BackupSessionStatus defines the observed state of BackupSession
type BackupSessionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +optional
	SessionState `json:"state,omitempty"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	Targets []TargetStatus `json:"target,omitempty"`
	// +optional
	Keep string `json:"keep,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName="bs"
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ref",type=string,JSONPath=`.spec.ref.name`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Started",type=string,format=date-time,JSONPath=`.status.startTime`
// +kubebuilder:printcolumn:name="Keep",type=string,JSONPath=`.status.keep`

// BackupSession is the Schema for the backupsessions API
type BackupSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSessionSpec   `json:"spec,omitempty"`
	Status BackupSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupSessionList contains a list of BackupSession
type BackupSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSession{}, &BackupSessionList{})
}
