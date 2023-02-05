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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
	context.Context
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

	switch backupSession.Status.SessionState {
	case formolv1alpha1.New:
		if r.isBackupOngoing(backupConf) {
			r.Log.V(0).Info("there is an ongoing backup. Let's reschedule this operation")
			return ctrl.Result{
				RequeueAfter: 30 * time.Second,
			}, nil
		}
	default:
		// BackupSession has just been created
		backupSession.Status.SessionState = formolv1alpha1.New
		backupSession.Status.StartTime = &metav1.Time{Time: time.Now()}
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
