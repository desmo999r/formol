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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupSessionReconciler reconciles a BackupSession object
type BackupSessionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions/status,verbs=get;update;patch

func (r *BackupSessionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("backupsession", req.NamespacedName)

	// your logic here
	var backupSession formolv1alpha1.BackupSession
	if err := r.Get(ctx, req.NamespacedName, &backupSession); err != nil {
		log.Error(err, "unable to get backupsession")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var backupConf formolv1alpha1.BackupConfiguration
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: backupSession.Spec.Ref.Namespace,
		Name:      backupSession.Spec.Ref.Name}, &backupConf); err != nil {
		log.Error(err, "unable to get backupConfiguration")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(1).Info("Found BackupConfiguration", "BackupConfiguration", backupConf)
	var childJobs batchv1.JobList
	if err := r.List(ctx, &childJobs, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: backupSession.Spec.Ref.Name}); err != nil {
		log.Error(err, "unable to list child jobs")
		return ctrl.Result{}, err
	}

	var activeJobs []*batchv1.Job
	var successfulJobs []*batchv1.Job
	var failedJobs []*batchv1.Job

	isJobFinished := func(job *batchv1.Job) (bool, batchv1.JobConditionType) {
		for _, c := range job.Status.Conditions {
			if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
				return true, c.Type
			}
		}
		return false, ""
	}

	for i, job := range childJobs.Items {
		_, finishedType := isJobFinished(&job)
		switch finishedType {
		case "": // running
			activeJobs = append(activeJobs, &childJobs.Items[i])
		case batchv1.JobFailed:
			failedJobs = append(failedJobs, &childJobs.Items[i])
		case batchv1.JobComplete:
			successfulJobs = append(successfulJobs, &childJobs.Items[i])
		}
	}

	if len(activeJobs) > 0 {
		log.V(0).Info("A backup job is already running. Skipping")
		return ctrl.Result{}, nil
	}

	constructJobForBackupConfiguration := func(backupConf formolv1alpha1.BackupConfiguration) (*batchv1.Job, error) {
		name := fmt.Sprintf("%s-%d", backupConf.Name, time.Now().Unix())
		log.V(1).Info("constructing a new Job", "name", name)
		task := &formolv1alpha1.Task{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: "backup",
			Name:      backupConf.Spec.Task,
		}, task); err != nil {
			log.Error(err, "unable to get Task from BackupConfiguration")
			return nil, err
		}
		log.V(1).Info("found task", "task", task.Name)
		containers := []corev1.Container{}
		for _, step := range task.Spec.Steps {
			function := &formolv1alpha1.Function{}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: "backup",
				Name:      step.Name,
			}, function); err != nil {
				log.Error(err, "unable to get Function")
				return nil, err
			}
			log.V(1).Info("found function", "function", function.Name)
			containers = append(containers, *function.Spec.DeepCopy())
		}
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        name,
				Namespace:   backupConf.Namespace,
			},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers:    containers,
						RestartPolicy: corev1.RestartPolicyOnFailure,
					},
				},
			},
		}
		return job, nil

	}
	job, err := constructJobForBackupConfiguration(backupConf)
	if err != nil {
		log.Error(err, "unable to construct job")
		return ctrl.Result{}, nil
	}

	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "unable to create job")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

var (
	jobOwnerKey = ".metadata.controller"
	apiGVStr    = formolv1alpha1.GroupVersion.String()
)

func (r *BackupSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(&batchv1.Job{}, jobOwnerKey, func(rawObj runtime.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGVStr || owner.Kind != "BackupSession" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupSession{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
