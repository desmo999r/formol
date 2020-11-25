/*


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
	kbatch_beta1 "k8s.io/api/batch/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

// BackupConfigurationReconciler reconciles a BackupConfiguration object
type BackupConfigurationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupconfigurations/status,verbs=get;update;patch

func (r *BackupConfigurationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("backupconfiguration", req.NamespacedName)

	log.V(1).Info("Enter Reconcile with req", "req", req)

	// your logic here
	var backupConf formolv1alpha1.BackupConfiguration
	if err := r.Get(ctx, req.NamespacedName, &backupConf); err != nil {
		log.Error(err, "unable to fetch BackupConfiguration")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if backupConf.Spec.Suspend != nil && *backupConf.Spec.Suspend == true {
		log.V(0).Info("We are suspended return and wait for the next event")
		// TODO Suspend the CronJob
		return ctrl.Result{}, nil
	}
	backupConf.Status.Suspended = backupConf.Spec.Suspend

	return ctrl.Result{}, nil
}

func (r *BackupConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupConfiguration{}).
		Owns(&kbatch_beta1.CronJob{}).
		Complete(r)
}
