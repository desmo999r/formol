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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

// BackupConfigurationReconciler reconciles a BackupConfiguration object
type BackupConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
	context.Context
}

//+kubebuilder:rbac:groups=formol.desmojim.fr,resources=*,verbs=*
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=volumesnapshotclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs/status,verbs=get
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=persistentvolumes,verbs=get;list;watch
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshotclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete

func (r *BackupConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Context = ctx
	r.Log = log.FromContext(ctx)

	r.Log.V(1).Info("Enter Reconcile with req", "req", req, "reconciler", r)

	backupConf := formolv1alpha1.BackupConfiguration{}
	err := r.Get(ctx, req.NamespacedName, &backupConf)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	finalizerName := "finalizer.backupconfiguration.formol.desmojim.fr"

	if !backupConf.ObjectMeta.DeletionTimestamp.IsZero() {
		r.Log.V(0).Info("backupconf being deleted", "backupconf", backupConf.ObjectMeta.Finalizers)
		if controllerutil.ContainsFinalizer(&backupConf, finalizerName) {
			_ = r.DeleteSidecar(backupConf)
			_ = r.DeleteCronJob(backupConf)
			_ = r.deleteRBACSidecar(backupConf.Namespace)
			controllerutil.RemoveFinalizer(&backupConf, finalizerName)
			if err := r.Update(ctx, &backupConf); err != nil {
				r.Log.Error(err, "unable to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		// We have been deleted. Return here
		r.Log.V(0).Info("backupconf deleted", "backupconf", backupConf.Name)
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&backupConf, finalizerName) {
		r.Log.V(0).Info("adding finalizer", "backupconf", backupConf)
		controllerutil.AddFinalizer(&backupConf, finalizerName)
		if err := r.Update(ctx, &backupConf); err != nil {
			r.Log.Error(err, "unable to append finalizer")
			return ctrl.Result{}, err
		}
		// backupConf has been updated. Exit here. The reconciler will be called again so we can finish the job.
		return ctrl.Result{}, nil
	}

	if err := r.AddCronJob(backupConf); err != nil {
		return ctrl.Result{}, err
	} else {
		backupConf.Status.ActiveCronJob = true
	}

	for _, target := range backupConf.Spec.Targets {
		if err := r.addSidecar(backupConf, target); err != nil {
			r.Log.Error(err, "unable to add online sidecar")
			return ctrl.Result{}, err
		}
		backupConf.Status.ActiveSidecar = true
	}

	if err := r.Status().Update(ctx, &backupConf); err != nil {
		r.Log.Error(err, "Unable to update BackupConfiguration status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupConfiguration{}).
		Complete(r)
}
