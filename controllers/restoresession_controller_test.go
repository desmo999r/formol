package controllers

import (
	"context"
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Testing RestoreSession controller", func() {
	const (
		RSRestoreSessionName = "test-restoresession-controller"
	)
	var (
		ctx = context.Background()
		key = types.NamespacedName{
			Name:      RSRestoreSessionName,
			Namespace: TestNamespace,
		}
		restoreSession = &formolv1alpha1.RestoreSession{}
	)
	BeforeEach(func() {
		restoreSession = &formolv1alpha1.RestoreSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      RSRestoreSessionName,
				Namespace: TestNamespace,
			},
			Spec: formolv1alpha1.RestoreSessionSpec{
				Ref: TestBackupSessionName,
			},
		}
	})
	Context("Creating a RestoreSession", func() {
		JustBeforeEach(func() {
			Eventually(func() error {
				return k8sClient.Create(ctx, restoreSession)
			}, timeout, interval).Should(Succeed())
			realRestoreSession := &formolv1alpha1.RestoreSession{}
			Eventually(func() error {
				return k8sClient.Get(ctx, key, realRestoreSession)
			}, timeout, interval).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				_ = k8sClient.Get(ctx, key, realRestoreSession)
				return realRestoreSession.Status.SessionState
			}, timeout, interval).Should(Equal(formolv1alpha1.Running))
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, restoreSession)).Should(Succeed())
		})
		It("Should have a new task and should fail if the task fails", func() {
			restoreSession := &formolv1alpha1.RestoreSession{}
			Expect(k8sClient.Get(ctx, key, restoreSession)).Should(Succeed())
			Expect(len(restoreSession.Status.Targets)).Should(Equal(1))
			Expect(restoreSession.Status.Targets[0].SessionState).Should(Equal(formolv1alpha1.New))
			restoreSession.Status.Targets[0].SessionState = formolv1alpha1.Running
			Expect(k8sClient.Status().Update(ctx, restoreSession)).Should(Succeed())
			Expect(k8sClient.Get(ctx, key, restoreSession)).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				_ = k8sClient.Get(ctx, key, restoreSession)
				return restoreSession.Status.Targets[0].SessionState
			}, timeout, interval).Should(Equal(formolv1alpha1.Running))
			restoreSession.Status.Targets[0].SessionState = formolv1alpha1.Failure
			Expect(k8sClient.Status().Update(ctx, restoreSession)).Should(Succeed())
			Expect(k8sClient.Get(ctx, key, restoreSession)).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				_ = k8sClient.Get(ctx, key, restoreSession)
				return restoreSession.Status.SessionState
			}, timeout, interval).Should(Equal(formolv1alpha1.Failure))
		})
		It("Should move to the new task if the first one is a success and be a success if all the tasks succeed", func() {
			restoreSession := &formolv1alpha1.RestoreSession{}
			Expect(k8sClient.Get(ctx, key, restoreSession)).Should(Succeed())
			Expect(len(restoreSession.Status.Targets)).Should(Equal(1))
			restoreSession.Status.Targets[0].SessionState = formolv1alpha1.Success
			Expect(k8sClient.Status().Update(ctx, restoreSession)).Should(Succeed())
			Eventually(func() int {
				_ = k8sClient.Get(ctx, key, restoreSession)
				return len(restoreSession.Status.Targets)
			}, timeout, interval).Should(Equal(2))
			restoreSession.Status.Targets[1].SessionState = formolv1alpha1.Success
			Expect(k8sClient.Status().Update(ctx, restoreSession)).Should(Succeed())
			Eventually(func() formolv1alpha1.SessionState {
				_ = k8sClient.Get(ctx, key, restoreSession)
				return restoreSession.Status.SessionState
			}, timeout, interval).Should(Equal(formolv1alpha1.Success))
		})
	})
})
