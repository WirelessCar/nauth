/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	finalizerNatsCluster = "natscluster.nauth.io/finalizer"
	finalizerAccount     = "account.nauth.io/finalizer"
	finalizerUser        = "user.nauth.io/finalizer"
)

const ( // Environment Variables
	envOperatorVersion = "OPERATOR_VERSION"
)

const ( // "requeue after" durations
	// Allow some time to avoid reading stale data
	requeueImmediately = time.Millisecond * 250

	// Poll interval when a required dependency exists but is not yet ready.
	requeueDependencyNotReady = 5 * time.Second
)
