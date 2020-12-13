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

// Resource references a repsoitory where the backups will be stored
type Repository struct {
	Name string `json:"name"`
}

type Target struct {
	ApiVersion string `json:"apiVersion"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
}

type Keep struct {
	Last    int32 `json:"last,omitempty"`
	Daily   int32 `json:"daily,omitempty"`
	Weekly  int32 `json:"weekly,omitempty"`
	Monthly int32 `json:"monthly,omitempty"`
	Yearly  int32 `json:"yearly,omitempty"`
}

// BackupConfigurationSpec defines the desired state of BackupConfiguration
type BackupConfigurationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of BackupConfiguration. Edit BackupConfiguration_types.go to remove/update
	Repository `json:"repository"`
	Task       string `json:"task,omitempty"`
	Schedule   string `json:"schedule"`
	Target     `json:"target"`
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
	Paths        []string             `json:"paths"`
	// +optional
	Keep `json:"keep,omitempty"`
}

// BackupConfigurationStatus defines the observed state of BackupConfiguration
type BackupConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	Suspended      bool         `json:"suspended"`
	ActiveCronJob  bool         `json:"activeCronJob"`
	ActiveSidecar  bool         `json:"activeSidecar"`
}

// BackupConfiguration is the Schema for the backupconfigurations API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type BackupConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupConfigurationSpec   `json:"spec,omitempty"`
	Status BackupConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupConfigurationList contains a list of BackupConfiguration
type BackupConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupConfiguration{}, &BackupConfigurationList{})
}
