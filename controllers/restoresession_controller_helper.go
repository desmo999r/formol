package controllers

import (
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

func (r *RestoreSessionReconciler) initRestore(restoreSession *formolv1alpha1.RestoreSession, backupConf formolv1alpha1.BackupConfiguration) formolv1alpha1.SessionState {
	return formolv1alpha1.Running
}
