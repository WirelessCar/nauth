package controller

const (
	ControllerTypeReady = "Ready"

	ControllerAccountFinalizer = "account.nauth.io/finalizer"
	ControllerUserFinalizer    = "user.nauth.io/finalizer"

	ControllerReasonNoAccountFound = "NoAccountFound"
	ControllerReasonReconciling    = "Reconciling"
	ControllerReasonReconciled     = "Reconciled"
	ControllerReasonErrored        = "Errored"
)

const (
	OperatorVersion = "OPERATOR_VERSION"
)
