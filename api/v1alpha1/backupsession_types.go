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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SessionState string

const (
	New           SessionState = "New"
	Initializing  SessionState = "Initializing"
	Initialized   SessionState = "Initialized"
	Running       SessionState = "Running"
	Waiting       SessionState = "Waiting"
	WaitingForJob SessionState = "WaitingForJob"
	Finalize      SessionState = "Finalize"
	Success       SessionState = "Success"
	Failure       SessionState = "Failure"
	//Deleted      SessionState = "Deleted"
)

type TargetStatus struct {
	BackupType   `json:"backupType"`
	TargetName   string `json:"targetName"`
	TargetKind   `json:"targetKind"`
	SessionState `json:"state"`
	// +optional
	SnapshotId string           `json:"snapshotId,omitempty"`
	StartTime  *metav1.Time     `json:"startTime"`
	Duration   *metav1.Duration `json:"duration,omitempty"`
	Try        int              `json:"try"`
}

// BackupSessionSpec defines the desired state of BackupSession
type BackupSessionSpec struct {
	Ref corev1.ObjectReference `json:"ref"`
}

// BackupSessionStatus defines the observed state of BackupSession
type BackupSessionStatus struct {
	SessionState `json:"state"`
	StartTime    *metav1.Time `json:"startTime"`
	// +optional
	Targets []TargetStatus `json:"target,omitempty"`
	Keep    string         `json:"keep"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName="bs"
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

//+kubebuilder:object:root=true

// BackupSessionList contains a list of BackupSession
type BackupSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSession{}, &BackupSessionList{})
}
