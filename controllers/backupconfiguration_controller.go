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
	batchv1 "k8s.io/api/batch/v1"
	kbatch_beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	//	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=repoes,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs/status,verbs=get

func (r *BackupConfigurationReconciler) addSidecarContainer(backupConf *formolv1alpha1.BackupConfiguration) error {
	log := r.Log.WithValues("backupconf", backupConf.Name)
	getDeployment := func() (*appsv1.Deployment, error) {
		deployment := &appsv1.Deployment{}
		err := r.Get(context.Background(), client.ObjectKey{
			Namespace: backupConf.Namespace,
			Name:      backupConf.Spec.Target.Name,
		}, deployment)
		return deployment, err
	}

	deployment, err := getDeployment()
	if err != nil {
		log.Error(err, "unable to get Deployment")
		return err
	}
	log.WithValues("Deployment", backupConf.Spec.Target.Name)
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "backup" {
			log.V(0).Info("There is already a backup sidecar container. Skipping", "container", container)
			return nil
		}
	}
	repo := &formolv1alpha1.Repo{}
	sidecar := corev1.Container{
		Name:  "backup",
		Image: "desmo999r/formolcli:latest",
		Args:  []string{"create", "server"},
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
		},
		VolumeMounts: []corev1.VolumeMount{},
	}

	// Gather information from the repo
	if err := r.Get(context.Background(), client.ObjectKey{
		Namespace: backupConf.Namespace,
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

	for _, volumemount := range backupConf.Spec.VolumeMounts {
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
	err = r.List(context.Background(), pods, client.MatchingLabels(selector))
	if err != nil {
		log.Error(err, "unable to get deployment pods")
		return nil
	}
	podsToDelete := []appsv1.ReplicaSet{}
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
					podsToDelete = append(podsToDelete, *rs)
				}
			}
		}
	}
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, sidecar)
	log.V(0).Info("Adding a sicar container")
	if err := r.Update(context.Background(), deployment); err != nil {
		log.Error(err, "unable to update the Deployment")
		return err
	}
	for _, pod := range podsToDelete {
		if err := r.Delete(context.TODO(), &pod); err != nil {
			log.Error(err, "unable to delete pod", "pod", pod.Name)
			return nil
		}
	}
	deployment, err = getDeployment()
	if err != nil {
		log.Error(err, "unable to get Deployment")
		return err
	}
	return nil
}

func (r *BackupConfigurationReconciler) addCronJob(backupConf *formolv1alpha1.BackupConfiguration) error {
	log := r.Log.WithName("addCronJob")

	//	serviceaccount := &corev1.ServiceAccount{
	//		ObjectMeta: metav1.ObjectMeta{
	//			Namespace: backupConf.Namespace,
	//			Name:      "backupsession-creator",
	//		},
	//	}
	//	if err := r.Get(context.Background(), client.ObjectKey{
	//		Namespace: backupConf.Namespace,
	//		Name:      "backupsession-creator",
	//	}, serviceaccount); err != nil && errors.IsNotFound(err) {
	//		log.V(0).Info("creating service account", "service account", serviceaccount)
	//		if err = r.Create(context.Background(), serviceaccount); err != nil {
	//			log.Error(err, "unable to create serviceaccount", "serviceaccount", serviceaccount)
	//			return nil
	//		}
	//	}
	//	rolebinding := &rbacv1.RoleBinding{
	//		ObjectMeta: metav1.ObjectMeta{
	//			Namespace: backupConf.Namespace,
	//			Name:      "backupsession-creator-rolebinding",
	//		},
	//		Subjects: []rbacv1.Subject{
	//			rbacv1.Subject{
	//				Kind: "ServiceAccount",
	//				Name: "backupsession-creator",
	//			},
	//		},
	//		RoleRef: rbacv1.RoleRef{
	//			APIGroup: "rbac.authorization.k8s.io",
	//			Kind:     "ClusterRole",
	//			Name:     "backupsession-creator",
	//		},
	//	}
	//	if err := r.Get(context.Background(), client.ObjectKey{
	//		Namespace: backupConf.Namespace,
	//		Name:      "backupsession-creator-rolebinding",
	//	}, rolebinding); err != nil && errors.IsNotFound(err) {
	//		log.V(0).Info("creating role binding for service account", "rolebinding", rolebinding, "service account", serviceaccount)
	//		if err = r.Create(context.Background(), rolebinding); err != nil {
	//			log.Error(err, "unable to create rolebinding", "rolebinding", rolebinding)
	//			return nil
	//		}
	//	}

	cronjob := &kbatch_beta1.CronJob{}
	if err := r.Get(context.Background(), client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      "backup-" + backupConf.Name,
	}, cronjob); err == nil {
		log.V(0).Info("there is already a cronjob", "cronjob", cronjob, "backupconf", backupConf.Name)
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
										"create",
										"backupsession",
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

	log.V(1).Info("Enter Reconcile with req", "req", req)

	// your logic here
	backupConf := &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, backupConf); err != nil {
		log.Error(err, "unable to fetch BackupConfiguration")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.addCronJob(backupConf); err != nil {
		return ctrl.Result{}, nil
	}
	backupConf.Status.ActiveCronJob = true

	switch backupConf.Spec.Target.Kind {
	case "Deployment":
		if err := r.addSidecarContainer(backupConf); err != nil {
			return ctrl.Result{}, nil
		}
		backupConf.Status.ActiveSidecar = true
	case "PersistentVolumeClaim":
		log.V(0).Info("TODO backup PVC")
		return ctrl.Result{}, nil
	}

	if backupConf.Spec.Suspend != nil && *backupConf.Spec.Suspend == true {
		log.V(0).Info("We are suspended return and wait for the next event")
		// TODO Suspend the CronJob
		return ctrl.Result{}, nil
	}

	log.V(1).Info("updating backupconf")
	if err := r.Status().Update(ctx, backupConf); err != nil {
		log.Error(err, "unable to update backupconf", "backupconf", backupConf)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BackupConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupConfiguration{}).
		Owns(&kbatch_beta1.CronJob{}).
		Complete(r)
}
