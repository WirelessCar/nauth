package controller

const (
	controllerTypeReady = "Ready"

	controllerAccountFinalizer = "account.nauth.io/finalizer"
	controllerUserFinalizer    = "user.nauth.io/finalizer"

	controllerReasonReconciling = "Reconciling"
	controllerReasonReconciled  = "Reconciled"
	controllerActionReconciled  = "Reconciled"
	controllerReasonOK          = "OK"
	controllerReasonErrored     = "Errored"
	controllerReasonInvalid     = "Invalid"
)

const (
	EnvOperatorVersion = "OPERATOR_VERSION"
)
