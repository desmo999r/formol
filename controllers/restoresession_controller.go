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
	"strings"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	formolutils "github.com/desmo999r/formol/pkg/utils"
)

var (
	RESTORESESSION = "restoresession"
)

// RestoreSessionReconciler reconciles a RestoreSession object
type RestoreSessionReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	RestoreSession *formolv1alpha1.RestoreSession
	BackupSession  *formolv1alpha1.BackupSession
	BackupConf     *formolv1alpha1.BackupConfiguration
}

func (r *RestoreSessionReconciler) CreateRestoreJob(target formolv1alpha1.Target) error {
	log := r.Log.WithValues("createrestorejob", target.Name)
	ctx := context.Background()
	restoreSessionEnv := []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "TARGET_NAME",
			Value: target.Name,
		},
		corev1.EnvVar{
			Name:  "RESTORESESSION_NAME",
			Value: r.RestoreSession.Name,
		},
		corev1.EnvVar{
			Name:  "RESTORESESSION_NAMESPACE",
			Value: r.RestoreSession.Namespace,
		},
	}

	output := corev1.VolumeMount{
		Name:      "output",
		MountPath: "/output",
	}
	for _, targetStatus := range r.BackupSession.Status.Targets {
		if targetStatus.Name == target.Name {
			snapshotId := targetStatus.SnapshotId
			restic := corev1.Container{
				Name:         "restic",
				Image:        "desmo999r/formolcli:latest",
				Args:         []string{"volume", "restore", "--snapshot-id", snapshotId},
				VolumeMounts: []corev1.VolumeMount{output},
				Env:          restoreSessionEnv,
			}
			finalizer := corev1.Container{
				Name:         "finalizer",
				Image:        "desmo999r/formolcli:latest",
				Args:         []string{"target", "finalize"},
				VolumeMounts: []corev1.VolumeMount{output},
				Env:          restoreSessionEnv,
			}
			repo := &formolv1alpha1.Repo{}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: r.BackupConf.Namespace,
				Name:      r.BackupConf.Spec.Repository.Name,
			}, repo); err != nil {
				log.Error(err, "unable to get Repo from BackupConfiguration")
				return err
			}
			// S3 backing storage
			var ttl int32 = 300
			restic.Env = append(restic.Env, formolutils.ConfigureResticEnvVar(r.BackupConf, repo)...)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("%s-%s-", r.RestoreSession.Name, target.Name),
					Namespace:    r.RestoreSession.Namespace,
				},
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: &ttl,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{restic},
							Containers:     []corev1.Container{finalizer},
							Volumes: []corev1.Volume{
								corev1.Volume{Name: "output"},
							},
							RestartPolicy: corev1.RestartPolicyOnFailure,
						},
					},
				},
			}
			for _, step := range target.Steps {
				function := &formolv1alpha1.Function{}
				if err := r.Get(ctx, client.ObjectKey{
					Namespace: r.RestoreSession.Namespace,
					Name:      strings.Replace(step.Name, "backup", "restore", 1)}, function); err != nil {
					log.Error(err, "unable to get function", "function", step)
					return err
				}
				function.Spec.Env = append(step.Env, restoreSessionEnv...)
				function.Spec.VolumeMounts = append(function.Spec.VolumeMounts, output)
				job.Spec.Template.Spec.InitContainers = append(job.Spec.Template.Spec.InitContainers, function.Spec)
			}
			if err := ctrl.SetControllerReference(r.RestoreSession, job, r.Scheme); err != nil {
				log.Error(err, "unable to set controller on job", "job", job, "restoresession", r.RestoreSession)
				return err
			}
			log.V(0).Info("creating a restore job", "target", target.Name)
			if err := r.Create(ctx, job); err != nil {
				log.Error(err, "unable to create job", "job", job)
				return err
			}
		}
	}
	return nil
}

func (r *RestoreSessionReconciler) CreateRestoreInitContainer(target formolv1alpha1.Target) error {
	return nil
}

