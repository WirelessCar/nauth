package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileAccountClaimsMismatchLogging(t *testing.T) {
	matching := &domain.NatsAccountState{
		Status: domain.NatsAccountStateComplete, ServerID: "server-a", ClaimsHash: "desired-hash",
	}
	mismatch := &domain.NatsAccountState{
		Status: domain.NatsAccountStateComplete, ServerID: "server-a", ClaimsHash: "observed-hash",
	}
	otherServer := &domain.NatsAccountState{
		Status: domain.NatsAccountStateComplete, ServerID: "server-b", ClaimsHash: "observed-hash",
	}
	changedMismatch := &domain.NatsAccountState{
		Status: domain.NatsAccountStateComplete, ServerID: "server-a", ClaimsHash: "changed-observed-hash",
	}
	incomplete := &domain.NatsAccountState{
		Status: domain.NatsAccountStateIncomplete, ServerID: "server-a", ClaimsHash: "desired-hash",
	}
	unknown := &domain.NatsAccountState{Status: domain.NatsAccountStateUnknown}
	tests := []struct {
		name             string
		observations     []*domain.NatsAccountState
		desiredHashes    []string
		wantLogs         int
		failStatusWrites int
		failLabelWrites  int
		bootstrap        bool
	}{
		{name: "first_mismatch", observations: []*domain.NatsAccountState{mismatch}, wantLogs: 1},
		{name: "mismatch_after_matching_claims", observations: []*domain.NatsAccountState{matching, mismatch}, wantLogs: 1},
		{name: "unchanged_mismatch_across_servers_and_skipped_validation", observations: []*domain.NatsAccountState{mismatch, otherServer, nil, mismatch}, wantLogs: 1},
		{name: "changed_observed_hash", observations: []*domain.NatsAccountState{mismatch, changedMismatch}, wantLogs: 2},
		{
			name: "changed_desired_hash", observations: []*domain.NatsAccountState{mismatch, mismatch},
			desiredHashes: []string{"desired-hash", "changed-desired-hash"}, wantLogs: 2,
		},
		{name: "mismatch_recurs_after_recovery", observations: []*domain.NatsAccountState{mismatch, matching, mismatch}, wantLogs: 2},
		{name: "mismatch_after_unknown_observation", observations: []*domain.NatsAccountState{mismatch, unknown, mismatch}, wantLogs: 2},
		{
			name: "status_conflict_followed_by_recovery", observations: []*domain.NatsAccountState{mismatch, matching},
			failStatusWrites: 1, wantLogs: 1,
		},
		{
			name: "label_conflict_repeats_until_persisted_then_recovers", observations: []*domain.NatsAccountState{mismatch, mismatch, mismatch, matching},
			failLabelWrites: 1, wantLogs: 2,
		},
		{
			name: "bootstrap_mismatch_followed_by_recovery", observations: []*domain.NatsAccountState{mismatch, matching},
			bootstrap: true, wantLogs: 1,
		},
		{name: "matching_claims", observations: []*domain.NatsAccountState{matching}},
		{name: "incomplete_account_with_matching_claims", observations: []*domain.NatsAccountState{incomplete}},
		{name: "unknown_observation", observations: []*domain.NatsAccountState{unknown}},
		{name: "skipped_validation", observations: []*domain.NatsAccountState{nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAccountLoggingFixture(t, len(tt.observations), accountLoggingOptions{bootstrap: tt.bootstrap, failStatusWrites: tt.failStatusWrites, failLabelWrites: tt.failLabelWrites})

			for i, state := range tt.observations {
				desiredHash := "desired-hash"
				if tt.desiredHashes != nil {
					desiredHash = tt.desiredHashes[i]
				}
				result := fixture.result(state, desiredHash)
				err := fixture.reconcile(result)
				if i < tt.failStatusWrites+tt.failLabelWrites {
					require.ErrorIs(t, err, fixture.writeErr)
				} else {
					require.NoError(t, err)
				}
			}

			entries := fixture.logs.FilterMessage("Observed NATS Account claims do not match the desired claims").All()
			require.Len(t, entries, tt.wantLogs)
			for _, entry := range entries {
				require.Equal(t, zapcore.InfoLevel, entry.Level)
				fields := entry.ContextMap()
				require.Equal(t, fixture.account.Name, fields["name"])
				require.Equal(t, fixture.account.Namespace, fields["namespace"])
				require.Equal(t, fixture.accountID, fields["accountID"])
				require.Equal(t, fixture.clusterUID, fields["natsClusterUID"])
				require.Equal(t, "server-a", fields["observedServerID"])
				require.NotEmpty(t, fields["desiredClaimsHash"])
				require.NotEmpty(t, fields["observedClaimsHash"])
				require.NotEqual(t, fields["desiredClaimsHash"], fields["observedClaimsHash"])
			}
		})
	}
}

