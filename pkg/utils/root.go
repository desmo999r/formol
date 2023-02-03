package utils

import (
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func ConfigureResticEnvVar(backupConf *formolv1alpha1.BackupConfiguration, repo *formolv1alpha1.Repo) []corev1.EnvVar {
	env := []corev1.EnvVar{}
	// S3 backing storage
	return env
}
