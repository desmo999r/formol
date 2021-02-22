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
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	formolutils "github.com/desmo999r/formol/pkg/utils"
)

const (
	sessionState  string = ".metadata.state"
	finalizerName string = "finalizer.backupsession.formol.desmojim.fr"
	JOBTTL int32 = 7200
)

// BackupSessionReconciler reconciles a BackupSession object
type BackupSessionReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	BackupSession *formolv1alpha1.BackupSession
	BackupConf    *formolv1alpha1.BackupConfiguration
}

func (r *BackupSessionReconciler) StatusUpdate() error {
	log := r.Log.WithValues("backupsession-statusupdate", r.BackupSession)
	ctx := context.Background()
	// start the next task
	startNextTask := func() (*formolv1alpha1.TargetStatus, error) {
		nextTarget := len(r.BackupSession.Status.Targets)
		if nextTarget < len(r.BackupConf.Spec.Targets) {
			target := r.BackupConf.Spec.Targets[nextTarget]
			targetStatus := formolv1alpha1.TargetStatus{
				Name:         target.Name,
				Kind:         target.Kind,
				SessionState: formolv1alpha1.New,
				StartTime:    &metav1.Time{Time: time.Now()},
			}
			r.BackupSession.Status.Targets = append(r.BackupSession.Status.Targets, targetStatus)
			switch target.Kind {
			case "Task":
				if err := r.CreateBackupJob(target); err != nil {
					log.V(0).Info("unable to create task", "task", target)
					targetStatus.SessionState = formolv1alpha1.Failure
					return nil, err
				}
			}
			return &targetStatus, nil
		} else {
			return nil, nil
		}
	}
	// Test the backupsession backupstate to decide what to do
	switch r.BackupSession.Status.SessionState {
	case formolv1alpha1.New:
		// Brand new backupsession; start the first task
		r.BackupSession.Status.SessionState = formolv1alpha1.Running
		targetStatus, err := startNextTask()
		if err != nil {
			return err
		}
		log.V(0).Info("New backup. Start the first task", "task", targetStatus)
		if err := r.Status().Update(ctx, r.BackupSession); err != nil {
			log.Error(err, "unable to update BackupSession status")
			return err
		}
	case formolv1alpha1.Running:
		// Backup ongoing. Check the status of the last task to decide what to do
		currentTargetStatus := r.BackupSession.Status.Targets[len(r.BackupSession.Status.Targets)-1]
		switch currentTargetStatus.SessionState {
		case formolv1alpha1.Failure:
			// The last task failed. We mark the backupsession as failed and we stop here.
			log.V(0).Info("last backup task failed. Stop here", "targetStatus", currentTargetStatus)
			r.BackupSession.Status.SessionState = formolv1alpha1.Failure
			log.V(1).Info("New BackupSession status", "status", r.BackupSession.Status.SessionState)
			if err := r.Status().Update(ctx, r.BackupSession); err != nil {
				log.Error(err, "unable to update BackupSession status")
				return err
			}
		case formolv1alpha1.Running:
			// The current task is still running. Nothing to do
			log.V(0).Info("task is still running", "targetStatus", currentTargetStatus)
		case formolv1alpha1.Success:
			// The last task successed. Let's try to start the next one
			log.V(0).Info("last task was a success. start a new one", "currentTargetStatus", currentTargetStatus, "targetStatus", currentTargetStatus)
			targetStatus, err := startNextTask()
			if err != nil {
				return err
			}
			if targetStatus == nil {
				// No more task to start. The backup is a success
				r.BackupSession.Status.SessionState = formolv1alpha1.Success
				log.V(0).Info("Backup is successful. Let's try to do some cleanup")
				backupSessionList := &formolv1alpha1.BackupSessionList{}
				if err := r.List(ctx, backupSessionList, client.InNamespace(r.BackupConf.Namespace), client.MatchingFieldsSelector{Selector: fields.SelectorFromSet(fields.Set{sessionState: "Success"})}); err != nil {
					log.Error(err, "unable to get backupsessionlist")
					return nil
				}
				if len(backupSessionList.Items) < 2 {
					// Not enough backupSession to proceed
					log.V(1).Info("Not enough successful backup jobs")
					break
				}

				sort.Slice(backupSessionList.Items, func(i, j int) bool {
					return backupSessionList.Items[i].Status.StartTime.Time.Unix() > backupSessionList.Items[j].Status.StartTime.Time.Unix()
				})

				type KeepBackup struct {
					Counter int32
					Last    time.Time
				}

				var lastBackups, dailyBackups, weeklyBackups, monthlyBackups, yearlyBackups KeepBackup
				lastBackups.Counter = r.BackupConf.Spec.Keep.Last
				dailyBackups.Counter = r.BackupConf.Spec.Keep.Daily
				weeklyBackups.Counter = r.BackupConf.Spec.Keep.Weekly
				monthlyBackups.Counter = r.BackupConf.Spec.Keep.Monthly
				yearlyBackups.Counter = r.BackupConf.Spec.Keep.Yearly
				for _, session := range backupSessionList.Items {
					if session.Spec.Ref.Name != r.BackupConf.Name {
						continue
					}
					deleteSession := true
					keep := []string{}
					if lastBackups.Counter > 0 {
						log.V(1).Info("Keep backup", "last", session.Status.StartTime)
						lastBackups.Counter--
						keep = append(keep, "last")
						deleteSession = false
					}
					if dailyBackups.Counter > 0 {
						if session.Status.StartTime.Time.YearDay() != dailyBackups.Last.YearDay() {
							log.V(1).Info("Keep backup", "daily", session.Status.StartTime)
							dailyBackups.Counter--
							dailyBackups.Last = session.Status.StartTime.Time
							keep = append(keep, "daily")
							deleteSession = false
						}
					}
					if weeklyBackups.Counter > 0 {
						if session.Status.StartTime.Time.Weekday().String() == "Sunday" && session.Status.StartTime.Time.YearDay() != weeklyBackups.Last.YearDay() {
							log.V(1).Info("Keep backup", "weekly", session.Status.StartTime)
							weeklyBackups.Counter--
							weeklyBackups.Last = session.Status.StartTime.Time
							keep = append(keep, "weekly")
							deleteSession = false
						}
					}
					if monthlyBackups.Counter > 0 {
						if session.Status.StartTime.Time.Day() == 1 && session.Status.StartTime.Time.Month() != monthlyBackups.Last.Month() {
							log.V(1).Info("Keep backup", "monthly", session.Status.StartTime)
							monthlyBackups.Counter--
							monthlyBackups.Last = session.Status.StartTime.Time
							keep = append(keep, "monthly")
							deleteSession = false
						}
					}
					if yearlyBackups.Counter > 0 {
						if session.Status.StartTime.Time.YearDay() == 1 && session.Status.StartTime.Time.Year() != yearlyBackups.Last.Year() {
							log.V(1).Info("Keep backup", "yearly", session.Status.StartTime)
							yearlyBackups.Counter--
							yearlyBackups.Last = session.Status.StartTime.Time
							keep = append(keep, "yearly")
							deleteSession = false
						}
					}
					if deleteSession {
						log.V(1).Info("Delete session", "delete", session.Status.StartTime)
						if err := r.Delete(ctx, &session); err != nil {
							log.Error(err, "unable to delete backupsession", "session", session.Name)
							// we don't return anything, we keep going
						}
					} else {
						session.Status.Keep = strings.Join(keep, ",") // + " " + time.Now().Format("2006 Jan 02 15:04:05 -0700 MST")
						if err := r.Status().Update(ctx, &session); err != nil {
							log.Error(err, "unable to update session status", "session", session)
						}
					}
				}
			}
			log.V(1).Info("New BackupSession status", "status", r.BackupSession.Status.SessionState)
			if err := r.Status().Update(ctx, r.BackupSession); err != nil {
				log.Error(err, "unable to update BackupSession status")
				return err
			}
		}
	case formolv1alpha1.Deleted:
		for _, target := range r.BackupSession.Status.Targets {
			if target.SessionState != formolv1alpha1.Deleted {
				log.V(1).Info("snaphot has not been deleted. won't delete the backupsession", "target", target)
				return nil
			}
		}
		log.V(1).Info("all the snapshots have been deleted. deleting the backupsession")
		controllerutil.RemoveFinalizer(r.BackupSession, finalizerName)
		if err := r.Update(ctx, r.BackupSession); err != nil {
			log.Error(err, "unable to remove finalizer")
			return err
		}
	}
	return nil
}

