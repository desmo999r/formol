package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SessionState string

const (
	New      SessionState = "New"
	Init     SessionState = "Initializing"
	Running  SessionState = "Running"
	Finalize SessionState = "Finalizing"
	Success  SessionState = "Success"
	Failure  SessionState = "Failure"
	Deleted  SessionState = "Deleted"
	// Environment variables used by the sidecar container
	// the name of the sidecar container
	SIDECARCONTAINER_NAME string = "formol"
	// Used by both the backupsession and restoresession controllers to identified the target deployment
	TARGET_NAME string = "TARGET_NAME"
	// Used by restoresession controller
	RESTORESESSION_NAMESPACE string = "RESTORESESSION_NAMESPACE"
	RESTORESESSION_NAME      string = "RESTORESESSION_NAME"
	// Used by the backupsession controller
	POD_NAME      string = "POD_NAME"
	POD_NAMESPACE string = "POD_NAMESPACE"
)

type TargetStatus struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// +optional
	SessionState `json:"state,omitempty"`
	// +optional
	SnapshotId string `json:"snapshotId,omitempty"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`
	// +optional
	Try int `json:"try,omitemmpty"`
}
