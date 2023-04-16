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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

const (
	FORMOL_SA                  = "formol-controller"
	FORMOL_SIDECAR_ROLE        = "formol:sidecar-role"
	FORMOL_SIDECAR_CLUSTERROLE = "formol:sidecar-clusterrole"
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
	repo := formolv1alpha1.Repo{}
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      backupConf.Spec.Repository,
	}, &repo); err != nil {
		r.Log.Error(err, "unable to get Repo", "repo", backupConf.Spec.Repository)
		return err
	}
	r.Log.V(1).Info("Got Repository", "repo", repo)
	for _, target := range backupConf.Spec.Targets {
		targetObject, targetPodSpec := formolv1alpha1.GetTargetObjects(target.TargetKind)
		if err := r.Get(r.Context, client.ObjectKey{
			Namespace: backupConf.Namespace,
			Name:      target.TargetName,
		}, targetObject); err != nil {
			r.Log.Error(err, "cannot get target", "target", target.TargetName)
			return err
		}
		restoreContainers := []corev1.Container{}
		for _, container := range targetPodSpec.Containers {
			if container.Name == formolv1alpha1.SIDECARCONTAINER_NAME {
				continue
			}
			restoreVms := []corev1.VolumeMount{}
			for _, vm := range container.VolumeMounts {
				if vm.Name == formolv1alpha1.FORMOL_SHARED_VOLUME {
					r.Log.V(0).Info("cleanup VolumeMounts", "container", container.Name, "VolumeMount", vm.Name)
					continue
				}
				restoreVms = append(restoreVms, vm)
			}
			r.Log.V(0).Info("cleanup VolumeMounts", "container", container.Name, "restoreVms", restoreVms)
			container.VolumeMounts = restoreVms
			restoreContainers = append(restoreContainers, container)
		}
		targetPodSpec.Containers = restoreContainers
		restoreVolumes := []corev1.Volume{}
		for _, volume := range targetPodSpec.Volumes {
			if volume.Name == formolv1alpha1.RESTIC_REPO_VOLUME {
				continue
			}
			if volume.Name == formolv1alpha1.FORMOL_SHARED_VOLUME {
				continue
			}
			restoreVolumes = append(restoreVolumes, volume)
		}
		targetPodSpec.Volumes = restoreVolumes
		removeTags(targetPodSpec, target)
		if err := r.Update(r.Context, targetObject); err != nil {
			r.Log.Error(err, "unable to remove sidecar", "targetObject", targetObject)
			return err
		}
	}

	return nil
}

func hasSidecar(podSpec *corev1.PodSpec) bool {
	for _, container := range podSpec.Containers {
		if container.Name == formolv1alpha1.SIDECARCONTAINER_NAME {
			return true
		}
	}
	return false
}

