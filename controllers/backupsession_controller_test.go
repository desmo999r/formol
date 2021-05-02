package controllers

import (
	"context"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//corev1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Testing BackupSession controller", func() {
	const (
		BSBackupSessionName = "test-backupsession-controller"
	)
	var (
		ctx = context.Background()
		key = types.NamespacedName{
			Name:      BSBackupSessionName,
			Namespace: TestNamespace,
		}
		backupSession = &formolv1alpha1.BackupSession{}
	)
	BeforeEach(func() {
		backupSession = &formolv1alpha1.BackupSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      BSBackupSessionName,
				Namespace: TestNamespace,
			},
			Spec: formolv1alpha1.BackupSessionSpec{
				Ref: corev1.ObjectReference{
					Name: TestBackupConfName,
				},
			},
		}
	})
	Context("Creating a backupsession", func() {
		JustBeforeEach(func() {
			Eventually(func() error {
				return k8sClient.Create(ctx, backupSession)
			}, timeout, interval).Should(Succeed())
			realBackupSession := &formolv1alpha1.BackupSession{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, key, realBackupSession)
				return err
			}, timeout, interval).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				if err := k8sClient.Get(ctx, key, realBackupSession); err != nil {
					return ""
				} else {
					return realBackupSession.Status.SessionState
				}
			}, timeout, interval).Should(Equal(formolv1alpha1.Running))
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, backupSession)).Should(Succeed())
		})

		It("Should have a new task", func() {
			realBackupSession := &formolv1alpha1.BackupSession{}
			_ = k8sClient.Get(ctx, key, realBackupSession)
			Expect(realBackupSession.Status.Targets[0].Name).Should(Equal(TestDeploymentName))
			Expect(realBackupSession.Status.Targets[0].SessionState).Should(Equal(formolv1alpha1.New))
			Expect(realBackupSession.Status.Targets[0].Kind).Should(Equal(formolv1alpha1.SidecarKind))
			Expect(realBackupSession.Status.Targets[0].Try).Should(Equal(1))
		})

		It("Should move to the next task when the first one is a success", func() {
			realBackupSession := &formolv1alpha1.BackupSession{}
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			realBackupSession.Status.Targets[0].SessionState = formolv1alpha1.Success
			Expect(k8sClient.Status().Update(ctx, realBackupSession)).Should(Succeed())
			Eventually(func() int {
				_ = k8sClient.Get(ctx, key, realBackupSession)
				return len(realBackupSession.Status.Targets)
			}, timeout, interval).Should(Equal(2))
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			Expect(realBackupSession.Status.Targets[1].Name).Should(Equal(TestBackupFuncName))
			Expect(realBackupSession.Status.Targets[1].SessionState).Should(Equal(formolv1alpha1.New))
			Expect(realBackupSession.Status.Targets[1].Kind).Should(Equal(formolv1alpha1.JobKind))
		})

		It("Should be a success when the last task is a success", func() {
			realBackupSession := &formolv1alpha1.BackupSession{}
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			realBackupSession.Status.Targets[0].SessionState = formolv1alpha1.Success
			Expect(k8sClient.Status().Update(ctx, realBackupSession)).Should(Succeed())
			Eventually(func() int {
				_ = k8sClient.Get(ctx, key, realBackupSession)
				return len(realBackupSession.Status.Targets)
			}, timeout, interval).Should(Equal(2))
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			realBackupSession.Status.Targets[1].SessionState = formolv1alpha1.Success
			Expect(k8sClient.Status().Update(ctx, realBackupSession)).Should(Succeed())
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				_ = k8sClient.Get(ctx, key, realBackupSession)
				return realBackupSession.Status.SessionState
			}, timeout, interval).Should(Equal(formolv1alpha1.Success))
		})

		It("Should retry when the task is a failure", func() {
			realBackupSession := &formolv1alpha1.BackupSession{}
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			realBackupSession.Status.Targets[0].SessionState = formolv1alpha1.Success
			Expect(k8sClient.Status().Update(ctx, realBackupSession)).Should(Succeed())
			Eventually(func() int {
				_ = k8sClient.Get(ctx, key, realBackupSession)
				return len(realBackupSession.Status.Targets)
			}, timeout, interval).Should(Equal(2))
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			realBackupSession.Status.Targets[1].SessionState = formolv1alpha1.Failure
			Expect(k8sClient.Status().Update(ctx, realBackupSession)).Should(Succeed())
			Eventually(func() int {
				_ = k8sClient.Get(ctx, key, realBackupSession)
				return realBackupSession.Status.Targets[1].Try
			}, timeout, interval).Should(Equal(2))
			Expect(k8sClient.Get(ctx, key, realBackupSession)).Should(Succeed())
			Expect(realBackupSession.Status.Targets[1].SessionState).Should(Equal(formolv1alpha1.New))
			realBackupSession.Status.Targets[1].SessionState = formolv1alpha1.Failure
			Expect(k8sClient.Status().Update(ctx, realBackupSession)).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				_ = k8sClient.Get(ctx, key, realBackupSession)
				return realBackupSession.Status.SessionState
			}, timeout, interval).Should(Equal(formolv1alpha1.Failure))
		})

		It("should create a backup job", func() {
		})
	})
	Context("When other BackupSession exist", func() {
		const (
			bs1Name = "test-backupsession-controller1"
			bs2Name = "test-backupsession-controller2"
			bs3Name = "test-backupsession-controller3"
		)
		var ()
		BeforeEach(func() {
		})
		JustBeforeEach(func() {
		})
		It("Should clean up old sessions", func() {
		})
	})
})
