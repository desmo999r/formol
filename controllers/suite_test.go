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
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	//+kubebuilder:scaffold:imports

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	NAMESPACE_NAME  = "test-namespace"
	REPO_NAME       = "test-repo"
	DEPLOYMENT_NAME = "test-deployment"
	CONTAINER_NAME  = "test-container"
	DATAVOLUME_NAME = "data"
	SECRET_NAME     = "test-secret"
	timeout         = time.Second * 10
	interval        = time.Millisecond * 250
)

var (
	namespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: NAMESPACE_NAME,
		},
	}
	deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: NAMESPACE_NAME,
			Name:      DEPLOYMENT_NAME,
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
							Name: DATAVOLUME_NAME,
						},
					},
				},
			},
		},
	}
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SECRET_NAME,
			Namespace: NAMESPACE_NAME,
		},
		Data: map[string][]byte{
			"RESTIC_PASSWORD":       []byte("toto"),
			"AWS_ACCESS_KEY_ID":     []byte("titi"),
			"AWS_SECRET_ACCESS_KEY": []byte("tata"),
		},
	}
	repo = &formolv1alpha1.Repo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      REPO_NAME,
			Namespace: NAMESPACE_NAME,
		},
		Spec: formolv1alpha1.RepoSpec{
			Backend: formolv1alpha1.Backend{
				S3: &formolv1alpha1.S3{
					Server: "raid5.desmojim.fr:9000",
					Bucket: "testbucket2",
				},
			},
			RepositorySecrets: "test-secret",
		},
	}
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = formolv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
	Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
	Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	Expect(k8sClient.Create(ctx, repo)).Should(Succeed())
	Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&BackupConfigurationReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
