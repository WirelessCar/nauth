package controller

const ( // Conditions
	// Types
	conditionTypeReady            = "Ready"
	conditionTypeBoundToAccount   = "BoundToAccount"
	conditionTypeValidRules       = "ValidRules"
	conditionTypeAdoptedByAccount = "AdoptedByAccount"

	// Reasons
	conditionReasonReady       = "Ready"
	conditionReasonNotReady    = "NotReady"
	conditionReasonReconciling = "Reconciling"
	conditionReasonReconciled  = "Reconciled"
	conditionReasonOK          = "OK"
	conditionReasonErrored     = "Errored"
	conditionReasonInvalid     = "Invalid"
	conditionReasonConflict    = "Conflict"
)

const ( // Events
	// Actions
	actionReconciled = "Reconciled"
)

const ( // Finalizers
	finalizerAccount = "account.nauth.io/finalizer"
	finalizerUser    = "user.nauth.io/finalizer"
)

const ( // Environment Variables
	envOperatorVersion = "OPERATOR_VERSION"
)
