package controller

import "time"

const ( // Conditions
	// Types
	conditionTypeReady                = "Ready"
	conditionTypeBoundToAccount       = "BoundToAccount"
	conditionTypeBoundToExportAccount = "BoundToExportAccount"
	conditionTypeValidRules           = "ValidRules"
	conditionTypeAdoptedByAccount     = "AdoptedByAccount"

	// Reasons
	conditionReasonReady       = "Ready"
	conditionReasonNotReady    = "NotReady"
	conditionReasonReconciling = "Reconciling"
	conditionReasonReconciled  = "Reconciled"
	conditionReasonOK          = "OK"
	conditionReasonNOK         = "NOK"
	conditionReasonErrored     = "Errored"
	conditionReasonInvalid     = "Invalid"
	conditionReasonConflict    = "Conflict"
	conditionReasonBinding     = "Binding"
	conditionReasonNotFound    = "NotFound"
	conditionReasonAdopting    = "Adopting"
	conditionReasonFailed      = "Failed"

	// Messages
	conditionMessageAdopted = "Adopted"
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

const ( // "requeue after" durations
	// Allow some time to avoid reading stale data
	requeueImmediately = time.Millisecond * 250
)
