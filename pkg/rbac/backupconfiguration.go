package rbac

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	backupListenerRole                    = "backup-listener-role"
	backupListenerRoleBinding             = "backup-listener-rolebinding"
	backupSessionCreatorSA                = "backupsession-creator"
	backupSessionCreatorRole              = "backupsession-creator-role"
	backupSessionCreatorRoleBinding       = "backupsession-creator-rolebinding"
	backupSessionStatusUpdaterRole        = "backupsession-statusupdater-role"
	backupSessionStatusUpdaterRoleBinding = "backupsession-statusupdater-rolebinding"
)

func DeleteBackupSessionCreatorRBAC(cl client.Client, namespace string) error {
	serviceaccount := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionCreatorSA,
	}, serviceaccount); err == nil {
		if err = cl.Delete(context.Background(), serviceaccount); err != nil {
			return err
		}
	}
	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionCreatorRole,
	}, role); err == nil {
		if err = cl.Delete(context.Background(), role); err != nil {
			return err
		}
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionCreatorRoleBinding,
	}, rolebinding); err == nil {
		if err = cl.Delete(context.Background(), rolebinding); err != nil {
			return err
		}
	}

	return nil
}

func CreateBackupSessionCreatorRBAC(cl client.Client, namespace string) error {
	serviceaccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupSessionCreatorSA,
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionCreatorSA,
	}, serviceaccount); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), serviceaccount); err != nil {
			return err
		}
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupSessionCreatorRole,
		},
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupsessions"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupsessions/status"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupconfigurations"},
			},
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionCreatorRole,
	}, role); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), role); err != nil {
			return err
		}
	}
	rolebinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupSessionCreatorRoleBinding,
		},
		Subjects: []rbacv1.Subject{
			rbacv1.Subject{
				Kind: "ServiceAccount",
				Name: backupSessionCreatorSA,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     backupSessionCreatorRole,
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionCreatorRoleBinding,
	}, rolebinding); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), rolebinding); err != nil {
			return err
		}
	}

	return nil
}

func DeleteBackupSessionListenerRBAC(cl client.Client, saName string, namespace string) error {
	if saName == "" {
		saName = "default"
	}
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      saName,
	}, sa); err != nil {
		return err
	}

	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupListenerRole,
	}, role); err == nil {
		if err = cl.Delete(context.Background(), role); err != nil {
			return err
		}
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupListenerRoleBinding,
	}, rolebinding); err == nil {
		if err = cl.Delete(context.Background(), rolebinding); err != nil {
			return err
		}
	}

	return nil
}

func CreateBackupSessionListenerRBAC(cl client.Client, saName string, namespace string) error {
	if saName == "" {
		saName = "default"
	}
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      saName,
	}, sa); err != nil {
		return err
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupListenerRole,
		},
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "replicasets"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupsessions", "backupconfigurations"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"update", "delete"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupsessions"},
			},
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupListenerRole,
	}, role); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), role); err != nil {
			return err
		}
	}
	rolebinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupListenerRoleBinding,
		},
		Subjects: []rbacv1.Subject{
			rbacv1.Subject{
				Kind: "ServiceAccount",
				Name: saName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     backupListenerRole,
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupListenerRoleBinding,
	}, rolebinding); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), rolebinding); err != nil {
			return err
		}
	}
	return nil
}

func DeleteBackupSessionStatusUpdaterRBAC(cl client.Client, saName string, namespace string) error {
	if saName == "" {
		saName = "default"
	}
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      saName,
	}, sa); err != nil {
		return err
	}

	role := &rbacv1.Role{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionStatusUpdaterRole,
	}, role); err == nil {
		if err = cl.Delete(context.Background(), role); err != nil {
			return err
		}
	}

	rolebinding := &rbacv1.RoleBinding{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionStatusUpdaterRoleBinding,
	}, rolebinding); err == nil {
		if err = cl.Delete(context.Background(), rolebinding); err != nil {
			return err
		}
	}

	return nil
}

func CreateBackupSessionStatusUpdaterRBAC(cl client.Client, saName string, namespace string) error {
	if saName == "" {
		saName = "default"
	}
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      saName,
	}, sa); err != nil {
		return err
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupSessionStatusUpdaterRole,
		},
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch", "patch", "update"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupsessions/status"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{"formol.desmojim.fr"},
				Resources: []string{"backupsessions"},
			},
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionStatusUpdaterRole,
	}, role); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), role); err != nil {
			return err
		}
	}
	rolebinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backupSessionStatusUpdaterRoleBinding,
		},
		Subjects: []rbacv1.Subject{
			rbacv1.Subject{
				Kind: "ServiceAccount",
				Name: saName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     backupSessionStatusUpdaterRole,
		},
	}
	if err := cl.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      backupSessionStatusUpdaterRoleBinding,
	}, rolebinding); err != nil && errors.IsNotFound(err) {
		if err = cl.Create(context.Background(), rolebinding); err != nil {
			return err
		}
	}
	return nil
}
