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

var _ = Describe("Setup the environment", func() {
	const (
		BackupConfName      = "test-backupconf"
		BackupConfNamespace = "test-backupconf-namespace"
		RepoName            = "test-repo"
		DeploymentName      = "test-deployment"
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
	)
	var (
		key = types.NamespacedName{
			Name:      BackupConfName,
			Namespace: BackupConfNamespace,
		}
		ctx       = context.Background()
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: BackupConfNamespace,
			},
		}
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: BackupConfNamespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test-deployment"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test-deployment"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							corev1.Container{
								Name:  "test-container",
								Image: "test-image",
							},
						},
					},
				},
			},
		}
		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: BackupConfNamespace,
			},
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: BackupConfNamespace,
			},
			Data: map[string][]byte{
				"RESTIC_PASSWORD":       []byte("toto"),
				"AWS_ACCESS_KEY_ID":     []byte("titi"),
				"AWS_SECRET_ACCESS_KEY": []byte("tata"),
			},
		}
		repo = &formolv1alpha1.Repo{
			ObjectMeta: metav1.ObjectMeta{
				Name:      RepoName,
				Namespace: BackupConfNamespace,
			},
			Spec: formolv1alpha1.RepoSpec{
				Backend: formolv1alpha1.Backend{
					S3: formolv1alpha1.S3{
						Server: "raid5.desmojim.fr:9000",
						Bucket: "testbucket2",
					},
				},
				RepositorySecrets: "test-secret",
			},
		}
		backupConf = &formolv1alpha1.BackupConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      BackupConfName,
				Namespace: BackupConfNamespace,
			},
			Spec: formolv1alpha1.BackupConfigurationSpec{
				Repository: RepoName,
				Schedule:   "1 * * * *",
				Targets: []formolv1alpha1.Target{
					formolv1alpha1.Target{
						Kind: "Deployment",
						Name: DeploymentName,
					},
				},
			},
		}
	)

	BeforeEach(func() {
	})
	AfterEach(func() {
	})

	Context("Testing backupconf", func() {
		It("Requires initialisation", func() {
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
			Expect(k8sClient.Create(ctx, sa)).Should(Succeed())
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			Expect(k8sClient.Create(ctx, repo)).Should(Succeed())
			Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())
		})
		Context("Creating a backupconf", func() {
			It("Requires initialisation", func() {
				Expect(k8sClient.Create(ctx, backupConf)).Should(Succeed())
				realBackupConf := &formolv1alpha1.BackupConfiguration{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, key, realBackupConf)
					if err != nil {
						return false
					}
					return true
				}, timeout, interval).Should(BeTrue())
				Expect(realBackupConf.Spec.Schedule).Should(Equal("1 * * * *"))
			})
			It("Should also create a CronJob", func() {
				cronJob := &batchv1beta1.CronJob{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      "backup-" + BackupConfName,
						Namespace: BackupConfNamespace,
					}, cronJob)
					if err != nil {
						return false
					}
					return true
				}, timeout, interval).Should(BeTrue())
				Expect(cronJob.Spec.Schedule).Should(Equal("1 * * * *"))
			})
			It("Should also create a sidecar container", func() {
				realDeployment := &appsv1.Deployment{}
				Eventually(func() (int, error) {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      DeploymentName,
						Namespace: BackupConfNamespace,
					}, realDeployment)
					if err != nil {
						return 0, err
					}
					return len(realDeployment.Spec.Template.Spec.Containers), nil
				}, timeout, interval).Should(Equal(2))
			})
		})
		Context("Updating a backupconf", func() {
			It("Should also update the CronJob", func() {
				realBackupConf := &formolv1alpha1.BackupConfiguration{}
				Expect(k8sClient.Get(ctx, key, realBackupConf)).Should(Succeed())
				realBackupConf.Spec.Schedule = "1 0 * * *"
				suspend := true
				realBackupConf.Spec.Suspend = &suspend
				Expect(k8sClient.Update(ctx, realBackupConf)).Should(Succeed())
				cronJob := &batchv1beta1.CronJob{}
				Eventually(func() (string, error) {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      "backup-" + BackupConfName,
						Namespace: BackupConfNamespace,
					}, cronJob)
					if err != nil {
						return "", err
					}
					return cronJob.Spec.Schedule, nil
				}, timeout, interval).Should(Equal("1 0 * * *"))
				Eventually(func() (bool, error) {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      "backup-" + BackupConfName,
						Namespace: BackupConfNamespace,
					}, cronJob)
					if err != nil {
						return false, err
					}
					return *cronJob.Spec.Suspend == true, nil
				}, timeout, interval).Should(BeTrue())
			})
		})
		Context("Deleting a backupconf", func() {
			It("Should also delete the sidecar container", func() {
				Expect(k8sClient.Delete(ctx, backupConf)).Should(Succeed())
				realDeployment := &appsv1.Deployment{}
				Eventually(func() (int, error) {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      DeploymentName,
						Namespace: BackupConfNamespace,
					}, realDeployment)
					if err != nil {
						return 0, err
					}
					return len(realDeployment.Spec.Template.Spec.Containers), nil
				}, timeout, interval).Should(Equal(1))

			})
		})
	})

})
