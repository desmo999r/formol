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

const (
	SidecarKind   string = "Sidecar"
	JobKind       string = "Job"
	BackupVolumes string = "Volumes"
)

type Step struct {
	Name string `json:"name"`
	// +optional
	Finalize *bool `json:"finalize,omitempty"`
}

type Hook struct {
	Cmd string `json:"cmd"`
	// +optional
	Args []string `json:"args,omitempty"`
}

type Target struct {
	// +kubebuilder:validation:Enum=Sidecar;Job
	Kind string `json:"kind"`
	Name string `json:"name"`
	// +optional
	ContainerName string `json:"containerName"`
	// +optional
	ApiVersion string `json:"apiVersion,omitempty"`
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
	// +optional
	Paths []string `json:"paths,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	Steps []Step `json:"steps,omitempty"`
	// +kubebuilder:default:=2
	Retry int `json:"retry,omitempty"`
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
	Repository string `json:"repository"`
	Image      string `json:"image"`

	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// +optional
	Schedule string `json:"schedule,omitempty"`
	// +kubebuilder:validation:MinItems=1
	Targets []Target `json:"targets"`
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
// +kubebuilder:resource:shortName="bc"
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