func TestReconcileAccountIncompleteLogging(t *testing.T) {
	complete := &domain.NatsAccountState{
		Status: domain.NatsAccountStateComplete, ServerID: "server-a", ClaimsHash: "desired-hash",
	}
	incomplete := &domain.NatsAccountState{
		Status: domain.NatsAccountStateIncomplete, ServerID: "server-a", ClaimsHash: "desired-hash",
	}
	invalid := &domain.NatsAccountState{
		Status: domain.NatsAccountStateIncomplete, ServerID: "server-a", ClaimsHash: "desired-hash",
		Imports: []domain.NatsAccountImport{{AccountID: "export-account", Subject: "orders.>", Type: "stream", Invalid: true}},
	}
	otherServer := *invalid
	otherServer.ServerID = "server-b"
	loadedWithInvalidImports := *invalid
	loadedWithInvalidImports.Status = domain.NatsAccountStateComplete
	changedInvalid := &domain.NatsAccountState{
		Status: domain.NatsAccountStateIncomplete, ServerID: "server-a", ClaimsHash: "desired-hash",
		Imports: []domain.NatsAccountImport{{AccountID: "export-account", Subject: "stock.>", Type: "stream", Invalid: true}},
	}
	mismatch := *invalid
	mismatch.ClaimsHash = "other-claims-hash"
	unknown := &domain.NatsAccountState{Status: domain.NatsAccountStateUnknown}
	twoInvalid := *invalid
	twoInvalid.Imports = append(append([]domain.NatsAccountImport{}, invalid.Imports...), changedInvalid.Imports...)
	reordered := twoInvalid
	reordered.Imports = []domain.NatsAccountImport{twoInvalid.Imports[1], twoInvalid.Imports[0]}
	const incompleteReason = "NATS reports the Account as incomplete"
	const invalidReason = "NATS reports invalid imports: export-account -> orders.> (stream)"
	const changedInvalidReason = "NATS reports invalid imports: export-account -> stock.> (stream)"
	tests := []struct {
		name             string
		observations     []*domain.NatsAccountState
		imports          nauth.Imports
		wantReasons      []string
		failStatusWrites int
		failLabelWrites  int
		bootstrap        bool
	}{
		{name: "first_incomplete_observation", observations: []*domain.NatsAccountState{incomplete}, wantReasons: []string{incompleteReason}},
		{name: "first_invalid_import", observations: []*domain.NatsAccountState{invalid}, wantReasons: []string{invalidReason}},
		{name: "loaded_account_with_invalid_import", observations: []*domain.NatsAccountState{&loadedWithInvalidImports}, wantReasons: []string{invalidReason}},
		{name: "unchanged_cause_across_servers_and_skipped_validation", observations: []*domain.NatsAccountState{invalid, &otherServer, nil, invalid}, wantReasons: []string{invalidReason}},
		{name: "changed_invalid_import", observations: []*domain.NatsAccountState{invalid, changedInvalid}, wantReasons: []string{invalidReason, changedInvalidReason}},
		{name: "changed_incompleteness_cause", observations: []*domain.NatsAccountState{incomplete, invalid}, wantReasons: []string{incompleteReason, invalidReason}},
		{name: "recurrence_after_recovery", observations: []*domain.NatsAccountState{invalid, complete, invalid}, wantReasons: []string{invalidReason, invalidReason}},
		{name: "incomplete_after_unknown_observation", observations: []*domain.NatsAccountState{invalid, unknown, invalid}, wantReasons: []string{invalidReason, invalidReason}},
		{name: "incomplete_after_claims_converge", observations: []*domain.NatsAccountState{&mismatch, invalid}, wantReasons: []string{invalidReason}},
		{
			name: "status_conflict_followed_by_recovery", observations: []*domain.NatsAccountState{invalid, complete},
			failStatusWrites: 1, wantReasons: []string{invalidReason},
		},
		{
			name: "label_conflict_followed_by_recovery", observations: []*domain.NatsAccountState{invalid, complete},
			failLabelWrites: 1, wantReasons: []string{invalidReason},
		},
		{
			name: "failed_status_writes_repeat_until_persistence_succeeds", observations: []*domain.NatsAccountState{invalid, invalid, invalid, invalid},
			failStatusWrites: 2, wantReasons: []string{invalidReason, invalidReason, invalidReason},
		},
		{
			name: "bootstrap_incomplete_followed_by_recovery", observations: []*domain.NatsAccountState{incomplete, complete},
			bootstrap: true, wantReasons: []string{incompleteReason},
		},
		{
			name: "desired_import_missing_from_runtime", observations: []*domain.NatsAccountState{incomplete},
			imports:     nauth.Imports{{AccountID: "export-account", Subject: "orders.>", Type: nauth.ExportTypeStream}},
			wantReasons: []string{"NATS did not report desired imports: export-account -> orders.> (stream)"},
		},
		{
			name: "unchanged_imports_returned_in_different_order", observations: []*domain.NatsAccountState{&twoInvalid, &reordered},
			wantReasons: []string{"NATS reports invalid imports: export-account -> orders.> (stream); export-account -> stock.> (stream)"},
		},
		{name: "complete_account", observations: []*domain.NatsAccountState{complete}},
		{name: "unknown_observation", observations: []*domain.NatsAccountState{unknown}},
		{name: "skipped_validation", observations: []*domain.NatsAccountState{nil}},
		{name: "invalid_imports_from_mismatching_claims", observations: []*domain.NatsAccountState{&mismatch}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAccountLoggingFixture(t, len(tt.observations), accountLoggingOptions{bootstrap: tt.bootstrap, failStatusWrites: tt.failStatusWrites, failLabelWrites: tt.failLabelWrites})

			for i, state := range tt.observations {
				result := fixture.result(state, "desired-hash")
				result.Claims = &nauth.AccountClaims{Imports: tt.imports}
				err := fixture.reconcile(result)
				if i < tt.failStatusWrites+tt.failLabelWrites {
					require.ErrorIs(t, err, fixture.writeErr)
				} else {
					require.NoError(t, err)
				}
			}

			entries := fixture.logs.FilterMessage("NATS Account is incomplete").All()
			require.Len(t, entries, len(tt.wantReasons))
			for i, entry := range entries {
				require.Equal(t, zapcore.InfoLevel, entry.Level)
				fields := entry.ContextMap()
				require.Equal(t, fixture.account.Name, fields["name"])
				require.Equal(t, fixture.account.Namespace, fields["namespace"])
				require.Equal(t, fixture.accountID, fields["accountID"])
				require.Equal(t, fixture.clusterUID, fields["natsClusterUID"])
				require.Equal(t, "server-a", fields["observedServerID"])
				require.Equal(t, "desired-hash", fields["desiredClaimsHash"])
				require.Equal(t, "desired-hash", fields["observedClaimsHash"])
				require.Equal(t, tt.wantReasons[i], fields["reason"])
			}
		})
	}
}