func (r *BackupSessionReconciler) IsBackupOngoing() bool {
	log := r.Log.WithName("IsBackupOngoing")
	ctx := context.Background()

	backupSessionList := &formolv1alpha1.BackupSessionList{}
	if err := r.List(ctx, backupSessionList, client.InNamespace(r.BackupConf.Namespace), client.MatchingFieldsSelector{Selector: fields.SelectorFromSet(fields.Set{sessionState: "Running"})}); err != nil {
		log.Error(err, "unable to get backupsessionlist")
		return true
	}
	if len(backupSessionList.Items) > 0 {
		return true
	} else {
		return false
	}
}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions/status,verbs=get;update;patch;create;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch

func (r *BackupSessionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("backupsession", req.NamespacedName)
	ctx := context.Background()

	// your logic here
	r.BackupSession = &formolv1alpha1.BackupSession{}
	if err := r.Get(ctx, req.NamespacedName, r.BackupSession); err != nil {
		log.Error(err, "unable to get backupsession")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(0).Info("backupSession", "backupSession.ObjectMeta", r.BackupSession.ObjectMeta, "backupSession.Status", r.BackupSession.Status)
	if r.BackupSession.Status.ObservedGeneration == r.BackupSession.ObjectMeta.Generation {
		// status update
		log.V(0).Info("status update")
		return ctrl.Result{}, r.StatusUpdate()
	}
	r.BackupSession.Status.ObservedGeneration = r.BackupSession.ObjectMeta.Generation
	r.BackupSession.Status.SessionState = formolv1alpha1.New
	// Prepare the next schedule to start the first task
	reschedule := ctrl.Result{RequeueAfter: 5 * time.Second}

	r.BackupSession.Status.StartTime = &metav1.Time{Time: time.Now()}
	r.BackupConf = &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: r.BackupSession.Namespace,
		Name:      r.BackupSession.Spec.Ref.Name}, r.BackupConf); err != nil {
		log.Error(err, "unable to get backupConfiguration")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if r.IsBackupOngoing() {
		// There is already a backup ongoing. We don't do anything and we reschedule
		log.V(0).Info("there is an ongoing backup. let's reschedule this operation")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if r.BackupSession.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(r.BackupSession, finalizerName) {
			controllerutil.AddFinalizer(r.BackupSession, finalizerName)
			if err := r.Update(ctx, r.BackupSession); err != nil {
				log.Error(err, "unable to add finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		log.V(0).Info("backupsession being deleted", "backupsession", r.BackupSession.Name)
		if controllerutil.ContainsFinalizer(r.BackupSession, finalizerName) {
			if err := r.deleteExternalResources(); err != nil {
				return ctrl.Result{}, err
			}
		}
		controllerutil.RemoveFinalizer(r.BackupSession, finalizerName)
		if err := r.Update(ctx, r.BackupSession); err != nil {
			log.Error(err, "unable to remove finalizer")
			return ctrl.Result{}, err
		}
		// We have been deleted. Return here
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, r.BackupSession); err != nil {
		log.Error(err, "unable to update backupSession")
		return ctrl.Result{}, err
	}

	return reschedule, nil
}

func (r *BackupSessionReconciler) CreateBackupJob(target formolv1alpha1.Target) error {
	log := r.Log.WithValues("createbackupjob", target.Name)
	ctx := context.Background()
	backupSessionEnv := []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "TARGET_NAME",
			Value: target.Name,
		},
		corev1.EnvVar{
			Name:  "BACKUPSESSION_NAME",
			Value: r.BackupSession.Name,
		},
		corev1.EnvVar{
			Name:  "BACKUPSESSION_NAMESPACE",
			Value: r.BackupSession.Namespace,
		},
	}

	output := corev1.VolumeMount{
		Name:      "output",
		MountPath: "/output",
	}
	restic := corev1.Container{
		Name:         "restic",
		Image:        "desmo999r/formolcli:latest",
		Args:         []string{"volume", "backup", "--tag", r.BackupSession.Name, "--path", "/output"},
		VolumeMounts: []corev1.VolumeMount{output},
		Env:          backupSessionEnv,
	}
	log.V(1).Info("creating a tagged backup job", "container", restic)
	// Gather information from the repo
	repo := &formolv1alpha1.Repo{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: r.BackupConf.Namespace,
		Name:      r.BackupConf.Spec.Repository.Name,
	}, repo); err != nil {
		log.Error(err, "unable to get Repo from BackupConfiguration")
		return err
	}
	// S3 backing storage
	restic.Env = append(restic.Env, formolutils.ConfigureResticEnvVar(r.BackupConf, repo)...)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", r.BackupSession.Name, target.Name),
			Namespace:    r.BackupConf.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &JOBTTL,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
					Containers:     []corev1.Container{restic},
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
			Namespace: step.Namespace,
			Name:      step.Name}, function); err != nil {
			log.Error(err, "unable to get function", "Function", step)
			return err
		}
		function.Spec.Env = append(step.Env, backupSessionEnv...)
		function.Spec.VolumeMounts = append(function.Spec.VolumeMounts, output)
		job.Spec.Template.Spec.InitContainers = append(job.Spec.Template.Spec.InitContainers, function.Spec)
	}
	if err := ctrl.SetControllerReference(r.BackupConf, job, r.Scheme); err != nil {
		log.Error(err, "unable to set controller on job", "job", job, "backupconf", r.BackupConf)
		return err
	}
	log.V(0).Info("creating a backup job", "target", target)
	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "unable to create job", "job", job)
		return err
	}
	return nil
}

