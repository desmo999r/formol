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
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (r *BackupSessionReconciler) startNextTask(backupSession formolv1alpha1.BackupSession, backupConf formolv1alpha1.BackupConfiguration) (formolv1alpha1.TargetStatus, error) {
	nextTargetIndex := len(backupSession.Status.Targets)
	if nextTargetIndex < len(backupConf.Spec.Targets) {
		nextTarget := backupConf.Spec.Targets[nextTargetIndex]
	}
	return nil, nil
}