func TestReconcileAccountUnknownLogging(t *testing.T) {
	complete := &domain.NatsAccountState{
		Status: domain.NatsAccountStateComplete, ServerID: "server-a", ClaimsHash: "desired-hash",
	}
	incomplete := &domain.NatsAccountState{
		Status: domain.NatsAccountStateIncomplete, ServerID: "server-a", ClaimsHash: "desired-hash",
	}
	mismatch := *complete
	mismatch.ClaimsHash = "other-claims-hash"
	unknown := &domain.NatsAccountState{Status: domain.NatsAccountStateUnknown}
	otherServer := &domain.NatsAccountState{Status: domain.NatsAccountStateUnknown, ServerID: "server-b"}
	const lookupReason = "failed to lookup Account state: nats: timeout"
	const loadReason = "failed to request runtime Account load: nats: timeout"
	const fallbackReason = "NATS Account completeness is Unknown"
	type observation struct {
		state   *domain.NatsAccountState
		message string
	}
	tests := []struct {
		name             string
		observations     []observation
		wantReasons      []string
		failStatusWrites int
		failLabelWrites  int
		bootstrap        bool
		observe          bool
	}{
		{name: "first_unknown_observation", observations: []observation{{state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason}},
		{name: "unknown_after_complete", observations: []observation{{state: complete}, {state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason}},
		{name: "unknown_after_incomplete", observations: []observation{{state: incomplete}, {state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason}},
		{name: "unknown_after_claims_mismatch", observations: []observation{{state: &mismatch}, {state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason}},
		{name: "unchanged_reason_across_skipped_validation", observations: []observation{{state: unknown, message: lookupReason}, {}, {state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason}},
		{name: "unchanged_reason_across_servers", observations: []observation{{state: unknown, message: lookupReason}, {state: otherServer, message: lookupReason}}, wantReasons: []string{lookupReason}},
		{name: "changed_observation_reason", observations: []observation{{state: unknown, message: lookupReason}, {state: unknown, message: loadReason}}, wantReasons: []string{lookupReason, loadReason}},
		{name: "recurrence_after_recovery", observations: []observation{{state: unknown, message: lookupReason}, {state: complete}, {state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason, lookupReason}},
		{name: "recurrence_after_incomplete", observations: []observation{{state: unknown, message: lookupReason}, {state: incomplete}, {state: unknown, message: lookupReason}}, wantReasons: []string{lookupReason, lookupReason}},
		{name: "empty_reason_uses_condition_fallback", observations: []observation{{state: unknown}, {state: unknown}}, wantReasons: []string{fallbackReason}},
		{name: "diagnostic_after_fallback", observations: []observation{{state: unknown}, {state: unknown, message: lookupReason}}, wantReasons: []string{fallbackReason, lookupReason}},
		{
			name: "status_conflict_followed_by_recovery", observations: []observation{{state: unknown, message: lookupReason}, {state: complete}},
			failStatusWrites: 1, wantReasons: []string{lookupReason},
		},
		{
			name: "label_conflict_followed_by_recovery", observations: []observation{{state: unknown, message: lookupReason}, {state: complete}},
			failLabelWrites: 1, wantReasons: []string{lookupReason},
		},
		{
			name: "failed_status_writes_repeat_until_persistence_succeeds", observations: []observation{{state: unknown, message: lookupReason}, {state: unknown, message: lookupReason}, {state: unknown, message: lookupReason}, {state: unknown, message: lookupReason}},
			failStatusWrites: 2, wantReasons: []string{lookupReason, lookupReason, lookupReason},
		},
		{
			name: "bootstrap_unknown_followed_by_recovery", observations: []observation{{state: unknown, message: lookupReason}, {state: complete}},
			bootstrap: true, wantReasons: []string{lookupReason},
		},
		{
			name: "bootstrap_unknown_repeats_until_status_is_persisted", observations: []observation{{state: unknown, message: lookupReason}, {state: unknown, message: lookupReason}, {state: unknown, message: lookupReason}},
			bootstrap: true, wantReasons: []string{lookupReason, lookupReason},
		},
		{
			name: "observe_unknown_suppresses_unchanged_reason", observations: []observation{{state: unknown, message: lookupReason}, {state: unknown, message: lookupReason}},
			observe: true, wantReasons: []string{lookupReason},
		},
		{name: "known_observations", observations: []observation{{state: complete}, {state: incomplete}, {state: &mismatch}}},
		{name: "skipped_validation", observations: []observation{{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAccountLoggingFixture(t, len(tt.observations), accountLoggingOptions{bootstrap: tt.bootstrap, observe: tt.observe, failStatusWrites: tt.failStatusWrites, failLabelWrites: tt.failLabelWrites})

			for i, observation := range tt.observations {
				state := observation.state
				result := fixture.result(state, "desired-hash")
				result.NatsObservationMessage = observation.message
				err := fixture.reconcile(result)
				if i < tt.failStatusWrites+tt.failLabelWrites {
					require.ErrorIs(t, err, fixture.writeErr)
				} else {
					require.NoError(t, err)
				}
			}

			entries := fixture.logs.FilterMessage("NATS Account state observation is Unknown").All()
			require.Len(t, entries, len(tt.wantReasons))
			for i, entry := range entries {
				require.Equal(t, zapcore.InfoLevel, entry.Level)
				fields := entry.ContextMap()
				require.Equal(t, fixture.account.Name, fields["name"])
				require.Equal(t, fixture.account.Namespace, fields["namespace"])
				require.Equal(t, fixture.accountID, fields["accountID"])
				require.Equal(t, fixture.clusterUID, fields["natsClusterUID"])
				require.Equal(t, "desired-hash", fields["desiredClaimsHash"])
				require.Equal(t, tt.wantReasons[i], fields["reason"])
				require.NotContains(t, fields, "observedServerID")
				require.NotContains(t, fields, "observedClaimsHash")
			}
		})
	}
}

func TestReconcileAccountReadinessRecoveryLogging(t *testing.T) {
	complete := &domain.NatsAccountState{Status: domain.NatsAccountStateComplete, ServerID: "server-a", ClaimsHash: "desired-hash"}
	incomplete := &domain.NatsAccountState{Status: domain.NatsAccountStateIncomplete, ServerID: "server-a", ClaimsHash: "desired-hash"}
	unknown := &domain.NatsAccountState{Status: domain.NatsAccountStateUnknown}
	validatedAt := metav1.NewTime(time.Date(2026, time.October, 5, 7, 0, 0, 0, time.UTC))
	tests := []struct {
		name             string
		observations     []*domain.NatsAccountState
		initialNotReady  bool
		cachedValidation bool
		failStatusWrites int
		wantLogCounts    []int
		wantPrevious     []metav1.ConditionStatus
	}{
		{
			name: "recovery_recurrence_and_steady_ready", initialNotReady: true,
			observations:  []*domain.NatsAccountState{complete, incomplete, complete, unknown, complete, complete},
			wantLogCounts: []int{1, 1, 2, 2, 3, 3}, wantPrevious: []metav1.ConditionStatus{metav1.ConditionFalse, metav1.ConditionFalse, metav1.ConditionUnknown},
		},
		{name: "initial_and_repeated_ready", observations: []*domain.NatsAccountState{complete, complete}, wantLogCounts: []int{0, 0}},
		{
			name: "status_conflict_then_successful_retry", initialNotReady: true, failStatusWrites: 1,
			observations: []*domain.NatsAccountState{complete, complete}, wantLogCounts: []int{0, 1}, wantPrevious: []metav1.ConditionStatus{metav1.ConditionFalse},
		},
		{
			name: "cached_validation_recovery", initialNotReady: true, cachedValidation: true,
			observations: []*domain.NatsAccountState{nil}, wantLogCounts: []int{1}, wantPrevious: []metav1.ConditionStatus{metav1.ConditionFalse},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := v1alpha1.AccountStatus{}
			if tt.initialNotReady {
				status.Conditions = []metav1.Condition{{Type: conditionTypeReady, Status: metav1.ConditionFalse, Reason: conditionReasonNotReady}}
			}
			wantServerID := "server-a"
			if tt.cachedValidation {
				wantServerID = "cached-server"
				status.Nats = &v1alpha1.AccountNatsStatus{ObservedServerID: wantServerID, ObservedClaimsHash: "desired-hash", StateValidatedAt: validatedAt}
				status.ClaimsHash = "desired-hash"
				status.Conditions = append(status.Conditions, metav1.Condition{Type: conditionTypeNatsAccountComplete, Status: metav1.ConditionTrue, Reason: conditionReasonOK})
			}
			fixture := newAccountLoggingFixture(t, len(tt.observations), accountLoggingOptions{initialStatus: status, failStatusWrites: tt.failStatusWrites})
			for i, state := range tt.observations {
				result := fixture.result(state, "desired-hash")
				if state != nil && state.Status != domain.NatsAccountStateUnknown {
					result.State.StateValidatedAt = validatedAt.Time
				}
				err := fixture.reconcile(result)
				if i < tt.failStatusWrites {
					require.ErrorIs(t, err, fixture.writeErr)
				} else {
					require.NoError(t, err)
				}
				require.Len(t, fixture.logs.FilterMessage("Account readiness has recovered").All(), tt.wantLogCounts[i])
			}

			for i, entry := range fixture.logs.FilterMessage("Account readiness has recovered").All() {
				require.Equal(t, zapcore.InfoLevel, entry.Level)
				fields := entry.ContextMap()
				require.Equal(t, fixture.account.Name, fields["name"])
				require.Equal(t, fixture.account.Namespace, fields["namespace"])
				require.Equal(t, fixture.accountID, fields["accountID"])
				require.Equal(t, fixture.clusterUID, fields["natsClusterUID"])
				require.Equal(t, "desired-hash", fields["desiredClaimsHash"])
				require.Equal(t, string(tt.wantPrevious[i]), fields["previousReadyStatus"])
				wantReason := conditionReasonNotReady
				if tt.wantPrevious[i] == metav1.ConditionUnknown {
					wantReason = conditionReasonUnknown
				}
				require.Equal(t, wantReason, fields["previousReadyReason"])
				require.Equal(t, wantServerID, fields["observedServerID"])
				require.Equal(t, "desired-hash", fields["observedClaimsHash"])
				require.IsType(t, time.Time{}, fields["stateValidatedAt"])
				require.WithinDuration(t, validatedAt.Time, fields["stateValidatedAt"].(time.Time), 0)
			}
		})
	}
}

type accountLoggingOptions struct {
	initialStatus    v1alpha1.AccountStatus
	bootstrap        bool
	observe          bool
	failStatusWrites int
	failLabelWrites  int
}

type accountLoggingFixture struct {
	ctx        context.Context
	account    *v1alpha1.Account
	accountID  string
	clusterUID string
	client     client.Client
	manager    *accountManagerMock
	reconciler *AccountReconciler
	logs       *observer.ObservedLogs
	writeErr   error
	observe    bool
}

func newAccountLoggingFixture(t *testing.T, reconciliations int, options accountLoggingOptions) *accountLoggingFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	accountID := testutil.NatsTestAccountA.Root.PublicKey
	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-account", Namespace: "test-namespace", Finalizers: []string{finalizerAccount},
			Labels: map[string]string{string(v1alpha1.AccountLabelAccountID): accountID},
		},
		Status: options.initialStatus,
	}
	if options.bootstrap {
		delete(account.Labels, string(v1alpha1.AccountLabelAccountID))
	}
	if options.observe {
		account.Labels[k8s.LabelManagementPolicy] = k8s.ManagementPolicyObserve
	}
	writeErr := apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("accounts").GroupResource(), account.Name, errors.New("concurrent update"))
	failedStatusWrites, failedLabelWrites := 0, 0
	kubernetes := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(account).WithObjects(account).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if subResource == "status" && failedStatusWrites < options.failStatusWrites {
					failedStatusWrites++
					return writeErr
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if failedLabelWrites < options.failLabelWrites {
					failedLabelWrites++
					return writeErr
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	manager := &accountManagerMock{}
	clusterManager := &clusterManagerMock{}
	clusterTarget := createDummyClusterTarget()
	clusterManager.On("GetClusterTarget", mock.Anything, mock.Anything).Return(clusterTarget, nil).Times(reconciliations)
	t.Cleanup(func() {
		manager.AssertExpectations(t)
		clusterManager.AssertExpectations(t)
	})
	logCore, logs := observer.New(zapcore.InfoLevel)
	return &accountLoggingFixture{
		ctx:     logf.IntoContext(context.Background(), zapr.NewLogger(zap.New(logCore))),
		account: account, accountID: accountID, clusterUID: clusterTarget.UID, client: kubernetes,
		manager: manager, logs: logs, writeErr: writeErr, observe: options.observe,
		reconciler: NewAccountReconciler(kubernetes, scheme, manager, clusterManager,
			k8s.NewAccountClient(kubernetes), events.NewFakeRecorder(1), testAccountReconciliationInterval, false),
	}
}

func (f *accountLoggingFixture) result(state *domain.NatsAccountState, desiredHash string) *nauth.AccountResult {
	result := &nauth.AccountResult{
		AccountID: f.accountID, AccountSignedBy: "operator-signing-key",
		State: nauth.AccountState{ClaimsHash: desiredHash}, NatsState: state, ValidationOutcome: nauth.AccountValidationReady,
	}
	if state != nil {
		result.State.ObservedStatus = state.Status
		result.ValidationOutcome = nauth.AccountValidationUnknown
		if state.Status != domain.NatsAccountStateUnknown {
			result.State.ObservedServerID = state.ServerID
			result.State.ObservedClaimsHash = state.ClaimsHash
			result.State.StateValidatedAt = time.Now()
			result.ValidationOutcome = nauth.AccountValidationPending
			if state.Status == domain.NatsAccountStateComplete && !state.HasInvalidImports() && state.MatchesClaimsHash(desiredHash) {
				result.ValidationOutcome = nauth.AccountValidationReady
			}
		}
	}
	return result
}

func (f *accountLoggingFixture) reconcile(result *nauth.AccountResult) error {
	if f.observe {
		f.manager.mockImport(f.ctx, mock.Anything, result).Once()
	} else {
		f.manager.mockCreateOrUpdate(f.ctx, mock.Anything, result).Once()
	}
	_, err := f.reconciler.Reconcile(f.ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(f.account)})
	return err
}