func (r *BackupSessionReconciler) deleteExternalResources() error {
	ctx := context.Background()
	log := r.Log.WithValues("deleteExternalResources", r.BackupSession.Name)
	// Gather information from the repo
	repo := &formolv1alpha1.Repo{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: r.BackupConf.Namespace,
		Name:      r.BackupConf.Spec.Repository.Name,
	}, repo); err != nil {
		log.Error(err, "unable to get Repo from BackupConfiguration")
		return err
	}
	env := formolutils.ConfigureResticEnvVar(r.BackupConf, repo)
	// container that will delete the restic snapshot(s) matching the backupsession
	deleteSnapshots := []corev1.Container{}
	for _, target := range r.BackupSession.Status.Targets {
		if target.SessionState == formolv1alpha1.Success {
			deleteSnapshots = append(deleteSnapshots, corev1.Container{
				Name:  target.Name,
				Image: "desmo999r/formolcli:latest",
				Args:  []string{"snapshot", "delete", "--snapshot-id", target.SnapshotId},
				Env:   env,
			})
		}
	}
	// create a job to delete the restic snapshot(s) with the backupsession name tag
	if len(deleteSnapshots) > 0 {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("delete-%s-", r.BackupSession.Name),
				Namespace:    r.BackupSession.Namespace,
			},
			Spec: batchv1.JobSpec{
				TTLSecondsAfterFinished: &JOBTTL,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{},
						Containers:     deleteSnapshots,
						RestartPolicy:  corev1.RestartPolicyOnFailure,
					},
				},
			},
		}
		log.V(0).Info("creating a job to delete restic snapshots")
		if err := r.Create(ctx, job); err != nil {
			log.Error(err, "unable to delete job", "job", job)
			return err
		}
	}
	return nil
}

func (r *BackupSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &formolv1alpha1.BackupSession{}, sessionState, func(rawObj runtime.Object) []string {
		session := rawObj.(*formolv1alpha1.BackupSession)
		return []string{string(session.Status.SessionState)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupSession{}).
		//WithEventFilter(predicate.GenerationChangedPredicate{}). // Don't reconcile when status gets updated
		Owns(&batchv1.Job{}).
		Complete(r)
}
