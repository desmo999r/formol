package controllers

import (
	"context"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type Session struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	context.Context
	Namespace string
}

func (s Session) isBackupOngoing(backupConf formolv1alpha1.BackupConfiguration) bool {
	backupSessionList := &formolv1alpha1.BackupSessionList{}
	if err := s.List(s.Context, backupSessionList,
		client.InNamespace(backupConf.Namespace), client.MatchingFieldsSelector{
			Selector: fields.SelectorFromSet(fields.Set{
				sessionState: "Running",
			}),
		}); err != nil {
		s.Log.Error(err, "unable to get backupsessionlist")
		return true
	}
	return len(backupSessionList.Items) > 0
}

func (s Session) initSession(backupConf formolv1alpha1.BackupConfiguration) []formolv1alpha1.TargetStatus {
	tss := []formolv1alpha1.TargetStatus{}
	for _, target := range backupConf.Spec.Targets {
		s.Log.V(0).Info("Creating new target", "target", target.TargetName)
		tss = append(tss, formolv1alpha1.TargetStatus{
			BackupType:   target.BackupType,
			TargetName:   target.TargetName,
			TargetKind:   target.TargetKind,
			SessionState: "",
			StartTime:    &metav1.Time{Time: time.Now()},
			Try:          1,
		})
	}
	return tss
}

func (s Session) checkSessionState(
	tss []formolv1alpha1.TargetStatus,
	backupConf formolv1alpha1.BackupConfiguration,
	currentState formolv1alpha1.SessionState,
	waitState formolv1alpha1.SessionState,
	nextState formolv1alpha1.SessionState) (sessionState formolv1alpha1.SessionState) {
	for i, targetStatus := range tss {
		s.Log.V(0).Info("Target status", "target", targetStatus.TargetName, "session state", targetStatus.SessionState)
		switch targetStatus.SessionState {
		case currentState:
			s.Log.V(0).Info("Move target to waitState", "target", targetStatus.TargetName, "waitState", waitState)
			tss[i].SessionState = waitState
			return waitState
		case formolv1alpha1.Failure:
			if targetStatus.Try < backupConf.Spec.Targets[i].Retry {
				s.Log.V(0).Info("Target failed. Try one more time", "target", targetStatus.TargetName, "waitState", waitState)
				tss[i].SessionState = waitState
				tss[i].Try++
				tss[i].StartTime = &metav1.Time{Time: time.Now()}
				return waitState
			} else {
				s.Log.V(0).Info("Target failed for the last time", "target", targetStatus.TargetName)
				return formolv1alpha1.Failure
			}
		case waitState:
			// target is still busy with its current state. Wait until it is done.
			s.Log.V(0).Info("Waiting for one target to finish", "waitState", waitState)
			return ""
		case formolv1alpha1.WaitingForJob:
			// SnapshotKind special case
			// A Job is scheduled to do the backup from a Volume Snapshot. It might take some time.
			// We still want to run Finalize for all the targets (continue)
			// but we also don't want to move the global BackupSession to Success (rewrite sessionState)
			// When the Job is over, it will move the target state to Finalized and we'll be fine
			defer func() { sessionState = "" }()
			continue
		default:
			if i == len(tss)-1 {
				s.Log.V(0).Info("Moving to next state", "nextState", nextState)
				return nextState
			}
		}
	}
	return ""
}

func (s Session) checkInitialized(tss []formolv1alpha1.TargetStatus, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return s.checkSessionState(tss, backupConf, "", formolv1alpha1.Initializing, formolv1alpha1.Running)
}

func (s Session) checkWaiting(tss []formolv1alpha1.TargetStatus, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return s.checkSessionState(tss, backupConf, formolv1alpha1.Initialized, formolv1alpha1.Running, formolv1alpha1.Finalize)
}

func (s Session) checkSuccess(tss []formolv1alpha1.TargetStatus, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return s.checkSessionState(tss, backupConf, formolv1alpha1.Waiting, formolv1alpha1.Finalize, formolv1alpha1.Success)
}
