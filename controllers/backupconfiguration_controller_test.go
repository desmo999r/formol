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
	"context"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	//"time"
	//appsv1 "k8s.io/api/apps/v1"
	//corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("BackupConfiguration controller", func() {
	const BACKUPCONF_NAME = "test-backupconf-controller"

	var (
		backupConf *formolv1alpha1.BackupConfiguration
		ctx        = context.Background()
		key        = types.NamespacedName{
			Name:      BACKUPCONF_NAME,
			Namespace: NAMESPACE_NAME,
		}
	)

	BeforeEach(func() {
		backupConf = &formolv1alpha1.BackupConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      BACKUPCONF_NAME,
				Namespace: NAMESPACE_NAME,
			},
			Spec: formolv1alpha1.BackupConfigurationSpec{
				Repository: REPO_NAME,
				Schedule:   "1 * * * *",
				Image:      "desmo999r/formolcli:v0.3.2",
				Targets: []formolv1alpha1.Target{
					formolv1alpha1.Target{
						BackupType: formolv1alpha1.OnlineKind,
						TargetKind: formolv1alpha1.Deployment,
						TargetName: DEPLOYMENT_NAME,
						Containers: []formolv1alpha1.TargetContainer{
							formolv1alpha1.TargetContainer{
								Name: CONTAINER_NAME,
							},
						},
					},
				},
			},
		}
	})

	Context("Creating a BackupConf", func() {
		JustBeforeEach(func() {
			Eventually(func() error {
				return k8sClient.Create(ctx, backupConf)
			}, timeout, interval).Should(Succeed())
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, backupConf)).Should(Succeed())
		})
		It("Has a schedule", func() {
			realBackupConf := &formolv1alpha1.BackupConfiguration{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, key, realBackupConf); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(realBackupConf.Spec.Schedule).Should(Equal("1 * * * *"))
		})
		It("Should create a CronJob", func() {
			realBackupConf := &formolv1alpha1.BackupConfiguration{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, key, realBackupConf); err != nil {
					return false
				}
				return realBackupConf.Status.ActiveCronJob
			}, timeout, interval).Should(BeTrue())
			cronJob := &batchv1.CronJob{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BACKUPCONF_NAME,
					Namespace: NAMESPACE_NAME,
				}, cronJob); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(cronJob.Spec.Schedule).Should(Equal("1 * * * *"))
		})
		It("Should update the CronJob", func() {
			realBackupConf := &formolv1alpha1.BackupConfiguration{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, key, realBackupConf); err != nil {
					return false
				}
				return realBackupConf.Status.ActiveCronJob
			}, timeout, interval).Should(BeTrue())
			realBackupConf.Spec.Schedule = "1 0 * * *"
			suspend := true
			realBackupConf.Spec.Suspend = &suspend
			Expect(k8sClient.Update(ctx, realBackupConf)).Should(Succeed())
			cronJob := &batchv1.CronJob{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BACKUPCONF_NAME,
					Namespace: NAMESPACE_NAME,
				}, cronJob); err != nil {
					return ""
				}
				return cronJob.Spec.Schedule
			}, timeout, interval).Should(Equal("1 0 * * *"))
			Expect(*cronJob.Spec.Suspend).Should(BeTrue())
		})
	})
	Context("Deleting a BackupConf", func() {
		JustBeforeEach(func() {
			Eventually(func() error {
				return k8sClient.Create(ctx, backupConf)
			}, timeout, interval).Should(Succeed())
		})
		It("Should delete the CronJob", func() {
			cronJob := &batchv1.CronJob{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BACKUPCONF_NAME,
					Namespace: NAMESPACE_NAME,
				}, cronJob); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			By("The CronJob has been created. Now deleting the BackupConfiguration")
			Expect(k8sClient.Delete(ctx, backupConf)).Should(Succeed())
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BACKUPCONF_NAME,
					Namespace: NAMESPACE_NAME,
				}, cronJob); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeFalse())

		})
	})
})
