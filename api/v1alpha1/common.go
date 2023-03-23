package v1alpha1

const (
	RESTORECONTAINER_NAME string = "formol-restore"
	// the name of the sidecar container
	SIDECARCONTAINER_NAME string = "formol"
	// the name of the container we backup when there are more than 1 container in the pod
	TARGETCONTAINER_TAG string = "FORMOL_TARGET"
	// Used by both the backupsession and restoresession controllers to identified the target deployment
	TARGET_NAME string = "TARGET_NAME"
	// Used by the backupsession controller
	POD_NAME      string = "POD_NAME"
	POD_NAMESPACE string = "POD_NAMESPACE"
	// Backup Paths list
	BACKUP_PATHS = "BACKUP_PATHS"
)
