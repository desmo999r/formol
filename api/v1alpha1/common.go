package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SessionState string

const (
	New     SessionState = "New"
	Running SessionState = "Running"
	Success SessionState = "Success"
	Failure SessionState = "Failure"
	Deleted SessionState = "Deleted"
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
}
