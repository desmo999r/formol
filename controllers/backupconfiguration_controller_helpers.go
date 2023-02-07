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
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

func (r *BackupConfigurationReconciler) DeleteCronJob(backupConf formolv1alpha1.BackupConfiguration) error {
	cronjob := &batchv1.CronJob{}
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      "backup-" + backupConf.Name,
	}, cronjob); err == nil {
		r.Log.V(0).Info("Deleting cronjob", "cronjob", cronjob.Name)
		return r.Delete(r.Context, cronjob)
	} else {
		return err
	}
}

func (r *BackupConfigurationReconciler) AddCronJob(backupConf formolv1alpha1.BackupConfiguration) error {
	cronjob := &batchv1.CronJob{}
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      "backup-" + backupConf.Name,
	}, cronjob); err == nil {
		r.Log.V(0).Info("there is already a cronjob")
		var changed bool
		if backupConf.Spec.Schedule != cronjob.Spec.Schedule {
			r.Log.V(0).Info("cronjob schedule has changed", "old schedule", cronjob.Spec.Schedule, "new schedule", backupConf.Spec.Schedule)
			cronjob.Spec.Schedule = backupConf.Spec.Schedule
			changed = true
		}
		if backupConf.Spec.Suspend != nil && backupConf.Spec.Suspend != cronjob.Spec.Suspend {
			r.Log.V(0).Info("cronjob suspend has changed", "before", cronjob.Spec.Suspend, "new", backupConf.Spec.Suspend)
			cronjob.Spec.Suspend = backupConf.Spec.Suspend
			changed = true
		}
		if changed == true {
			if err := r.Update(r.Context, cronjob); err != nil {
				r.Log.Error(err, "unable to update cronjob definition")
				return err
			}
			backupConf.Status.Suspended = *backupConf.Spec.Suspend
		}
		return nil
	} else if errors.IsNotFound(err) == false {
		r.Log.Error(err, "something went wrong")
		return err
	}

	cronjob = &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-" + backupConf.Name,
			Namespace: backupConf.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Suspend:  backupConf.Spec.Suspend,
			Schedule: backupConf.Spec.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							ServiceAccountName: "backupsession-creator",
							Containers: []corev1.Container{
								corev1.Container{
									Name:  "job-createbackupsession-" + backupConf.Name,
									Image: backupConf.Spec.Image,
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
	if err := ctrl.SetControllerReference(&backupConf, cronjob, r.Scheme); err != nil {
		r.Log.Error(err, "unable to set controller on job", "cronjob", cronjob, "backupconf", backupConf)
		return err
	}
	r.Log.V(0).Info("creating the cronjob")
	if err := r.Create(r.Context, cronjob); err != nil {
		r.Log.Error(err, "unable to create the cronjob", "cronjob", cronjob)
		return err
	} else {
		backupConf.Status.Suspended = *backupConf.Spec.Suspend
		return nil
	}
}

func (r *BackupConfigurationReconciler) DeleteSidecar(backupConf formolv1alpha1.BackupConfiguration) error {
	removeTags := func(podSpec *corev1.PodSpec, target formolv1alpha1.Target) {
		for i, container := range podSpec.Containers {
			for _, targetContainer := range target.Containers {
				if targetContainer.Name == container.Name {
					if len(container.Env) > 1 && container.Env[len(container.Env)-1].Name == formolv1alpha1.TARGETCONTAINER_TAG {
						podSpec.Containers[i].Env = container.Env[:len(container.Env)-1]
					} else {
						for j, e := range container.Env {
							if e.Name == formolv1alpha1.TARGETCONTAINER_TAG {
								container.Env[j] = container.Env[len(container.Env)-1]
								podSpec.Containers[i].Env = container.Env[:len(container.Env)-1]
								break
							}
						}
					}
				}
			}
		}
	}
	for _, target := range backupConf.Spec.Targets {
		switch target.TargetKind {
		case formolv1alpha1.Deployment:
			deployment := &appsv1.Deployment{}
			if err := r.Get(r.Context, client.ObjectKey{
				Namespace: backupConf.Namespace,
				Name:      target.TargetName,
			}, deployment); err != nil {
				r.Log.Error(err, "cannot get deployment", "Deployment", target.TargetName)
				return err
			}
			restoreContainers := []corev1.Container{}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == formolv1alpha1.SIDECARCONTAINER_NAME {
					continue
				}
				restoreContainers = append(restoreContainers, container)
			}
			deployment.Spec.Template.Spec.Containers = restoreContainers
			removeTags(&deployment.Spec.Template.Spec, target)
			if err := r.Update(r.Context, deployment); err != nil {
				r.Log.Error(err, "unable to update deployment", "deployment", deployment)
				return err
			}
		}
	}

	return nil
}

