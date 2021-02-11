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
	"time"

	formolrbac "github.com/desmo999r/formol/pkg/rbac"
	formolutils "github.com/desmo999r/formol/pkg/utils"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	kbatch_beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

// BackupConfigurationReconciler reconciles a BackupConfiguration object
type BackupConfigurationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *BackupConfigurationReconciler) getDeployment(namespace string, name string) (*appsv1.Deployment, error) {

	deployment := &appsv1.Deployment{}
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	return deployment, err
}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=*,verbs=*
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs/status,verbs=get

func (r *BackupConfigurationReconciler) deleteSidecarContainer(backupConf *formolv1alpha1.BackupConfiguration, target formolv1alpha1.Target) error {
	deployment, err := r.getDeployment(backupConf.Namespace, target.Name)
	if err != nil {
		return err
	}
	restorecontainers := []corev1.Container{}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "backup" {
			continue
		}
		restorecontainers = append(restorecontainers, container)
	}
	deployment.Spec.Template.Spec.Containers = restorecontainers
	if err := r.Update(context.Background(), deployment); err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	if err != nil {
		return nil
	}
	pods := &corev1.PodList{}
	err = r.List(context.Background(), pods, client.MatchingLabels(selector))
	if err != nil {
		return nil
	}
	replicasToDelete := []appsv1.ReplicaSet{}
	for _, pod := range pods.Items {
		for _, podRef := range pod.OwnerReferences {
			rs := &appsv1.ReplicaSet{}
			if err := r.Get(context.Background(), client.ObjectKey{
				Name:      podRef.Name,
				Namespace: pod.Namespace,
			}, rs); err != nil {
				return nil
			}
			for _, rsRef := range rs.OwnerReferences {
				if rsRef.Kind == deployment.Kind && rsRef.Name == deployment.Name {
					replicasToDelete = append(replicasToDelete, *rs)
				}
			}
		}
	}

	for _, replica := range replicasToDelete {
		if err := r.Delete(context.TODO(), &replica); err != nil {
			return nil
		}
	}
	return nil
}

func (r *BackupConfigurationReconciler) addSidecarContainer(backupConf *formolv1alpha1.BackupConfiguration, target formolv1alpha1.Target) error {
	log := r.Log.WithValues("backupconf", backupConf.Name)
	deployment, err := r.getDeployment(backupConf.Namespace, target.Name)
	if err != nil {
		log.Error(err, "unable to get Deployment")
		return err
	}
	log.V(1).Info("got deployment", "Deployment", deployment)
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "backup" {
			log.V(0).Info("There is already a backup sidecar container. Skipping", "container", container)
			return nil
		}
	}
	sidecar := corev1.Container{
		Name:  "backup",
		Image: "desmo999r/formolcli:latest",
		Args:  []string{"backupsession", "server"},
		//Image: "busybox",
		//Command: []string{
		//	"sh",
		//	"-c",
		//	"sleep 3600; echo done",
		//},
		Env: []corev1.EnvVar{
			corev1.EnvVar{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			corev1.EnvVar{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			corev1.EnvVar{
				Name:  "POD_DEPLOYMENT",
				Value: target.Name,
			},
		},
		VolumeMounts: []corev1.VolumeMount{},
	}

	// Gather information from the repo
	repo := &formolv1alpha1.Repo{}
	if err := r.Get(context.Background(), client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      backupConf.Spec.Repository.Name,
	}, repo); err != nil {
		log.Error(err, "unable to get Repo from BackupConfiguration")
		return err
	}
	sidecar.Env = append(sidecar.Env, formolutils.ConfigureResticEnvVar(backupConf, repo)...)

	for _, volumemount := range target.VolumeMounts {
		log.V(1).Info("mounts", "volumemount", volumemount)
		volumemount.ReadOnly = true
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, *volumemount.DeepCopy())
	}
	selector, err := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	if err != nil {
		log.Error(err, "unable to get LableSelector for deployment", "label", deployment.Spec.Selector)
		return nil
	}
	log.V(1).Info("getting pods matching label", "label", selector)
	pods := &corev1.PodList{}
	err = r.List(context.Background(), pods, client.InNamespace(backupConf.Namespace), client.MatchingLabels(selector))
	if err != nil {
		log.Error(err, "unable to get deployment pods")
		return nil
	}
	replicasToDelete := []appsv1.ReplicaSet{}
	log.V(1).Info("got that list of pods", "pods", len(pods.Items))
	for _, pod := range pods.Items {
		log.V(1).Info("checking pod", "pod", pod)
		for _, podRef := range pod.OwnerReferences {
			rs := &appsv1.ReplicaSet{}
			if err := r.Get(context.Background(), client.ObjectKey{
				Name:      podRef.Name,
				Namespace: pod.Namespace,
			}, rs); err != nil {
				log.Error(err, "unable to get replicaset", "replicaset", podRef.Name)
				return nil
			}
			log.V(1).Info("got a replicaset", "rs", rs.Name)
			for _, rsRef := range rs.OwnerReferences {
				if rsRef.Kind == deployment.Kind && rsRef.Name == deployment.Name {
					log.V(0).Info("Adding pod to the list of pods to be restarted", "pod", pod.Name)
					replicasToDelete = append(replicasToDelete, *rs)
				}
			}
		}
	}
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, sidecar)
	deployment.Spec.Template.Spec.ShareProcessNamespace = func() *bool { b := true; return &b }()

	if err := formolrbac.CreateFormolRBAC(r.Client, deployment.Spec.Template.Spec.ServiceAccountName, deployment.Namespace); err != nil {
		log.Error(err, "unable to create backupsessionlistener RBAC")
		return nil
	}

	log.V(0).Info("Adding a sicar container")
	if err := r.Update(context.Background(), deployment); err != nil {
		log.Error(err, "unable to update the Deployment")
		return err
	}
	for _, replica := range replicasToDelete {
		if err := r.Delete(context.TODO(), &replica); err != nil {
			log.Error(err, "unable to delete replica", "replica", replica.Name)
			return nil
		}
	}
	return nil
}

