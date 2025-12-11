package controller

const (
	controllerTypeReady = "Ready"

	controllerAccountFinalizer = "account.nauth.io/finalizer"
	controllerUserFinalizer    = "user.nauth.io/finalizer"

	controllerReasonReconciling = "Reconciling"
	controllerReasonReconciled  = "Reconciled"
	controllerReasonErrored     = "Errored"
)

const (
	EnvOperatorVersion = "OPERATOR_VERSION"
)
