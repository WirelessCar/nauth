package controller

const ( // Conditions
	// Types
	conditionTypeReady                = "Ready"
	conditionTypeBoundToAccount       = "BoundToAccount"
	conditionTypeBoundToExportAccount = "BoundToExportAccount"
	conditionTypeValidRules           = "ValidRules"
	conditionTypeAdoptedByAccount     = "AdoptedByAccount"

	// Reasons
	conditionReasonReconciling = "Reconciling"
	conditionReasonReconciled  = "Reconciled"
	conditionReasonOK          = "OK"
	conditionReasonErrored     = "Errored"
	conditionReasonInvalid     = "Invalid"
	conditionReasonConflict    = "Conflict"
	conditionReasonBinding     = "Binding"
	conditionReasonNotFound    = "NotFound"
	conditionReasonAdopting    = "Adopting"
	conditionReasonFailed      = "Failed"
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
