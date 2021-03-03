package controllers

import (
	"context"
	//"k8s.io/apimachinery/pkg/types"
	//"reflect"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//batchv1 "k8s.io/api/batch/v1"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Setup the environment", func() {
	const (
		BackupConfName      = "test-backupconf"
		BackupConfNamespace = "test-backupconf-namespace"
		RepoName            = "test-repo"
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
	)
	var (
		ctx       = context.Background()
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: BackupConfNamespace,
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
						Name: "test-deployment",
					},
				},
			},
		}
	)

	BeforeEach(func() {
	})
	AfterEach(func() {
	})

	Context("Creating a backupconf", func() {
		It("Should also create a CronJob", func() {
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
			Expect(k8sClient.Create(ctx, sa)).Should(Succeed())
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			Expect(k8sClient.Create(ctx, repo)).Should(Succeed())
			Expect(k8sClient.Create(ctx, backupConf)).Should(Succeed())
			realBackupConf := &formolv1alpha1.BackupConfiguration{}
			key := types.NamespacedName{
				Name:      BackupConfName,
				Namespace: BackupConfNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, realBackupConf)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(realBackupConf.Spec.Schedule).Should(Equal("1 * * * *"))
			//Expect(realBackupConf.Spec.Suspend).Should(BeFalse())
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
	})
	Context("Updating a backupconf", func() {
		It("Should also update the CronJob", func() {
			realBackupConf := &formolv1alpha1.BackupConfiguration{}
			key := types.NamespacedName{
				Name:      BackupConfName,
				Namespace: BackupConfNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, realBackupConf)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			realBackupConf.Spec.Schedule = "1 0 * * *"
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
		})
	})

})