func (r *BackupConfigurationReconciler) addSidecar(backupConf formolv1alpha1.BackupConfiguration, target formolv1alpha1.Target) (err error) {
	repo := formolv1alpha1.Repo{}
	if err = r.Get(r.Context, client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      backupConf.Spec.Repository,
	}, &repo); err != nil {
		r.Log.Error(err, "unable to get Repo")
		return err
	}
	r.Log.V(1).Info("Got Repository", "repo", repo)
	sidecar := formolv1alpha1.GetSidecar(backupConf, target)
	targetObject, targetPodSpec := formolv1alpha1.GetTargetObjects(target.TargetKind)
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: backupConf.Namespace,
		Name:      target.TargetName,
	}, targetObject); err != nil {
		r.Log.Error(err, "cannot get target", "target", target.TargetName)
		return err
	}
	if !hasSidecar(targetPodSpec) {
		if err = r.createRBACSidecar(corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: backupConf.Namespace,
				Name:      targetPodSpec.ServiceAccountName,
			},
		}); err != nil {
			r.Log.Error(err, "unable to create RBAC for the sidecar container")
			return
		}
		for i, container := range targetPodSpec.Containers {
			for _, targetContainer := range target.Containers {
				if targetContainer.Name == container.Name {
					// Found a target container. Tag it.
					targetPodSpec.Containers[i].Env = append(container.Env, corev1.EnvVar{
						Name:  formolv1alpha1.TARGETCONTAINER_TAG,
						Value: container.Name,
					})
					switch target.BackupType {
					case formolv1alpha1.OnlineKind:
						sidecarPaths, vms := formolv1alpha1.GetVolumeMounts(container, targetContainer)
						sidecar.Env = append(sidecar.Env, corev1.EnvVar{
							Name:  formolv1alpha1.BACKUP_PATHS,
							Value: strings.Join(sidecarPaths, string(os.PathListSeparator)),
						})
						sidecar.VolumeMounts = vms
					case formolv1alpha1.JobKind:
						sidecar.VolumeMounts = formolv1alpha1.GetSharedPath(targetPodSpec, i, targetContainer)
					}
				}
			}
		}
		if repo.Spec.Backend.Local != nil {
			sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{
				Name:      formolv1alpha1.RESTIC_REPO_VOLUME,
				MountPath: formolv1alpha1.RESTIC_REPO_PATH,
			})
			targetPodSpec.Volumes = append(targetPodSpec.Volumes, corev1.Volume{
				Name:         formolv1alpha1.RESTIC_REPO_VOLUME,
				VolumeSource: repo.Spec.Backend.Local.VolumeSource,
			})
		}

		// The sidecar definition is complete. Add it to the targetObject
		targetPodSpec.Containers = append(targetPodSpec.Containers, sidecar)
		targetPodSpec.ShareProcessNamespace = func() *bool { b := true; return &b }()
		r.Log.V(1).Info("Adding sidecar", "targetObject", targetObject, "sidecar", sidecar)
		if err = r.Update(r.Context, targetObject); err != nil {
			r.Log.Error(err, "unable to add sidecar", "targetObject", targetObject)
			return
		}
	}

	return
}

// Delete the sidecar role is there is no more sidecar container in the namespace
func (r *BackupConfigurationReconciler) deleteRBACSidecar(namespace string) error {
	podList := corev1.PodList{}
	if err := r.List(r.Context, &podList, &client.ListOptions{
		Namespace: namespace,
	}); err != nil {
		r.Log.Error(err, "unable to get the list of pods", "namespace", namespace)
		return err
	}
	for _, pod := range podList.Items {
		for _, container := range pod.Spec.Containers {
			for _, env := range container.Env {
				if env.Name == formolv1alpha1.SIDECARCONTAINER_NAME {
					// There is still a sidecar in the namespace.
					// cannot delete the sidecar role
					return nil
				}
			}
		}
	}
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      FORMOL_SIDECAR_ROLE,
		},
	}
	if err := r.Delete(r.Context, &roleBinding); err != nil {
		r.Log.Error(err, "unable to delete sidecar role binding")
	}
	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      FORMOL_SIDECAR_ROLE,
		},
	}
	if err := r.Delete(r.Context, &role); err != nil {
		r.Log.Error(err, "unable to delete sidecar role")
	}
	clusterRoleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      FORMOL_SIDECAR_CLUSTERROLE,
		},
	}
	if err := r.Delete(r.Context, &clusterRoleBinding); err != nil {
		r.Log.Error(err, "unable to delete sidecar clusterRole binding")
	}
	clusterRole := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      FORMOL_SIDECAR_CLUSTERROLE,
		},
	}
	if err := r.Delete(r.Context, &clusterRole); err != nil {
		r.Log.Error(err, "unable to delete sidecar clusterRole")
	}
	return nil
}

