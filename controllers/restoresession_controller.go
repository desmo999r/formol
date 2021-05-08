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
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	formolutils "github.com/desmo999r/formol/pkg/utils"
)

const (
	RESTORESESSION string = "restoresession"
	UPDATESTATUS   string = "updatestatus"
	jobOwnerKey    string = ".metadata.controller"
)

// RestoreSessionReconciler reconciles a RestoreSession object
type RestoreSessionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

var _ reconcile.Reconciler = &RestoreSessionReconciler{}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=restoresessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=restoresessions/status,verbs=get;update;patch

func (r *RestoreSessionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("restoresession", req.NamespacedName)

	// Get the RestoreSession
	restoreSession := &formolv1alpha1.RestoreSession{}
	if err := r.Get(ctx, req.NamespacedName, restoreSession); err != nil {
		log.Error(err, "unable to get restoresession")
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	log = r.Log.WithValues("restoresession", req.NamespacedName, "version", restoreSession.ObjectMeta.ResourceVersion)
	// Get the BackupSession the RestoreSession references
	backupSession := &formolv1alpha1.BackupSession{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: restoreSession.Namespace,
		Name:      restoreSession.Spec.BackupSessionRef.Ref.Name,
	}, backupSession); err != nil {
		if errors.IsNotFound(err) {
			backupSession = &formolv1alpha1.BackupSession{
				Spec:   restoreSession.Spec.BackupSessionRef.Spec,
				Status: restoreSession.Spec.BackupSessionRef.Status,
			}
			log.V(1).Info("generated backupsession", "spec", backupSession.Spec, "status", backupSession.Status)
		} else {
			log.Error(err, "unable to get backupsession", "restoresession", restoreSession.Spec)
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
	}
	// Get the BackupConfiguration linked to the BackupSession
	backupConf := &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: backupSession.Spec.Ref.Namespace,
		Name:      backupSession.Spec.Ref.Name,
	}, backupConf); err != nil {
		log.Error(err, "unable to get backupConfiguration", "name", backupSession.Spec.Ref, "namespace", backupSession.Namespace)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Helper functions
	createRestoreJob := func(target formolv1alpha1.Target, snapshotId string) error {
		// TODO: Get the list of existing jobs and see if there is already one scheduled for the target
		var jobList batchv1.JobList
		if err := r.List(ctx, &jobList, client.InNamespace(restoreSession.Namespace), client.MatchingFields{jobOwnerKey: restoreSession.Name}); err != nil {
			log.Error(err, "unable to get job list")
			return err
		}
		log.V(1).Info("Found jobs", "jobs", jobList.Items)
		for _, job := range jobList.Items {
			if job.Annotations["targetName"] == target.Name && job.Annotations["snapshotId"] == snapshotId {
				log.V(0).Info("there is already a cronjob to restore that target", "targetName", target.Name, "snapshotId", snapshotId)
				return nil
			}
		}
		restoreSessionEnv := []corev1.EnvVar{
			corev1.EnvVar{
				Name:  "TARGET_NAME",
				Value: target.Name,
			},
			corev1.EnvVar{
				Name:  "RESTORESESSION_NAME",
				Value: restoreSession.Name,
			},
			corev1.EnvVar{
				Name:  "RESTORESESSION_NAMESPACE",
				Value: restoreSession.Namespace,
			},
		}

		output := corev1.VolumeMount{
			Name:      "output",
			MountPath: "/output",
		}
		//for _, targetStatus := range backupSession.Status.Targets {
		//if targetStatus.Name == target.Name {
		//snapshotId := targetStatus.SnapshotId
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
			Namespace: backupConf.Namespace,
			Name:      backupConf.Spec.Repository,
		}, repo); err != nil {
			log.Error(err, "unable to get Repo from BackupConfiguration")
			return err
		}
		// S3 backing storage
		var ttl int32 = 300
		restic.Env = append(restic.Env, formolutils.ConfigureResticEnvVar(backupConf, repo)...)
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-%s-", restoreSession.Name, target.Name),
				Namespace:    restoreSession.Namespace,
				Annotations: map[string]string{
					"targetName": target.Name,
					"snapshotId": snapshotId,
				},
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
			// get the backup function
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: restoreSession.Namespace,
				Name:      step.Name,
			}, function); err != nil {
				log.Error(err, "unable to get backup function", "name", step.Name)
				return err
			}
			var restoreName string
			if function.Annotations["restoreFunction"] != "" {
				restoreName = function.Annotations["restoreFunction"]
			} else {
				restoreName = strings.Replace(step.Name, "backup", "restore", 1)
			}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: restoreSession.Namespace,
				Name:      restoreName,
			}, function); err != nil {
				log.Error(err, "unable to get function", "function", step)
				return err
			}
			function.Spec.Name = function.Name
			function.Spec.Env = append(step.Env, restoreSessionEnv...)
			function.Spec.VolumeMounts = append(function.Spec.VolumeMounts, output)
			job.Spec.Template.Spec.InitContainers = append(job.Spec.Template.Spec.InitContainers, function.Spec)
		}
		if err := ctrl.SetControllerReference(restoreSession, job, r.Scheme); err != nil {
			log.Error(err, "unable to set controller on job", "job", job, "restoresession", restoreSession)
			return err
		}
		log.V(0).Info("creating a restore job", "target", target.Name)
		if err := r.Create(ctx, job); err != nil {
			log.Error(err, "unable to create job", "job", job)
			return err
			//}
			//}
		}
		return nil
	}

	deleteRestoreInitContainer := func(target formolv1alpha1.Target) (err error) {
		deployment := &appsv1.Deployment{}
		if err = r.Get(context.Background(), client.ObjectKey{
			Namespace: backupConf.Namespace,
			Name:      target.Name,
		}, deployment); err != nil {
			log.Error(err, "unable to get deployment")
			return err
		}
		log.V(1).Info("got deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		newInitContainers := []corev1.Container{}
		for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
			if initContainer.Name == RESTORESESSION {
				log.V(0).Info("Found our restoresession container. Removing it from the list of init containers", "container", initContainer)
				defer func() {
					if err = r.Update(ctx, deployment); err != nil {
						log.Error(err, "unable to update deployment")
					}
				}()
			} else {
				newInitContainers = append(newInitContainers, initContainer)
			}
		}
		deployment.Spec.Template.Spec.InitContainers = newInitContainers
		return nil
	}

	createRestoreInitContainer := func(target formolv1alpha1.Target, snapshotId string) error {
		deployment := &appsv1.Deployment{}
		if err := r.Get(context.Background(), client.ObjectKey{
			Namespace: restoreSession.Namespace,
			Name:      target.Name,
		}, deployment); err != nil {
			log.Error(err, "unable to get deployment")
			return err
		}
		log.V(1).Info("got deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
			if initContainer.Name == RESTORESESSION {
				log.V(0).Info("there is already a restoresession initcontainer", "deployment", deployment.Spec.Template.Spec.InitContainers)
				return nil
			}
		}
		//for _, targetStatus := range backupSession.Status.Targets {
		//if targetStatus.Name == target.Name && targetStatus.Kind == target.Kind {
		//snapshotId := targetStatus.SnapshotId
		restoreSessionEnv := []corev1.EnvVar{
			corev1.EnvVar{
				Name:  formolv1alpha1.TARGET_NAME,
				Value: target.Name,
			},
			corev1.EnvVar{
				Name:  formolv1alpha1.RESTORESESSION_NAME,
				Value: restoreSession.Name,
			},
			corev1.EnvVar{
				Name:  formolv1alpha1.RESTORESESSION_NAMESPACE,
				Value: restoreSession.Namespace,
			},
		}
		initContainer := corev1.Container{
			Name:         RESTORESESSION,
			Image:        formolutils.FORMOLCLI,
			Args:         []string{"volume", "restore", "--snapshot-id", snapshotId},
			VolumeMounts: target.VolumeMounts,
			Env:          restoreSessionEnv,
		}
		repo := &formolv1alpha1.Repo{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: backupConf.Namespace,
			Name:      backupConf.Spec.Repository,
		}, repo); err != nil {
			log.Error(err, "unable to get Repo from BackupConfiguration")
			return err
		}
		// S3 backing storage
		initContainer.Env = append(initContainer.Env, formolutils.ConfigureResticEnvVar(backupConf, repo)...)
		deployment.Spec.Template.Spec.InitContainers = append([]corev1.Container{initContainer},
			deployment.Spec.Template.Spec.InitContainers...)
		if err := r.Update(ctx, deployment); err != nil {
			log.Error(err, "unable to update deployment")
			return err
		}

		return nil
		//}
		//}
		//return nil
	}

	startNextTask := func() (*formolv1alpha1.TargetStatus, error) {
		nextTarget := len(restoreSession.Status.Targets)
		if nextTarget < len(backupConf.Spec.Targets) {
			target := backupConf.Spec.Targets[nextTarget]
			targetStatus := formolv1alpha1.TargetStatus{
				Name:         target.Name,
				Kind:         target.Kind,
				SessionState: formolv1alpha1.New,
				StartTime:    &metav1.Time{Time: time.Now()},
			}
			restoreSession.Status.Targets = append(restoreSession.Status.Targets, targetStatus)
			switch target.Kind {
			case formolv1alpha1.SidecarKind:
				if err := createRestoreInitContainer(target, backupSession.Status.Targets[nextTarget].SnapshotId); err != nil {
					log.V(0).Info("unable to create restore init container", "task", target)
					targetStatus.SessionState = formolv1alpha1.Failure
					return nil, err
				}
			case formolv1alpha1.JobKind:
				if err := createRestoreJob(target, backupSession.Status.Targets[nextTarget].SnapshotId); err != nil {
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

	endTask := func() error {
		target := backupConf.Spec.Targets[len(restoreSession.Status.Targets)-1]
		switch target.Kind {
		case formolv1alpha1.SidecarKind:
			if err := deleteRestoreInitContainer(target); err != nil {
				log.Error(err, "unable to delete restore init container")
				return err
			}
		}
		return nil
	}

	switch restoreSession.Status.SessionState {
	case formolv1alpha1.New:
		restoreSession.Status.SessionState = formolv1alpha1.Running
		if targetStatus, err := startNextTask(); err != nil {
			log.Error(err, "unable to start next restore task")
			return reconcile.Result{}, err
		} else {
			log.V(0).Info("New restore. Start the first task", "task", targetStatus.Name)
			if err := r.Status().Update(ctx, restoreSession); err != nil {
				log.Error(err, "unable to update restoresession")
				return reconcile.Result{}, err
			}
		}
	case formolv1alpha1.Running:
		currentTargetStatus := &restoreSession.Status.Targets[len(restoreSession.Status.Targets)-1]
		switch currentTargetStatus.SessionState {
		case formolv1alpha1.Failure:
			log.V(0).Info("last restore task failed. Stop here", "target", currentTargetStatus.Name)
			restoreSession.Status.SessionState = formolv1alpha1.Failure
			if err := r.Status().Update(ctx, restoreSession); err != nil {
				log.Error(err, "unable to update restoresession")
				return reconcile.Result{}, err
			}
		case formolv1alpha1.Running:
			log.V(0).Info("task is still running", "target", currentTargetStatus.Name)
			return reconcile.Result{}, nil
		case formolv1alpha1.Waiting:
			target := backupConf.Spec.Targets[len(restoreSession.Status.Targets)-1]
			if target.Kind == formolv1alpha1.SidecarKind {
				deployment := &appsv1.Deployment{}
				if err := r.Get(context.Background(), client.ObjectKey{
					Namespace: restoreSession.Namespace,
					Name:      target.Name,
				}, deployment); err != nil {
					log.Error(err, "unable to get deployment")
					return reconcile.Result{}, err
				}

				if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
					log.V(0).Info("The deployment is ready. We can resume the backup")
					currentTargetStatus.SessionState = formolv1alpha1.Finalize
					if err := r.Status().Update(ctx, restoreSession); err != nil {
						log.Error(err, "unable to update restoresession")
						return reconcile.Result{}, err
					}
				} else {
					log.V(0).Info("Waiting for the sidecar to come back")
					return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
				}
			} else {
				log.V(0).Info("not a SidecarKind. Ignoring Waiting")
			}
		case formolv1alpha1.Success:
			_ = endTask()
			log.V(0).Info("last task was a success. start a new one", "target", currentTargetStatus, "restoreSession version", restoreSession.ObjectMeta.ResourceVersion)
			targetStatus, err := startNextTask()
			if err != nil {
				return reconcile.Result{}, err
			}
			if targetStatus == nil {
				// No more task to start. The restore is over
				restoreSession.Status.SessionState = formolv1alpha1.Success
			}
			if err := r.Status().Update(ctx, restoreSession); err != nil {
				log.Error(err, "unable to update restoresession")
				return reconcile.Result{RequeueAfter: 300 * time.Millisecond}, nil
			}
		}
	case "":
		// Restore session has just been created
		restoreSession.Status.SessionState = formolv1alpha1.New
		restoreSession.Status.StartTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, restoreSession); err != nil {
			log.Error(err, "unable to update restoreSession")
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *RestoreSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, jobOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != formolv1alpha1.GroupVersion.String() || owner.Kind != "RestoreSession" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.RestoreSession{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
