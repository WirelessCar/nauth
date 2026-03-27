package controller

const (
	controllerTypeReady = "Ready"

	controllerAccountFinalizer = "account.nauth.io/finalizer"
	controllerUserFinalizer    = "user.nauth.io/finalizer"

	controllerReasonReconciling = "Reconciling"
	controllerReasonReconciled  = "Reconciled"
	controllerActionReconciled  = "Reconciled"
	controllerReasonErrored     = "Errored"
)

const (
	EnvOperatorVersion = "OPERATOR_VERSION"
)

const (
	LabelAccountNatsClusterRef = "account.nauth.io/nats-cluster-ref"
	LabelValueUnknown          = "UNKNOWN"
)
