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
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

const (
	sessionState  string = ".metadata.state"
	finalizerName string = "finalizer.backupsession.formol.desmojim.fr"
)

// BackupSessionReconciler reconciles a BackupSession object
type BackupSessionReconciler struct {
	Session
}

//+kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions/finalizers,verbs=update

func (r *BackupSessionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	r.Context = ctx

	r.Log.V(1).Info("Enter Reconcile with req", "req", req, "reconciler", r)
	backupSession := formolv1alpha1.BackupSession{}
	err := r.Get(ctx, req.NamespacedName, &backupSession)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	// we might need the BackupConfiguration is the BackupSession
	// is being deleted
	backupConf := formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: backupSession.Spec.Ref.Namespace,
		Name:      backupSession.Spec.Ref.Name,
	}, &backupConf); err != nil {
		r.Log.Error(err, "unable to get BackupConfiguration")
		return ctrl.Result{}, err
	}
	if !backupSession.ObjectMeta.DeletionTimestamp.IsZero() {
		r.Log.V(0).Info("BackupSession is being deleted")
		if err := r.deleteSnapshots(backupSession, backupConf); err != nil {
			r.Log.Error(err, "unable to delete the BackupSession snapshots")
		}
		if controllerutil.ContainsFinalizer(&backupSession, finalizerName) {
			controllerutil.RemoveFinalizer(&backupSession, finalizerName)
			err := r.Update(ctx, &backupSession)
			if err != nil {
				r.Log.Error(err, "unable to remove finalizer")
			}
			return ctrl.Result{}, err
		}
	}
	if !controllerutil.ContainsFinalizer(&backupSession, finalizerName) {
		controllerutil.AddFinalizer(&backupSession, finalizerName)
		err := r.Update(ctx, &backupSession)
		if err != nil {
			r.Log.Error(err, "unable to add finalizer")
		}
		return ctrl.Result{}, err
	}

	var newSessionState formolv1alpha1.SessionState
	switch backupSession.Status.SessionState {
	case formolv1alpha1.New:
		// Go through the Targets and create the corresponding TargetStatus. Move to Initializing.
		if r.isBackupOngoing(backupConf) {
			r.Log.V(0).Info("there is an ongoing backup. Let's reschedule this operation")
			return ctrl.Result{
				RequeueAfter: 30 * time.Second,
			}, nil
		}
		backupSession.Status.Targets = r.initSession(backupConf)
		newSessionState = formolv1alpha1.Initializing
	case formolv1alpha1.Initializing:
		// Wait for all the Targets to be in the Initialized state then move them to Running and move to Running myself.
		// if one of the Target fails to initialize, move it back to New state and decrement Try.
		// if try reaches 0, move all the Targets to Finalize and move myself to Failure.
		newSessionState = r.checkInitialized(backupSession.Status.Targets, backupConf)
	case formolv1alpha1.Running:
		// Wait for all the target to be in Waiting state then move them to the Finalize state. Move myself to Finalize.
		// if one of the Target fails the backup, move it back to Running state and decrement Try.
		// if try reaches 0, move all the Targets to Finalize and move myself to Failure.
		newSessionState = r.checkWaiting(backupSession.Status.Targets, backupConf)
	case formolv1alpha1.Finalize:
		// Check the TargetStatus of all the Targets. If they are all Success then move myself to Success.
		// if one of the Target fails to Finalize, move it back to Finalize state and decrement Try.
		// if try reaches 0, move myself to Success because the backup was a Success even if the Finalize failed.
		if newSessionState = r.checkSuccess(backupSession.Status.Targets, backupConf); newSessionState == formolv1alpha1.Failure {
			r.Log.V(0).Info("One of the target did not manage to Finalize but the backup is still a Success")
			newSessionState = formolv1alpha1.Success
		}
	case formolv1alpha1.Success:
		r.cleanupSessions(backupConf)
		r.Log.V(0).Info("Backup was a success")

	case formolv1alpha1.Failure:
		r.Log.V(0).Info("Backup failed")

	default:
		// BackupSession has just been created
		newSessionState = formolv1alpha1.New
		backupSession.Status.StartTime = &metav1.Time{Time: time.Now()}
	}
	if newSessionState != "" {
		r.Log.V(0).Info("BackupSession needs a status update", "newSessionState", newSessionState, "backupSession", backupSession)
		backupSession.Status.SessionState = newSessionState
		if err := r.Status().Update(ctx, &backupSession); err != nil {
			r.Log.Error(err, "unable to update BackupSession.Status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&formolv1alpha1.BackupSession{},
		sessionState,
		func(rawObj client.Object) []string {
			session := rawObj.(*formolv1alpha1.BackupSession)
			return []string{
				string(session.Status.SessionState),
			}
		}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupSession{}).
		Complete(r)
}
