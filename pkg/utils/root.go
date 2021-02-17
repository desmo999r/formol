package utils

import (
	"fmt"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"strings"
)

const (
	FORMOLCLI string = "desmo999r/formolcli:latest"
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
	if (formolv1alpha1.S3{}) != repo.Spec.Backend.S3 {
		url := fmt.Sprintf("s3:http://%s/%s/%s-%s", repo.Spec.Backend.S3.Server, repo.Spec.Backend.S3.Bucket, strings.ToUpper(backupConf.Namespace), strings.ToLower(backupConf.Name))
		env = append(env, corev1.EnvVar{
			Name:  "RESTIC_REPOSITORY",
			Value: url,
		})
		for _, key := range []string{
			"AWS_ACCESS_KEY_ID",
			"AWS_SECRET_ACCESS_KEY",
			"RESTIC_PASSWORD",
		} {
			env = append(env, corev1.EnvVar{
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
	return env
}
