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
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	formolutils "github.com/desmo999r/formol/pkg/utils"
)

var (
	sessionState = ".metadata.state"
)

// BackupSessionReconciler reconciles a BackupSession object
type BackupSessionReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	BackupSession *formolv1alpha1.BackupSession
	BackupConf    *formolv1alpha1.BackupConfiguration
}

// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=formol.desmojim.fr,resources=backupsessions/status,verbs=get;update;patch

func (r *BackupSessionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("backupsession", req.NamespacedName)
	ctx := context.Background()

	// your logic here
	r.BackupSession = &formolv1alpha1.BackupSession{}
	if err := r.Get(ctx, req.NamespacedName, r.BackupSession); err != nil {
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

	log.V(1).Info("Found BackupConfiguration", "BackupConfiguration", r.BackupConf)

	finalizerName := "finalizer.backupsession.formol.desmojim.fr"

	if r.BackupSession.ObjectMeta.DeletionTimestamp.IsZero() {
		if !formolutils.ContainsString(r.BackupSession.ObjectMeta.Finalizers, finalizerName) {
			r.BackupSession.ObjectMeta.Finalizers = append(r.BackupSession.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, r.BackupSession); err != nil {
				log.Error(err, "unable to append finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		log.V(0).Info("backupsession being deleted", "backupsession", r.BackupSession.Name)
		if formolutils.ContainsString(r.BackupSession.ObjectMeta.Finalizers, finalizerName) {
			if err := r.deleteExternalResources(); err != nil {
				return ctrl.Result{}, err
			}
		}
		r.BackupSession.ObjectMeta.Finalizers = formolutils.RemoveString(r.BackupSession.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, r.BackupSession); err != nil {
			log.Error(err, "unable to remove finalizer")
			return ctrl.Result{}, err
		}
		// We have been deleted. Return here
		return ctrl.Result{}, nil
	}

	// Found the BackupConfiguration.
	for _, target := range r.BackupConf.Spec.Targets {
		switch target.Kind {
		case "task":
			if err := r.CreateJob(target); err != nil {
				log.V(0).Info("unable to create task", "task", target)
				return ctrl.Result{}, err
			}
		}
	}
	// cleanup old backups
	log.V(0).Info("try to cleanup old backups")
	backupSessionList := &formolv1alpha1.BackupSessionList{}
	if err := r.List(ctx, backupSessionList, client.InNamespace(r.BackupConf.Namespace), client.MatchingFieldsSelector{Selector: fields.SelectorFromSet(fields.Set{sessionState: "Success"})}); err != nil {
		log.Error(err, "unable to get backupsessionlist")
		return ctrl.Result{}, nil
	}
	if len(backupSessionList.Items) < 2 {
		// Not enough backupSession to proceed
		log.V(0).Info("Not enough successful backup jobs")
		return ctrl.Result{}, nil
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
		if lastBackups.Counter > 0 {
			log.V(1).Info("Keep backup", "last", session.Status.StartTime)
			lastBackups.Counter--
			deleteSession = false
		}
		if dailyBackups.Counter > 0 {
			if session.Status.StartTime.Time.YearDay() != dailyBackups.Last.YearDay() {
				log.V(1).Info("Keep backup", "daily", session.Status.StartTime)
				dailyBackups.Counter--
				dailyBackups.Last = session.Status.StartTime.Time
				deleteSession = false
			}
		}
		if weeklyBackups.Counter > 0 {
			if session.Status.StartTime.Time.Weekday().String() == "Sunday" && session.Status.StartTime.Time.YearDay() != weeklyBackups.Last.YearDay() {
				log.V(1).Info("Keep backup", "weekly", session.Status.StartTime)
				weeklyBackups.Counter--
				weeklyBackups.Last = session.Status.StartTime.Time
				deleteSession = false
			}
		}
		if monthlyBackups.Counter > 0 {
			if session.Status.StartTime.Time.Day() == 1 && session.Status.StartTime.Time.Month() != monthlyBackups.Last.Month() {
				log.V(1).Info("Keep backup", "monthly", session.Status.StartTime)
				monthlyBackups.Counter--
				monthlyBackups.Last = session.Status.StartTime.Time
				deleteSession = false
			}
		}
		if yearlyBackups.Counter > 0 {
			if session.Status.StartTime.Time.YearDay() == 1 && session.Status.StartTime.Time.Year() != yearlyBackups.Last.Year() {
				log.V(1).Info("Keep backup", "yearly", session.Status.StartTime)
				yearlyBackups.Counter--
				yearlyBackups.Last = session.Status.StartTime.Time
				deleteSession = false
			}
		}
		if deleteSession {
			log.V(1).Info("Delete session", "delete", session.Status.StartTime)
			if err := r.Delete(ctx, &session); err != nil {
				log.Error(err, "unable to delete backupsession", "session", session.Name)
				// we don't return anything, we keep going
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *BackupSessionReconciler) CreateJob(target formolv1alpha1.Target) error {
	log := r.Log.WithValues("createjob", target.Name)
	ctx := context.Background()

	output := corev1.VolumeMount{
		Name:      "output",
		MountPath: "/output",
	}
	restic := corev1.Container{
		Name:         "restic",
		Image:        "desmo999r/formolcli:latest",
		Args:         []string{"backup", "volume", "--tag", r.BackupSession.Name, "--path", "/output"},
		VolumeMounts: []corev1.VolumeMount{output},
		Env:          []corev1.EnvVar{},
	}
	log.V(1).Info("creating a tagget backup job", "container", restic)
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
		function.Spec.Env = step.Env
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
	// container that will delete the restic snapshot(s) matching the backupsession
	restic := corev1.Container{
		Name:  "restic",
		Image: "desmo999r/formolcli:latest",
		Args:  []string{"delete", "snapshot", "--tag", r.BackupSession.Name},
		Env:   []corev1.EnvVar{},
	}
	// Gather information from the repo
	repo := &formolv1alpha1.Repo{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: r.BackupConf.Namespace,
		Name:      r.BackupConf.Spec.Repository.Name,
	}, repo); err != nil {
		log.Error(err, "unable to get Repo from BackupConfiguration")
		return err
	}
	restic.Env = append(restic.Env, formolutils.ConfigureResticEnvVar(r.BackupConf, repo)...)
	// create a job to delete the restic snapshot(s) with the backupsession name tag
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("delete-%s-", r.BackupSession.Name),
			Namespace:    r.BackupSession.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
					Containers:     []corev1.Container{restic},
					RestartPolicy:  corev1.RestartPolicyOnFailure,
				},
			},
		},
	}
	log.V(0).Info("creating a job to delete restic snapshots")
	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "unable to create job", "job", job)
		return err
	}

	return nil
}

func (r *BackupSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &formolv1alpha1.BackupSession{}, sessionState, func(rawObj runtime.Object) []string {
		session := rawObj.(*formolv1alpha1.BackupSession)
		return []string{string(session.Status.BackupState)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&formolv1alpha1.BackupSession{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}). // Don't reconcile when status gets updated
		Owns(&batchv1.Job{}).
		Complete(r)
}
