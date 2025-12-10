package controller

const (
	controllerTypeReady = "Ready"

	controllerAccountFinalizer = "account.nauth.io/finalizer"
	controllerUserFinalizer    = "user.nauth.io/finalizer"

	//ControllerReasonNoAccountFound = "NoAccountFound"
	controllerReasonReconciling = "Reconciling"
	controllerReasonReconciled  = "Reconciled"
	controllerReasonErrored     = "Errored"
)

const (
	operatorVersion = "OPERATOR_VERSION"
)
