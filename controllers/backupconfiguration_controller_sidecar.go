package controllers

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
)

func (r *BackupConfigurationReconciler) DeleteSidecar(backupConf formolv1alpha1.BackupConfiguration) error {
	removeTags := func(podSpec *corev1.PodSpec, target formolv1alpha1.Target) {
		for i, container := range podSpec.Containers {
			for _, targetContainer := range target.Containers {
				if targetContainer.Name == container.Name {
					if container.Env[len(container.Env)-1].Name == formolv1alpha1.TARGETCONTAINER_TAG {
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
	addTags := func(podSpec *corev1.PodSpec, target formolv1alpha1.Target) bool {
		for i, container := range podSpec.Containers {
			if container.Name == formolv1alpha1.SIDECARCONTAINER_NAME {
				return false
			}
			for _, targetContainer := range target.Containers {
				if targetContainer.Name == container.Name {
					podSpec.Containers[i].Env = append(container.Env, corev1.EnvVar{
						Name:  formolv1alpha1.TARGETCONTAINER_TAG,
						Value: container.Name,
					})
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
				if addTags(&deployment.Spec.Template.Spec, target) {
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
