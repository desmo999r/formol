package v1alpha1

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"strings"
)

const (
	RESTORECONTAINER_NAME string = "formol-restore"
	// the name of the sidecar container
	SIDECARCONTAINER_NAME string = "formol"
	// the name of the container we backup when there are more than 1 container in the pod
	TARGETCONTAINER_TAG string = "FORMOL_TARGET"
	// Used by both the backupsession and restoresession controllers to identified the target deployment
	TARGET_NAME string = "TARGET_NAME"
	// Used by the backupsession controller
	POD_NAME      string = "POD_NAME"
	POD_NAMESPACE string = "POD_NAMESPACE"
	// Backup Paths list
	BACKUP_PATHS = "BACKUP_PATHS"
)

func GetSharedPath(podSpec *corev1.PodSpec, index int, targetContainer TargetContainer) (vms []corev1.VolumeMount) {
	// Create a shared mount between the target and sidecar container
	// the output of the Job will be saved in the shared volume
	// and restic will then backup the content of the volume
	var addSharedVol bool = true
	for _, vol := range podSpec.Volumes {
		if vol.Name == FORMOL_SHARED_VOLUME {
			addSharedVol = false
		}
	}
	if addSharedVol {
		podSpec.Volumes = append(podSpec.Volumes,
			corev1.Volume{
				Name:         FORMOL_SHARED_VOLUME,
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})
	}
	podSpec.Containers[index].VolumeMounts = append(podSpec.Containers[index].VolumeMounts, corev1.VolumeMount{
		Name:      FORMOL_SHARED_VOLUME,
		MountPath: targetContainer.SharePath,
	})
	vms = append(vms, corev1.VolumeMount{
		Name:      FORMOL_SHARED_VOLUME,
		MountPath: targetContainer.SharePath,
	})
	return
}

func GetVolumeMounts(container corev1.Container, targetContainer TargetContainer) (sidecarPaths []string, vms []corev1.VolumeMount) {
	// targetContainer.Paths are the paths to backup
	// We have to find what volumes are mounted under those paths
	// and mount them under a path that exists in the sidecar container
	for i, path := range targetContainer.Paths {
		vm := corev1.VolumeMount{ReadOnly: true}
		var longest int = 0
		var sidecarPath string
		path = filepath.Clean(path)
		splitPath := strings.Split(path, string(os.PathSeparator))
		for _, volumeMount := range container.VolumeMounts {
			splitMountPath := strings.Split(volumeMount.MountPath, string(os.PathSeparator))
			for j, pathItem := range splitMountPath {
				if j < len(splitPath) && pathItem == splitPath[j] && j > longest {
					longest = j
					vm.Name = volumeMount.Name
					vm.MountPath = fmt.Sprintf("/%s%d", BACKUP_PREFIX_PATH, i)
					vm.SubPath = volumeMount.SubPath
					rel, _ := filepath.Rel(volumeMount.MountPath, path)
					sidecarPath = filepath.Join(vm.MountPath, rel)
				}
			}
		}
		vms = append(vms, vm)
		sidecarPaths = append(sidecarPaths, sidecarPath)
	}
	return
}

func GetSidecar(backupConf BackupConfiguration, target Target) corev1.Container {
	sidecar := corev1.Container{
		Name:  SIDECARCONTAINER_NAME,
		Image: backupConf.Spec.Image,
		Args:  []string{"server"},
		Env: []corev1.EnvVar{
			corev1.EnvVar{
				Name:  TARGET_NAME,
				Value: target.TargetName,
			},
			corev1.EnvVar{
				Name: POD_NAMESPACE,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			}},
		VolumeMounts: []corev1.VolumeMount{},
		SecurityContext: &corev1.SecurityContext{
			Privileged: func() *bool { b := true; return &b }(),
		},
	}
	return sidecar
}