func (r *BackupConfigurationReconciler) addCronJob(backupConf *formolv1alpha1.BackupConfiguration) error {
	log := r.Log.WithValues("addCronJob", backupConf.Name)

	if err := formolrbac.CreateFormolRBAC(r.Client, "default", backupConf.Namespace); err != nil {
		log.Error(err, "unable to create backupsessionlistener RBAC")
		return nil
	}

	if err := formolrbac.CreateBackupSessionCreatorRBAC(r.Client, backupConf.Namespace); err != nil {
		log.Error(err, "unable to create backupsession-creator RBAC")
		return nil
	}

	cronjob := &kbatch_beta1.CronJob{}
	if err := r.Get(context.Background(), client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      "backup-" + backupConf.Name,
	}, cronjob); err == nil {
		log.V(0).Info("there is already a cronjob")
		var changed bool
		if backupConf.Spec.Schedule != cronjob.Spec.Schedule {
			log.V(0).Info("cronjob schedule has changed", "old schedule", cronjob.Spec.Schedule, "new schedule", backupConf.Spec.Schedule)
			cronjob.Spec.Schedule = backupConf.Spec.Schedule
			changed = true
		}
		if backupConf.Spec.Suspend != cronjob.Spec.Suspend {
			log.V(0).Info("cronjob suspend has changed", "before", cronjob.Spec.Suspend, "new", backupConf.Spec.Suspend)
			cronjob.Spec.Suspend = backupConf.Spec.Suspend
			changed = true
		}
		if changed == true {
			if err := r.Update(context.TODO(), cronjob); err != nil {
				log.Error(err, "unable to update cronjob definition")
				return err
			}
		}
		return nil
	} else if errors.IsNotFound(err) == false {
		log.Error(err, "something went wrong")
		return err
	}

	cronjob = &kbatch_beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-" + backupConf.Name,
			Namespace: backupConf.Namespace,
		},
		Spec: kbatch_beta1.CronJobSpec{
			Suspend:  backupConf.Spec.Suspend,
			Schedule: backupConf.Spec.Schedule,
			JobTemplate: kbatch_beta1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							ServiceAccountName: "backupsession-creator",
							Containers: []corev1.Container{
								corev1.Container{
									Name:  "job-createbackupsession-" + backupConf.Name,
									Image: "desmo999r/formolcli:latest",
									Args: []string{
										"backupsession",
										"create",
										"--namespace",
										backupConf.Namespace,
										"--name",
										backupConf.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(backupConf, cronjob, r.Scheme); err != nil {
		log.Error(err, "unable to set controller on job", "cronjob", cronjob, "backupconf", backupConf)
		return err
	}
	log.V(0).Info("creating the cronjob")
	if err := r.Create(context.Background(), cronjob); err != nil {
		log.Error(err, "unable to create the cronjob", "cronjob", cronjob)
		return err
	}
	return nil
}

func (r *BackupConfigurationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("backupconfiguration", req.NamespacedName)
	time.Sleep(300 * time.Millisecond)

	log.V(1).Info("Enter Reconcile with req", "req", req)

	// your logic here
	backupConf := &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, backupConf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := "finalizer.backupconfiguration.formol.desmojim.fr"

	if backupConf.ObjectMeta.DeletionTimestamp.IsZero() {
		if !formolutils.ContainsString(backupConf.ObjectMeta.Finalizers, finalizerName) {
			backupConf.ObjectMeta.Finalizers = append(backupConf.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), backupConf); err != nil {
				log.Error(err, "unable to append finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		log.V(0).Info("backupconf being deleted", "backupconf", backupConf.Name)
		if formolutils.ContainsString(backupConf.ObjectMeta.Finalizers, finalizerName) {
			if err := r.deleteExternalResources(backupConf); err != nil {
				return ctrl.Result{}, err
			}
		}
		backupConf.ObjectMeta.Finalizers = formolutils.RemoveString(backupConf.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(context.Background(), backupConf); err != nil {
			log.Error(err, "unable to remove finalizer")
			return ctrl.Result{}, err
		}
		// We have been deleted. Return here
		return ctrl.Result{}, nil
	}

	if err := r.addCronJob(backupConf); err != nil {
		return ctrl.Result{}, nil
	}
	backupConf.Status.ActiveCronJob = true

	for _, target := range backupConf.Spec.Targets {
		switch target.Kind {
		case "Deployment":
			if err := r.addSidecarContainer(backupConf, target); err != nil {
				return ctrl.Result{}, nil
			}
			backupConf.Status.ActiveSidecar = true
		case "PersistentVolumeClaim":
			log.V(0).Info("TODO backup PVC")
			return ctrl.Result{}, nil
		}
	}

	backupConf.Status.Suspended = false
	log.V(1).Info("updating backupconf")
	if err := r.Status().Update(ctx, backupConf); err != nil {
		log.Error(err, "unable to update backupconf", "backupconf", backupConf)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BackupConfigurationReconciler) deleteExternalResources(backupConf *formolv1alpha1.BackupConfiguration) error {
	for _, target := range backupConf.Spec.Targets {
		switch target.Kind {
		case "Deployment":
			deployment, err := r.getDeployment(backupConf.Namespace, target.Name)
			if err != nil {
				return err
			}
			if err := formolrbac.DeleteFormolRBAC(r.Client, deployment.Spec.Template.Spec.ServiceAccountName, deployment.Namespace); err != nil {
				return err
			}
			if err := formolrbac.DeleteBackupSessionCreatorRBAC(r.Client, backupConf.Namespace); err != nil {
				return err
			}
			if err := r.deleteSidecarContainer(backupConf, target); err != nil {
				return err
			}
		}
	}
	// TODO: remove the hardcoded "default"
	if err := formolrbac.DeleteFormolRBAC(r.Client, "default", backupConf.Namespace); err != nil {
		return err
	}
	if err := formolrbac.DeleteBackupSessionStatusUpdaterRBAC(r.Client, "default", backupConf.Namespace); err != nil {
		return err
	}
	if err := formolrbac.DeleteBackupSessionCreatorRBAC(r.Client, backupConf.Namespace); err != nil {
		return err
	}
	return nil
}

func (r *BackupConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupConfiguration{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		WithEventFilter(predicate.GenerationChangedPredicate{}). // Don't reconcile when status gets updated
		//Owns(&formolv1alpha1.BackupSession{}).
		Owns(&kbatch_beta1.CronJob{}).
		Complete(r)
}
