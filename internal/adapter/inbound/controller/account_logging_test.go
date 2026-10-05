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
		name          string
		observations  []*domain.NatsAccountState
		desiredHashes []string
		wantLogs      int
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
		{name: "matching_claims", observations: []*domain.NatsAccountState{matching}},
		{name: "incomplete_account_with_matching_claims", observations: []*domain.NatsAccountState{incomplete}},
		{name: "unknown_observation", observations: []*domain.NatsAccountState{unknown}},
		{name: "skipped_validation", observations: []*domain.NatsAccountState{nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAccountLoggingFixture(t, len(tt.observations), accountLoggingOptions{})

			for i, state := range tt.observations {
				desiredHash := "desired-hash"
				if tt.desiredHashes != nil {
					desiredHash = tt.desiredHashes[i]
				}
				result := fixture.result(state, desiredHash)
				err := fixture.reconcile(result)
				require.NoError(t, err)
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
