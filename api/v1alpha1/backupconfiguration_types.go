/*
Copyright 2023.

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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:validation:Enum=Deployment;StatefulSet;Pod
type TargetKind string

const (
	Deployment  TargetKind = "Deployment"
	StatefulSet TargetKind = "StatefulSet"
	Pod         TargetKind = "Pod"
)

// +kubebuilder:validation:Enum=Online;Snapshot;Job
type BackupType string

const (
	SnapshotKind BackupType = "Snapshot"
	OnlineKind   BackupType = "Online"
	JobKind      BackupType = "Job"
)

func GetTargetObjects(kind TargetKind) (targetObject client.Object, targetPodSpec *corev1.PodSpec) {
	switch kind {
	case Deployment:
		deployment := appsv1.Deployment{}
		targetObject = &deployment
		targetPodSpec = &deployment.Spec.Template.Spec

	case StatefulSet:
		statefulSet := appsv1.StatefulSet{}
		targetObject = &statefulSet
		targetPodSpec = &statefulSet.Spec.Template.Spec

	}
	return
}

const (
	BACKUP_PREFIX_PATH   = `backup`
	FORMOL_SHARED_VOLUME = `formol-shared`
)

type Step struct {
	// +optional
	Finalize *string `json:"finalize,omitempty"`
	// +optional
	Initialize *string `json:"initialize,omitempty"`
	// +optional
	Backup *string `json:"backup,omitempty"`
	// +optional
	Restore *string `json:"restore,omitempty"`
}

type TargetContainer struct {
	Name string `json:"name"`
	// +optional
	Paths []string `json:"paths,omitempty"`
	// +optional
	Steps []Step `json:"steps,omitempty"`
	// +optional
	// +kubebuilder:default:=/formol-shared
	SharePath string `json:"sharePath"`
	// +optional
	Job []Step `json:"job,omitempty"`
}

type Target struct {
	BackupType `json:"backupType"`
	TargetKind `json:"targetKind"`
	TargetName string            `json:"targetName"`
	Containers []TargetContainer `json:"containers"`
	// +optional
	// +kubebuilder:default:=2
	Retry int `json:"retry"`
	// +optional
	VolumeSnapshotClass string `json:"volumeSnapshotClass,omitempty"`
}

type Keep struct {
	Last    int32 `json:"last"`
	Daily   int32 `json:"daily"`
	Weekly  int32 `json:"weekly"`
	Monthly int32 `json:"monthly"`
	Yearly  int32 `json:"yearly"`
}

// BackupConfigurationSpec defines the desired state of BackupConfiguration
type BackupConfigurationSpec struct {
	Repository string `json:"repository"`
	Image      string `json:"image"`
	// +kubebuilder:default:=false
	Suspend  *bool  `json:"suspend"`
	Schedule string `json:"schedule"`
	Keep     `json:"keep"`
	Targets  []Target `json:"targets"`
}

// BackupConfigurationStatus defines the observed state of BackupConfiguration
type BackupConfigurationStatus struct {
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	Suspended      bool         `json:"suspended"`
	ActiveCronJob  bool         `json:"activeCronJob"`
	ActiveSidecar  bool         `json:"activeSidecar"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName="bc"
//+kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`
//+kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`

// BackupConfiguration is the Schema for the backupconfigurations API
type BackupConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupConfigurationSpec   `json:"spec,omitempty"`
	Status BackupConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupConfigurationList contains a list of BackupConfiguration
type BackupConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupConfiguration{}, &BackupConfigurationList{})
}