func (r *BackupConfigurationReconciler) AddSidecar(backupConf formolv1alpha1.BackupConfiguration) error {
	// Go through all the 'targets'
	// the backupType: Online needs a sidecar container for every single listed 'container'
	// if the backupType is something else than Online, the 'container' will still need a sidecar
	// if it has 'steps'
	addTags := func(sideCar *corev1.Container, podSpec *corev1.PodSpec, target formolv1alpha1.Target) bool {
		for i, container := range podSpec.Containers {
			if container.Name == formolv1alpha1.SIDECARCONTAINER_NAME {
				return false
			}
			for _, targetContainer := range target.Containers {
				if targetContainer.Name == container.Name {
					// Found a target container. Tag it.
					podSpec.Containers[i].Env = append(container.Env, corev1.EnvVar{
						Name:  formolv1alpha1.TARGETCONTAINER_TAG,
						Value: container.Name,
					})
					// targetContainer.Paths are the paths to backup
					// We have to find what volumes are mounted under those paths
					// and mount them under a path that exists in the sidecar container
					for i, path := range targetContainer.Paths {
						vm := corev1.VolumeMount{ReadOnly: true}
						for _, volumeMount := range container.VolumeMounts {
							var longest int = 0
							if strings.HasPrefix(path, volumeMount.MountPath) && len(volumeMount.MountPath) > longest {
								longest = len(volumeMount.MountPath)
								vm.Name = volumeMount.Name
								vm.MountPath = fmt.Sprintf("/backup%d", i)
								vm.SubPath = volumeMount.SubPath
							}
						}
						sideCar.VolumeMounts = append(sideCar.VolumeMounts, vm)
					}
				}
			}
		}
		return true
	}

	for _, target := range backupConf.Spec.Targets {
		addSidecar := false
		for _, targetContainer := range target.Containers {
			if len(targetContainer.Steps) > 0 {
				addSidecar = true
			}
		}
		if target.BackupType == formolv1alpha1.OnlineKind {
			addSidecar = true
		}
		if addSidecar {
			repo := formolv1alpha1.Repo{}
			if err := r.Get(r.Context, client.ObjectKey{
				Namespace: backupConf.Namespace,
				Name:      backupConf.Spec.Repository,
			}, &repo); err != nil {
				r.Log.Error(err, "unable to get Repo")
				return err
			}
			r.Log.V(1).Info("Got Repository", "repo", repo)
			env := repo.GetResticEnv(backupConf)
			sideCar := corev1.Container{
				Name:  formolv1alpha1.SIDECARCONTAINER_NAME,
				Image: backupConf.Spec.Image,
				Args:  []string{"backupsession", "server"},
				Env: append(env, corev1.EnvVar{
					Name:  formolv1alpha1.TARGET_NAME,
					Value: target.TargetName,
				}),
				VolumeMounts: []corev1.VolumeMount{},
			}
			switch target.TargetKind {
			case formolv1alpha1.Deployment:
				deployment := &appsv1.Deployment{}
				if err := r.Get(r.Context, client.ObjectKey{
					Namespace: backupConf.Namespace,
					Name:      target.TargetName,
				}, deployment); err != nil {
					r.Log.Error(err, "cannot get deployment", "Deployment", target.TargetName)
					return err
				}
				if addTags(&sideCar, &deployment.Spec.Template.Spec, target) {
					deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, sideCar)
					r.Log.V(1).Info("Updating deployment", "deployment", deployment, "containers", deployment.Spec.Template.Spec.Containers)
					if err := r.Update(r.Context, deployment); err != nil {
						r.Log.Error(err, "cannot update deployment", "Deployment", deployment)
						return err
					}
				}
			}
		}
	}

	return nil
}
