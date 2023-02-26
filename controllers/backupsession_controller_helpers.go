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

package controllers

import (
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func (r *BackupSessionReconciler) isBackupOngoing(backupConf formolv1alpha1.BackupConfiguration) bool {
	backupSessionList := &formolv1alpha1.BackupSessionList{}
	if err := r.List(r.Context, backupSessionList,
		client.InNamespace(backupConf.Namespace), client.MatchingFieldsSelector{
			Selector: fields.SelectorFromSet(fields.Set{
				sessionState: "Running",
			}),
		}); err != nil {
		r.Log.Error(err, "unable to get backupsessionlist")
		return true
	}
	return len(backupSessionList.Items) > 0
}

func (r *BackupSessionReconciler) initBackup(backupSession *formolv1alpha1.BackupSession, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	for _, target := range backupConf.Spec.Targets {
		r.Log.V(0).Info("Creating new target", "target", target.TargetName)
		backupSession.Status.Targets = append(backupSession.Status.Targets, formolv1alpha1.TargetStatus{
			BackupType:   target.BackupType,
			TargetName:   target.TargetName,
			TargetKind:   target.TargetKind,
			SessionState: "",
			StartTime:    &metav1.Time{Time: time.Now()},
			Try:          1,
		})
	}
	return formolv1alpha1.Initializing
}

func (r *BackupSessionReconciler) checkSessionState(
	backupSession *formolv1alpha1.BackupSession,
	backupConf formolv1alpha1.BackupConfiguration,
	currentState formolv1alpha1.SessionState,
	waitState formolv1alpha1.SessionState,
	nextState formolv1alpha1.SessionState) formolv1alpha1.SessionState {
	for i, targetStatus := range backupSession.Status.Targets {
		r.Log.V(0).Info("Target status", "target", targetStatus.TargetName, "session state", targetStatus.SessionState)
		switch targetStatus.SessionState {
		case currentState:
			r.Log.V(0).Info("Move target to waitState", "target", targetStatus.TargetName, "waitState", waitState)
			backupSession.Status.Targets[i].SessionState = waitState
			return waitState
		case formolv1alpha1.Failure:
			if targetStatus.Try < backupConf.Spec.Targets[i].Retry {
				r.Log.V(0).Info("Target failed. Try one more time", "target", targetStatus.TargetName, "waitState", waitState)
				backupSession.Status.Targets[i].SessionState = waitState
				backupSession.Status.Targets[i].Try++
				backupSession.Status.Targets[i].StartTime = &metav1.Time{Time: time.Now()}
				return waitState
			} else {
				r.Log.V(0).Info("Target failed for the last time", "target", targetStatus.TargetName)
				return formolv1alpha1.Failure
			}
		case waitState:
			// target is still busy with its current state. Wait until it is done.
			r.Log.V(0).Info("Waiting for one target to finish", "waitState", waitState)
			return ""
		default:
			if i == len(backupSession.Status.Targets)-1 {
				r.Log.V(0).Info("Moving to next state", "nextState", nextState)
				return nextState
			}
		}
	}
	return ""
}

func (r *BackupSessionReconciler) checkInitialized(backupSession *formolv1alpha1.BackupSession, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return r.checkSessionState(backupSession, backupConf, "", formolv1alpha1.Initializing, formolv1alpha1.Running)
}

func (r *BackupSessionReconciler) checkWaiting(backupSession *formolv1alpha1.BackupSession, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return r.checkSessionState(backupSession, backupConf, formolv1alpha1.Initialized, formolv1alpha1.Running, formolv1alpha1.Finalize)
}

func (r *BackupSessionReconciler) checkSuccess(backupSession *formolv1alpha1.BackupSession, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return r.checkSessionState(backupSession, backupConf, formolv1alpha1.Waiting, formolv1alpha1.Finalize, formolv1alpha1.Success)
}
