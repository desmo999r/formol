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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BackupSessionRef struct {
	Spec   BackupSessionSpec   `json:"spec"`
	Status BackupSessionStatus `json:"status"`
}

// RestoreSessionSpec defines the desired state of RestoreSession
type RestoreSessionSpec struct {
	BackupSessionRef `json:"backupSession"`
}

// RestoreSessionStatus defines the observed state of RestoreSession
type RestoreSessionStatus struct {
	SessionState `json:"state,omitempty"`
	StartTime    *metav1.Time   `json:"startTime,omitempty"`
	Targets      []TargetStatus `json:"targets,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RestoreSession is the Schema for the restoresessions API
type RestoreSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreSessionSpec   `json:"spec,omitempty"`
	Status RestoreSessionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RestoreSessionList contains a list of RestoreSession
type RestoreSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestoreSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestoreSession{}, &RestoreSessionList{})
}
