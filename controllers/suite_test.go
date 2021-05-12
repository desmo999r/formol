/*


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
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.
const (
	TestBackupFuncName    = "test-backup-func"
	TestFunc              = "test-norestore-func"
	TestRestoreFuncName   = "test-restore-func"
	TestNamespace         = "test-namespace"
	TestRepoName          = "test-repo"
	TestDeploymentName    = "test-deployment"
	TestBackupConfName    = "test-backupconf"
	TestBackupSessionName = "test-backupsession"
	TestDataVolume        = "data"
	TestDataMountPath     = "/data"
	timeout               = time.Second * 10
	interval              = time.Millisecond * 250
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

var (
	namespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestNamespace,
		},
	}
	deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestDeploymentName,
			Namespace: TestNamespace,
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
					Volumes: []corev1.Volume{
						corev1.Volume{
							Name: TestDataVolume,
						},
					},
				},
			},
		},
	}
	sa = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: TestNamespace,
		},
	}
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: TestNamespace,
		},
		Data: map[string][]byte{
			"RESTIC_PASSWORD":       []byte("toto"),
			"AWS_ACCESS_KEY_ID":     []byte("titi"),
			"AWS_SECRET_ACCESS_KEY": []byte("tata"),
		},
	}
	repo = &formolv1alpha1.Repo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestRepoName,
			Namespace: TestNamespace,
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
	function = &formolv1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestFunc,
			Namespace: TestNamespace,
		},
		Spec: corev1.Container{
			Name:  "norestore-func",
			Image: "myimage",
			Args:  []string{"a", "set", "of", "args"},
		},
	}
	backupFunc = &formolv1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestRestoreFuncName,
			Namespace: TestNamespace,
		},
		Spec: corev1.Container{
			Name:  "restore-func",
			Image: "myimage",
			Args:  []string{"a", "set", "of", "args"},
		},
	}
	restoreFunc = &formolv1alpha1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestBackupFuncName,
			Namespace: TestNamespace,
		},
		Spec: corev1.Container{
			Name:  "backup-func",
			Image: "myimage",
			Args:  []string{"a", "set", "of", "args"},
		},
	}
	testBackupConf = &formolv1alpha1.BackupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestBackupConfName,
			Namespace: TestNamespace,
		},
		Spec: formolv1alpha1.BackupConfigurationSpec{
			Repository: TestRepoName,
			Image:      "desmo999r/formolcli:latest",
			Schedule:   "1 * * * *",
			Keep: formolv1alpha1.Keep{
				Last: 2,
			},
			Targets: []formolv1alpha1.Target{
				formolv1alpha1.Target{
					Kind: formolv1alpha1.SidecarKind,
					Name: TestDeploymentName,
					Steps: []formolv1alpha1.Step{
						formolv1alpha1.Step{
							Name: TestFunc,
						},
					},
					Paths: []string{
						TestDataMountPath,
					},
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      TestDataVolume,
							MountPath: TestDataMountPath,
						},
					},
				},
				formolv1alpha1.Target{
					Kind: formolv1alpha1.JobKind,
					Name: TestBackupFuncName,
					Steps: []formolv1alpha1.Step{
						formolv1alpha1.Step{
							Name: TestFunc,
						},
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
	testBackupSession = &formolv1alpha1.BackupSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestBackupSessionName,
			Namespace: TestNamespace,
		},
		Spec: formolv1alpha1.BackupSessionSpec{
			Ref: corev1.ObjectReference{
				Name:      TestBackupConfName,
				Namespace: TestNamespace,
			},
		},
	}
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = formolv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&BackupConfigurationReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("BackupConfiguration"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&BackupSessionReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("BackupSession"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&RestoreSessionReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("RestoreSession"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient = k8sManager.GetClient()
	ctx := context.Background()
	Expect(k8sClient).ToNot(BeNil())
	Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
	Expect(k8sClient.Create(ctx, sa)).Should(Succeed())
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	Expect(k8sClient.Create(ctx, repo)).Should(Succeed())
	Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())
	Expect(k8sClient.Create(ctx, function)).Should(Succeed())
	Expect(k8sClient.Create(ctx, backupFunc)).Should(Succeed())
	Expect(k8sClient.Create(ctx, restoreFunc)).Should(Succeed())
	Expect(k8sClient.Create(ctx, testBackupConf)).Should(Succeed())
	Expect(k8sClient.Create(ctx, testBackupSession)).Should(Succeed())
	Eventually(func() error {
		return k8sClient.Get(ctx, client.ObjectKey{
			Name:      TestBackupSessionName,
			Namespace: TestNamespace,
		}, testBackupSession)
	}, timeout, interval).Should(Succeed())
	testBackupSession.Status.SessionState = formolv1alpha1.Success
	testBackupSession.Status.Targets = []formolv1alpha1.TargetStatus{
		formolv1alpha1.TargetStatus{
			Name:         TestDeploymentName,
			Kind:         formolv1alpha1.SidecarKind,
			SessionState: formolv1alpha1.Success,
			SnapshotId:   "12345abcdef",
		},
		formolv1alpha1.TargetStatus{
			Name:         TestBackupFuncName,
			Kind:         formolv1alpha1.JobKind,
			SessionState: formolv1alpha1.Success,
			SnapshotId:   "67890ghijk",
		},
	}
	Expect(k8sClient.Status().Update(ctx, testBackupSession)).Should(Succeed())
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
