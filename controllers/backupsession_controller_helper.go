package controllers

import (
	formolv1alpha1 "github.com/desmo999r/formol/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
	"strings"
	"time"
)

const (
	JOBTTL int32 = 7200
)

func (r *BackupSessionReconciler) cleanupSessions(backupConf formolv1alpha1.BackupConfiguration) {
	backupSessionList := formolv1alpha1.BackupSessionList{}
	if err := r.List(r.Context, &backupSessionList, client.InNamespace(backupConf.Namespace), client.MatchingFieldsSelector{Selector: fields.SelectorFromSet(fields.Set{sessionState: string(formolv1alpha1.Success)})}); err != nil {
		r.Log.Error(err, "unable to get backupsessionlist")
		return
	}
	if len(backupSessionList.Items) < 2 {
		// Not enough backupSession to proceed
		r.Log.V(1).Info("Not enough successful backup jobs")
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
			r.Log.V(1).Info("Keep backup", "last", session.Status.StartTime)
			lastBackups.Counter--
			keep = append(keep, "last")
			deleteSession = false
		}
		if dailyBackups.Counter > 0 {
			if session.Status.StartTime.Time.YearDay() != dailyBackups.Last.YearDay() {
				r.Log.V(1).Info("Keep backup", "daily", session.Status.StartTime)
				dailyBackups.Counter--
				dailyBackups.Last = session.Status.StartTime.Time
				keep = append(keep, "daily")
				deleteSession = false
			}
		}
		if weeklyBackups.Counter > 0 {
			if session.Status.StartTime.Time.Weekday().String() == "Sunday" && session.Status.StartTime.Time.YearDay() != weeklyBackups.Last.YearDay() {
				r.Log.V(1).Info("Keep backup", "weekly", session.Status.StartTime)
				weeklyBackups.Counter--
				weeklyBackups.Last = session.Status.StartTime.Time
				keep = append(keep, "weekly")
				deleteSession = false
			}
		}
		if monthlyBackups.Counter > 0 {
			if session.Status.StartTime.Time.Day() == 1 && session.Status.StartTime.Time.Month() != monthlyBackups.Last.Month() {
				r.Log.V(1).Info("Keep backup", "monthly", session.Status.StartTime)
				monthlyBackups.Counter--
				monthlyBackups.Last = session.Status.StartTime.Time
				keep = append(keep, "monthly")
				deleteSession = false
			}
		}
		if yearlyBackups.Counter > 0 {
			if session.Status.StartTime.Time.YearDay() == 1 && session.Status.StartTime.Time.Year() != yearlyBackups.Last.Year() {
				r.Log.V(1).Info("Keep backup", "yearly", session.Status.StartTime)
				yearlyBackups.Counter--
				yearlyBackups.Last = session.Status.StartTime.Time
				keep = append(keep, "yearly")
				deleteSession = false
			}
		}
		if deleteSession {
			r.Log.V(1).Info("Delete session", "delete", session.Status.StartTime)
			if err := r.Delete(r.Context, &session); err != nil {
				r.Log.Error(err, "unable to delete backupsession", "session", session.Name)
				// we don't return anything, we keep going
			}
		} else {
			session.Status.Keep = strings.Join(keep, ",") // + " " + time.Now().Format("2006 Jan 02 15:04:05 -0700 MST")
			if err := r.Status().Update(r.Context, &session); err != nil {
				r.Log.Error(err, "unable to update session status", "session", session)
			}
		}
	}
}

func (r *BackupSessionReconciler) deleteSnapshots(backupSession formolv1alpha1.BackupSession, backupConf formolv1alpha1.BackupConfiguration) error {
	snapshots := []corev1.Container{}
	for _, target := range backupSession.Status.Targets {
		if target.SnapshotId != "" {
			snapshots = append(snapshots, corev1.Container{
				Name:  target.TargetName,
				Image: backupConf.Spec.Image,
				Args:  []string{"snapshot", "delete", "--namespace", backupConf.Namespace, "--name", backupConf.Name, "--snapshot-id", target.SnapshotId},
			})
		}
	}
	if len(snapshots) > 0 {
		job := batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "delete-" + backupSession.Name + "-",
				Namespace:    backupSession.Namespace,
			},
			Spec: batchv1.JobSpec{
				TTLSecondsAfterFinished: func() *int32 { ttl := JOBTTL; return &ttl }(),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						// The snapshot deletions have to be sequential
						// otherwise the repository will be locked by restic and it won't work.
						InitContainers: snapshots[1:],
						Containers:     []corev1.Container{snapshots[0]},
						RestartPolicy:  corev1.RestartPolicyOnFailure,
					},
				},
			},
		}
		r.Log.V(0).Info("creating a job to delete the BackupSession restic snapshots", "backupSession", backupSession)
		if err := r.Create(r.Context, &job); err != nil {
			r.Log.Error(err, "unable to create the job")
			return err
		}
	}
	return nil
}
