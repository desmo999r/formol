package controllers

import (
	"context"
	//"k8s.io/apimachinery/pkg/types"
	//"reflect"
	//"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//batchv1 "k8s.io/api/batch/v1"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Testing BackupConf controller", func() {
	const (
		BCBackupConfName = "test-backupconf-controller"
	)
	var (
		key = types.NamespacedName{
			Name:      BCBackupConfName,
			Namespace: TestNamespace,
		}
		ctx        = context.Background()
		backupConf = &formolv1alpha1.BackupConfiguration{}
	)

	BeforeEach(func() {
		backupConf = &formolv1alpha1.BackupConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      BCBackupConfName,
				Namespace: TestNamespace,
			},
			Spec: formolv1alpha1.BackupConfigurationSpec{
				Repository: TestRepoName,
				Schedule:   "1 * * * *",
				Image:      "desmo999r/formolcli:latest",
				Targets: []formolv1alpha1.Target{
					formolv1alpha1.Target{
						Kind: formolv1alpha1.SidecarKind,
						Name: TestDeploymentName,
						VolumeMounts: []corev1.VolumeMount{
							corev1.VolumeMount{
								Name:      TestDataVolume,
								MountPath: TestDataMountPath,
							},
						},
						Paths: []string{
							TestDataMountPath,
						},
					},
					formolv1alpha1.Target{
						Kind: formolv1alpha1.JobKind,
						Name: TestBackupFuncName,
						Steps: []formolv1alpha1.Step{
							formolv1alpha1.Step{
								Name: TestBackupFuncName,
								Env: []corev1.EnvVar{
									corev1.EnvVar{
										Name:  "foo",
										Value: "bar",
									},
								},
							},
						},
					},
				},
			},
		}
	})
	Context("Creating a backupconf", func() {
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
				err := k8sClient.Get(ctx, key, realBackupConf)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(realBackupConf.Spec.Schedule).Should(Equal("1 * * * *"))
			Expect(realBackupConf.Spec.Targets[0].Retry).Should(Equal(2))
		})
		It("Should also create a CronJob", func() {
			cronJob := &batchv1beta1.CronJob{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BCBackupConfName,
					Namespace: TestNamespace,
				}, cronJob)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(cronJob.Spec.Schedule).Should(Equal("1 * * * *"))
		})
		It("Should also create a sidecar container", func() {
			realDeployment := &appsv1.Deployment{}
			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      TestDeploymentName,
					Namespace: TestNamespace,
				}, realDeployment)
				if err != nil {
					return 0, err
				}
				return len(realDeployment.Spec.Template.Spec.Containers), nil
			}, timeout, interval).Should(Equal(2))
		})
		It("Should also update the CronJob", func() {
			realBackupConf := &formolv1alpha1.BackupConfiguration{}
			time.Sleep(300 * time.Millisecond)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, realBackupConf)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			realBackupConf.Spec.Schedule = "1 0 * * *"
			suspend := true
			realBackupConf.Spec.Suspend = &suspend
			Expect(k8sClient.Update(ctx, realBackupConf)).Should(Succeed())
			cronJob := &batchv1beta1.CronJob{}
			Eventually(func() (string, error) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BCBackupConfName,
					Namespace: TestNamespace,
				}, cronJob)
				if err != nil {
					return "", err
				}
				return cronJob.Spec.Schedule, nil
			}, timeout, interval).Should(Equal("1 0 * * *"))
			Eventually(func() (bool, error) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "backup-" + BCBackupConfName,
					Namespace: TestNamespace,
				}, cronJob)
				if err != nil {
					return false, err
				}
				return *cronJob.Spec.Suspend == true, nil
			}, timeout, interval).Should(BeTrue())
		})
	})
	Context("Deleting a backupconf", func() {
		JustBeforeEach(func() {
			Eventually(func() error {
				return k8sClient.Create(ctx, backupConf)
			}, timeout, interval).Should(Succeed())
		})
		It("Should also delete the sidecar container", func() {
			Expect(k8sClient.Delete(ctx, backupConf)).Should(Succeed())
			realDeployment := &appsv1.Deployment{}
			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      TestDeploymentName,
					Namespace: TestNamespace,
				}, realDeployment)
				if err != nil {
					return 0, err
				}
				return len(realDeployment.Spec.Template.Spec.Containers), nil
			}, timeout, interval).Should(Equal(1))

		})
	})

})
