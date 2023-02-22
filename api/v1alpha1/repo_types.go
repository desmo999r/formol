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

package v1alpha1

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	RESTIC_REPO_VOLUME    = "restic-volume"
	RESTIC_REPO_PATH      = "/restic-repo"
	RESTIC_REPOSITORY     = "RESTIC_REPOSITORY"
	RESTIC_PASSWORD       = "RESTIC_PASSWORD"
	AWS_ACCESS_KEY_ID     = "AWS_ACCESS_KEY_ID"
	AWS_SECRET_ACCESS_KEY = "AWS_SECRET_ACCESS_KEY"
)

type S3 struct {
	Server string `json:"server"`
	Bucket string `json:"bucket"`
	// +optional
	Prefix string `json:"prefix,omitempty"`
}

type Local struct {
	corev1.VolumeSource `json:"source"`
}

type Backend struct {
	// +optional
	S3 *S3 `json:"s3,omitempty"`
	// +optional
	Local *Local `json:"local,omitempty"`
}

// RepoSpec defines the desired state of Repo
type RepoSpec struct {
	Backend           `json:"backend"`
	RepositorySecrets string `json:"repositorySecrets"`
}

// RepoStatus defines the observed state of Repo
type RepoStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Repo is the Schema for the repoes API
type Repo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepoSpec   `json:"spec,omitempty"`
	Status RepoStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RepoList contains a list of Repo
type RepoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Repo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Repo{}, &RepoList{})
}

func (repo *Repo) GetResticEnv(backupConf BackupConfiguration) []corev1.EnvVar {
	env := []corev1.EnvVar{}
	if repo.Spec.Backend.S3 != nil {
		url := fmt.Sprintf("s3:http://%s/%s/%s-%s",
			repo.Spec.Backend.S3.Server,
			repo.Spec.Backend.S3.Bucket,
			strings.ToUpper(backupConf.Namespace),
			strings.ToLower(backupConf.Name))
		env = append(env, corev1.EnvVar{
			Name:  RESTIC_REPOSITORY,
			Value: url,
		})
		for _, key := range []string{
			AWS_ACCESS_KEY_ID,
			AWS_SECRET_ACCESS_KEY,
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
	if repo.Spec.Backend.Local != nil {
		env = append(env, corev1.EnvVar{
			Name:  RESTIC_REPOSITORY,
			Value: RESTIC_REPO_PATH,
		})
	}
	env = append(env, corev1.EnvVar{
		Name: RESTIC_PASSWORD,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: repo.Spec.RepositorySecrets,
				},
				Key: RESTIC_PASSWORD,
			},
		},
	})

	return env
}
