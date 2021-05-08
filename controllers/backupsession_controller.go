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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	formolutils "github.com/desmo999r/formol/pkg/utils"
)

const (
	sessionState  string = ".metadata.state"
	finalizerName string = "finalizer.backupsession.formol.desmojim.fr"
	JOBTTL        int32  = 7200
)

// BackupSessionReconciler reconciles a BackupSession object
type BackupSessionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

var _ reconcile.Reconciler = &BackupSessionReconciler{}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions/status,verbs=get;update;patch;create;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=functions,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;patch;delete;watch

func (r *BackupSessionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.Log.WithValues("backupsession", req.NamespacedName)

	backupSession := &formolv1alpha1.BackupSession{}
	if err := r.Get(ctx, req.NamespacedName, backupSession); err != nil {
		log.Error(err, "unable to get backupsession")
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	backupConf := &formolv1alpha1.BackupConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: backupSession.Namespace,
		Name:      backupSession.Spec.Ref.Name,
	}, backupConf); err != nil {
		log.Error(err, "unable to get backupConfiguration")
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// helper functions
	// is there a backup operation ongoing
	isBackupOngoing := func() bool {
		backupSessionList := &formolv1alpha1.BackupSessionList{}
		if err := r.List(ctx, backupSessionList, client.InNamespace(backupConf.Namespace), client.MatchingFieldsSelector{Selector: fields.SelectorFromSet(fields.Set{sessionState: "Running"})}); err != nil {
			log.Error(err, "unable to get backupsessionlist")
			return true
		}
		return len(backupSessionList.Items) > 0
	}

	// delete session specific backup resources
	deleteExternalResources := func() error {
		log := r.Log.WithValues("deleteExternalResources", backupSession.Name)
		// Gather information from the repo
		repo := &formolv1alpha1.Repo{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: backupConf.Namespace,
			Name:      backupConf.Spec.Repository,
		}, repo); err != nil {
			log.Error(err, "unable to get Repo from BackupConfiguration")
			return err
		}
		env := formolutils.ConfigureResticEnvVar(backupConf, repo)
		// container that will delete the restic snapshot(s) matching the backupsession
		deleteSnapshots := []corev1.Container{}
		for _, target := range backupSession.Status.Targets {
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
			jobTtl := JOBTTL
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("delete-%s-", backupSession.Name),
					Namespace:    backupSession.Namespace,
				},
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: &jobTtl,
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

	// create a backup job
	createBackupJob := func(target formolv1alpha1.Target) error {
		log := r.Log.WithValues("createbackupjob", target.Name)
		ctx := context.Background()
		backupSessionEnv := []corev1.EnvVar{
			corev1.EnvVar{
				Name:  "TARGET_NAME",
				Value: target.Name,
			},
			corev1.EnvVar{
				Name:  "BACKUPSESSION_NAME",
				Value: backupSession.Name,
			},
			corev1.EnvVar{
				Name:  "BACKUPSESSION_NAMESPACE",
				Value: backupSession.Namespace,
			},
		}

		output := corev1.VolumeMount{
			Name:      "output",
			MountPath: "/output",
		}
		restic := corev1.Container{
			Name:         "restic",
			Image:        "desmo999r/formolcli:latest",
			Args:         []string{"volume", "backup", "--tag", backupSession.Name, "--path", "/output"},
			VolumeMounts: []corev1.VolumeMount{output},
			Env:          backupSessionEnv,
		}
		log.V(1).Info("creating a tagged backup job", "container", restic)
		// Gather information from the repo
		repo := &formolv1alpha1.Repo{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: backupConf.Namespace,
			Name:      backupConf.Spec.Repository,
		}, repo); err != nil {
			log.Error(err, "unable to get Repo from BackupConfiguration")
			return err
		}
		// S3 backing storage
		restic.Env = append(restic.Env, formolutils.ConfigureResticEnvVar(backupConf, repo)...)
		jobTtl := JOBTTL
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-%s-", backupSession.Name, target.Name),
				Namespace:    backupConf.Namespace,
			},
			Spec: batchv1.JobSpec{
				TTLSecondsAfterFinished: &jobTtl,
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
				Namespace: backupConf.Namespace,
				Name:      step.Name,
			}, function); err != nil {
				log.Error(err, "unable to get function", "Function", step)
				return err
			}
			function.Spec.Name = function.Name
			function.Spec.Env = append(step.Env, backupSessionEnv...)
			function.Spec.VolumeMounts = append(function.Spec.VolumeMounts, output)
			job.Spec.Template.Spec.InitContainers = append(job.Spec.Template.Spec.InitContainers, function.Spec)
		}
		if err := ctrl.SetControllerReference(backupConf, job, r.Scheme); err != nil {
			log.Error(err, "unable to set controller on job", "job", job, "backupconf", backupConf)
			return err
		}
		log.V(0).Info("creating a backup job", "target", target)
		if err := r.Create(ctx, job); err != nil {
			log.Error(err, "unable to create job", "job", job)
			return err
		}
		return nil
	}

	// start the next task
	startNextTask := func() (*formolv1alpha1.TargetStatus, error) {
		nextTarget := len(backupSession.Status.Targets)
		if nextTarget < len(backupConf.Spec.Targets) {
			target := backupConf.Spec.Targets[nextTarget]
			targetStatus := formolv1alpha1.TargetStatus{
				Name:         target.Name,
				Kind:         target.Kind,
				SessionState: formolv1alpha1.New,
				StartTime:    &metav1.Time{Time: time.Now()},
				Try:          1,
			}
			backupSession.Status.Targets = append(backupSession.Status.Targets, targetStatus)
			switch target.Kind {
			case formolv1alpha1.JobKind:
				if err := createBackupJob(target); err != nil {
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

	// cleanup existing backupsessions
	cleanupSessions := func() {
		backupSessionList := &formolv1alpha1.BackupSessionList{}
		if err := r.List(ctx, backupSessionList, client.InNamespace(backupConf.Namespace), client.MatchingFieldsSelector{Selector: fields.SelectorFromSet(fields.Set{sessionState: string(formolv1alpha1.Success)})}); err != nil {
			log.Error(err, "unable to get backupsessionlist")
			return
		}
		if len(backupSessionList.Items) < 2 {
			// Not enough backupSession to proceed
			log.V(1).Info("Not enough successful backup jobs")
			return
		}

		sort.Slice(backupSessionList.Items, func(i, j int) bool {
			return backupSessionList.Items[i].Status.StartTime.Time.Unix() > backupSessionList.Items[j].Status.StartTime.Time.Unix()
		})

		type KeepBackup struct {
			Counter int32
			Last    time.Time
		}

		var lastBackups, dailyBackups, weeklyBackups, monthlyBackups, yearlyBackups KeepBackup
		lastBackups.Counter = backupConf.Spec.Keep.Last
		dailyBackups.Counter = backupConf.Spec.Keep.Daily
		weeklyBackups.Counter = backupConf.Spec.Keep.Weekly
		monthlyBackups.Counter = backupConf.Spec.Keep.Monthly
		yearlyBackups.Counter = backupConf.Spec.Keep.Yearly
		for _, session := range backupSessionList.Items {
			if session.Spec.Ref.Name != backupConf.Name {
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
	// end helper functions

	log.V(0).Info("backupSession", "backupSession.ObjectMeta", backupSession.ObjectMeta, "backupSession.Status", backupSession.Status)
	if backupSession.ObjectMeta.DeletionTimestamp.IsZero() {
		switch backupSession.Status.SessionState {
		case formolv1alpha1.New:
			// Check if the finalizer has been registered
			if !controllerutil.ContainsFinalizer(backupSession, finalizerName) {
				controllerutil.AddFinalizer(backupSession, finalizerName)
				// We update the BackupSession to add the finalizer
				// Reconcile will be called again
				// return now
				err := r.Update(ctx, backupSession)
				if err != nil {
					log.Error(err, "unable to add finalizer")
				}
				return reconcile.Result{}, err
			}
			// Brand new backupsession
			if isBackupOngoing() {
				log.V(0).Info("There is an ongoing backup. Let's reschedule this operation")
				return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
			}
			// start the first task
			backupSession.Status.SessionState = formolv1alpha1.Running
			targetStatus, err := startNextTask()
			if err != nil {
				return reconcile.Result{}, err
			}
			log.V(0).Info("New backup. Start the first task", "task", targetStatus)
			if err := r.Status().Update(ctx, backupSession); err != nil {
				log.Error(err, "unable to update BackupSession status")
				return reconcile.Result{}, err
			}
		case formolv1alpha1.Running:
			// Backup ongoing. Check the status of the last task to decide what to do
			currentTargetStatus := &backupSession.Status.Targets[len(backupSession.Status.Targets)-1]
			switch currentTargetStatus.SessionState {
			case formolv1alpha1.Running:
				// The current task is still running. Nothing to do
				log.V(0).Info("task is still running", "targetStatus", currentTargetStatus)
			case formolv1alpha1.Success:
				// The last task succeed. Let's try to start the next one
				targetStatus, err := startNextTask()
				log.V(0).Info("last task was a success. start a new one", "currentTargetStatus", currentTargetStatus, "targetStatus", targetStatus)
				if err != nil {
					return reconcile.Result{}, err
				}
				if targetStatus == nil {
					// No more task to start. The backup is a success
					backupSession.Status.SessionState = formolv1alpha1.Success
					log.V(0).Info("Backup is successful. Let's try to do some cleanup")
					cleanupSessions()
				}
				if err := r.Status().Update(ctx, backupSession); err != nil {
					log.Error(err, "unable to update BackupSession status")
					return reconcile.Result{}, err
				}
			case formolv1alpha1.Failure:
				// last task failed. Try to run it again
				currentTarget := backupConf.Spec.Targets[len(backupSession.Status.Targets)-1]
				if currentTargetStatus.Try < currentTarget.Retry {
					log.V(0).Info("last task was a failure. try again", "currentTargetStatus", currentTargetStatus)
					currentTargetStatus.Try++
					currentTargetStatus.SessionState = formolv1alpha1.New
					currentTargetStatus.StartTime = &metav1.Time{Time: time.Now()}
					switch currentTarget.Kind {
					case formolv1alpha1.JobKind:
						if err := createBackupJob(currentTarget); err != nil {
							log.V(0).Info("unable to create task", "task", currentTarget)
							currentTargetStatus.SessionState = formolv1alpha1.Failure
							return reconcile.Result{}, err
						}
					}
				} else {
					log.V(0).Info("task failed again and for the last time", "currentTargetStatus", currentTargetStatus)
					backupSession.Status.SessionState = formolv1alpha1.Failure
				}
				if err := r.Status().Update(ctx, backupSession); err != nil {
					log.Error(err, "unable to update BackupSession status")
					return reconcile.Result{}, err
				}
			}
		case formolv1alpha1.Success:
			// Should never go there
		case formolv1alpha1.Failure:
			// The backup failed
		case "":
			// BackupSession has just been created
			backupSession.Status.SessionState = formolv1alpha1.New
			backupSession.Status.StartTime = &metav1.Time{Time: time.Now()}
			if err := r.Status().Update(ctx, backupSession); err != nil {
				log.Error(err, "unable to update backupSession")
				return reconcile.Result{}, err
			}
		}
	} else {
		log.V(0).Info("backupsession being deleted", "backupsession", backupSession.Name)
		if controllerutil.ContainsFinalizer(backupSession, finalizerName) {
			if err := deleteExternalResources(); err != nil {
				return reconcile.Result{}, err
			}
		}
		controllerutil.RemoveFinalizer(backupSession, finalizerName)
		if err := r.Update(ctx, backupSession); err != nil {
			log.Error(err, "unable to remove finalizer")
			return reconcile.Result{}, err
		}
		// We have been deleted. Return here
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *BackupSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &formolv1alpha1.BackupSession{}, sessionState, func(rawObj client.Object) []string {
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
