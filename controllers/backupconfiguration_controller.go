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
	appsv1 "k8s.io/api/apps/v1"
	kbatch_beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (r *BackupConfigurationReconciler) addSidecarContainer(backupConf *formolv1alpha1.BackupConfiguration) error {
	log := r.Log.WithValues("Repository", backupConf.Spec.Repository.Name)
	repo := &formolv1alpha1.Repo{}
	sidecar := corev1.Container{
		Name:    "backup",
		Image:   "busybox",
		Command: []string{"sh", "-c", "echo Toto; sleep 3600"},
		Env: []corev1.EnvVar{
			corev1.EnvVar{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{},
	}

	// Gather information from the repo
	if err := r.Get(context.Background(), client.ObjectKey{
		Namespace: "backup",
		Name:      backupConf.Spec.Repository.Name,
	}, repo); err != nil {
		log.Error(err, "unable to get Repo from BackupConfiguration")
		return err
	}
	// S3 backing storage
	if (formolv1alpha1.S3{}) != repo.Spec.Backend.S3 {
		url := "s3:http://" + repo.Spec.Backend.S3.Server + "/" + repo.Spec.Backend.S3.Bucket + "/" + backupConf.Spec.Target.Name
		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "RESTIC_REPOSITORY",
			Value: url,
		})
		for _, key := range []string{
			"AWS_ACCESS_KEY_ID",
			"AWS_SECRET_ACCESS_KEY",
			"RESTIC_PASSWORD",
		} {
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{
				Name: key,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: repo.Spec.RepositorySecrets,
						},
						Key: key,
					},
				},
			})
		}
	}

	log.WithValues("Deployment", backupConf.Spec.Target.Name)
	deployment := &appsv1.Deployment{}
	if err := r.Get(context.Background(), client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      backupConf.Spec.Target.Name,
	}, deployment); err != nil {
		log.Error(err, "unable to fetch Deployment")
		return client.IgnoreNotFound(err)
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "backup" {
			log.V(0).Info("There is already a backup sidecar container. Skipping", "container", container)
			return nil
		}
	}
	for _, volumemount := range backupConf.Spec.VolumeMounts {
		log.V(1).Info("mounts", "volumemount", volumemount)
		volumemount.ReadOnly = true
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, *volumemount.DeepCopy())
	}
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, sidecar)
	log.V(0).Info("Adding a sicar container")
	if err := r.Update(context.Background(), deployment); err != nil {
		log.Error(err, "unable to update the Deployment")
		return err
	}
	return nil
}

func (r *BackupConfigurationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("backupconfiguration", req.NamespacedName)

	log.V(1).Info("Enter Reconcile with req", "req", req)

	// your logic here
	backupConf := &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, backupConf); err != nil {
		log.Error(err, "unable to fetch BackupConfiguration")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	switch backupConf.Spec.Target.Kind {
	case "Deployment":
		if err := r.addSidecarContainer(backupConf); err != nil {
			return ctrl.Result{}, nil
		}
	case "PersistentVolumeClaim":
		log.V(0).Info("TODO backup PVC")
		return ctrl.Result{}, nil
	}

	if backupConf.Spec.Suspend != nil && *backupConf.Spec.Suspend == true {
		log.V(0).Info("We are suspended return and wait for the next event")
		// TODO Suspend the CronJob
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *BackupConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupConfiguration{}).
		Owns(&kbatch_beta1.CronJob{}).
		Complete(r)
}