func (r *RestoreSessionReconciler) StatusUpdate() error {
	log := r.Log.WithValues("statusupdate", r.RestoreSession.Name)
	ctx := context.Background()
	startNextTask := func() (*formolv1alpha1.TargetStatus, error) {
		nextTarget := len(r.RestoreSession.Status.Targets)
		if nextTarget < len(r.BackupConf.Spec.Targets) {
			target := r.BackupConf.Spec.Targets[nextTarget]
			targetStatus := formolv1alpha1.TargetStatus{
				Name:         target.Name,
				Kind:         target.Kind,
				SessionState: formolv1alpha1.New,
				StartTime:    &metav1.Time{Time: time.Now()},
			}
			r.RestoreSession.Status.Targets = append(r.RestoreSession.Status.Targets, targetStatus)
			switch target.Kind {
			case "Deployment":
				if err := r.CreateRestoreInitContainer(target); err != nil {
					log.V(0).Info("unable to create restore init container", "task", target)
					targetStatus.SessionState = formolv1alpha1.Failure
					return nil, err
				}
			case "Task":
				if err := r.CreateRestoreJob(target); err != nil {
					log.V(0).Info("unable to create restore job", "task", target)
					targetStatus.SessionState = formolv1alpha1.Failure
					return nil, err
				}
			}
			return &targetStatus, nil
		} else {
			return nil, nil
		}
	}
	switch r.RestoreSession.Status.SessionState {
	case formolv1alpha1.New:
		r.RestoreSession.Status.SessionState = formolv1alpha1.Running
		targetStatus, err := startNextTask()
		if err != nil {
			return err
		}
		log.V(0).Info("New restore. Start the first task", "task", targetStatus.Name)
	case formolv1alpha1.Running:
		currentTargetStatus := r.RestoreSession.Status.Targets[len(r.RestoreSession.Status.Targets)-1]
		switch currentTargetStatus.SessionState {
		case formolv1alpha1.Failure:
			log.V(0).Info("last restore task failed. Stop here", "target", currentTargetStatus.Name)
			r.RestoreSession.Status.SessionState = formolv1alpha1.Failure
			if err := r.Status().Update(ctx, r.RestoreSession); err != nil {
				log.Error(err, "unable to update restoresession")
				return err
			}
		case formolv1alpha1.Running:
			log.V(0).Info("task is still running", "target", currentTargetStatus.Name)
			return nil
		case formolv1alpha1.Success:
			log.V(0).Info("last task was a success. start a new one", "target", currentTargetStatus)
			targetStatus, err := startNextTask()
			if err != nil {
				return err
			}
			if targetStatus == nil {
				// No more task to start. The restore is over
				r.RestoreSession.Status.SessionState = formolv1alpha1.Success
				if err := r.Status().Update(ctx, r.RestoreSession); err != nil {
					log.Error(err, "unable to update restoresession")
					return err
				}
			}
		}
	}
	return nil
}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=restoresessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=restoresessions/status,verbs=get;update;patch

func (r *RestoreSessionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	time.Sleep(300 * time.Millisecond)
	ctx := context.Background()
	log := r.Log.WithValues("restoresession", req.NamespacedName)

	r.RestoreSession = &formolv1alpha1.RestoreSession{}
	if err := r.Get(ctx, req.NamespacedName, r.RestoreSession); err != nil {
		log.Error(err, "unable to get restoresession")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.BackupSession = &formolv1alpha1.BackupSession{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: r.RestoreSession.Spec.BackupSessionRef.Namespace,
		Name:      r.RestoreSession.Spec.BackupSessionRef.Name}, r.BackupSession); err != nil {
		log.Error(err, "unable to get backupsession")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.BackupConf = &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: r.BackupSession.Namespace,
		Name:      r.BackupSession.Spec.Ref.Name}, r.BackupConf); err != nil {
		log.Error(err, "unable to get backupConfiguration")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if r.RestoreSession.Status.ObservedGeneration == r.RestoreSession.ObjectMeta.Generation {
		// status update
		log.V(0).Info("status update")
		return ctrl.Result{}, r.StatusUpdate()
	}
	r.RestoreSession.Status.ObservedGeneration = r.RestoreSession.ObjectMeta.Generation
	r.RestoreSession.Status.SessionState = formolv1alpha1.New
	r.RestoreSession.Status.StartTime = &metav1.Time{Time: time.Now()}
	reschedule := ctrl.Result{RequeueAfter: 5 * time.Second}

	if err := r.Status().Update(ctx, r.RestoreSession); err != nil {
		log.Error(err, "unable to update restoresession")
		return ctrl.Result{}, err
	}

	return reschedule, nil
}

func (r *RestoreSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.RestoreSession{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