// Creates a role to allow the BackupSession controller in the sidecar to have access to resources
// like Repo, Functions, ...
func (r *BackupConfigurationReconciler) createRBACSidecar(sa corev1.ServiceAccount) error {
	if sa.Name == "" {
		sa.Name = "default"
	}
	role := rbacv1.Role{}
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: sa.Namespace,
		Name:      FORMOL_SIDECAR_ROLE,
	}, &role); err != nil && errors.IsNotFound(err) {
		role = rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sa.Namespace,
				Name:      FORMOL_SIDECAR_ROLE,
			},
			Rules: []rbacv1.PolicyRule{
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch", "update"},
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
				},
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch"},
					APIGroups: []string{"formol.desmojim.fr"},
					Resources: []string{"restoresessions", "backupsessions", "backupconfigurations", "functions", "repoes"},
				},
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					APIGroups: []string{""},
					Resources: []string{"secrets", "persistentvolumeclaims"},
				},
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					APIGroups: []string{"batch"},
					Resources: []string{"jobs"},
				},
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					APIGroups: []string{"formol.desmojim.fr"},
					Resources: []string{"backupsessions/status"},
				},
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					APIGroups: []string{"formol.desmojim.fr"},
					Resources: []string{"restoresessions/status"},
				},
			},
		}
		r.Log.V(0).Info("Creating formol sidecar role", "role", role)
		if err = r.Create(r.Context, &role); err != nil {
			r.Log.Error(err, "unable to create sidecar role")
			return err
		}
	}
	rolebinding := rbacv1.RoleBinding{}
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: sa.Namespace,
		Name:      FORMOL_SIDECAR_ROLE,
	}, &rolebinding); err != nil && errors.IsNotFound(err) {
		rolebinding = rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sa.Namespace,
				Name:      FORMOL_SIDECAR_ROLE,
			},
			Subjects: []rbacv1.Subject{
				rbacv1.Subject{
					Kind: "ServiceAccount",
					Name: sa.Name,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     FORMOL_SIDECAR_ROLE,
			},
		}
		r.Log.V(0).Info("Creating formol sidecar rolebinding", "rolebinding", rolebinding)
		if err = r.Create(r.Context, &rolebinding); err != nil {
			r.Log.Error(err, "unable to create sidecar rolebinding")
			return err
		}
	}
	clusterRole := rbacv1.ClusterRole{}
	if err := r.Get(r.Context, client.ObjectKey{
		Name: FORMOL_SIDECAR_CLUSTERROLE,
	}, &clusterRole); err != nil && errors.IsNotFound(err) {
		clusterRole = rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: FORMOL_SIDECAR_CLUSTERROLE,
			},
			Rules: []rbacv1.PolicyRule{
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch"},
					APIGroups: []string{"", "snapshot.storage.k8s.io"},
					Resources: []string{"volumesnapshotclasses", "persistentvolumes"},
				},
				rbacv1.PolicyRule{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					APIGroups: []string{"snapshot.storage.k8s.io"},
					Resources: []string{"volumesnapshots"},
				},
			},
		}
		r.Log.V(0).Info("Creating formol sidecar cluster role", "clusterRole", clusterRole)
		if err = r.Create(r.Context, &clusterRole); err != nil {
			r.Log.Error(err, "unable to create sidecar cluster role")
			return err
		}
	}
	clusterRolebinding := rbacv1.ClusterRoleBinding{}
	if err := r.Get(r.Context, client.ObjectKey{
		Namespace: sa.Namespace,
		Name:      FORMOL_SIDECAR_CLUSTERROLE,
	}, &clusterRolebinding); err != nil && errors.IsNotFound(err) {
		clusterRolebinding = rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: FORMOL_SIDECAR_CLUSTERROLE,
			},
			Subjects: []rbacv1.Subject{
				rbacv1.Subject{
					Kind:      "ServiceAccount",
					Name:      sa.Name,
					Namespace: sa.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     FORMOL_SIDECAR_CLUSTERROLE,
			},
		}
		r.Log.V(0).Info("Creating formol sidecar clusterrolebinding", "clusterrolebinding", clusterRolebinding)
		if err = r.Create(r.Context, &clusterRolebinding); err != nil {
			r.Log.Error(err, "unable to create sidecar cluster rolebinding")
			return err
		}
	}
	return nil
}
