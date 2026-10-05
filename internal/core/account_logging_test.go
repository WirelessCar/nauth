package core

import (
	"errors"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (t *AccountManagerTestSuite) Test_DeleteAccountJWTLogging() {
	deleteErr := errors.New("NATS deletion failed")
	cleanupErr := errors.New("secret cleanup failed")
	tests := []struct {
		name       string
		deleteErr  error
		cleanupErr error
		wantErr    error
		wantLogs   int
	}{
		{name: "successful_deletion", wantLogs: 1},
		{name: "failed_nats_deletion", deleteErr: deleteErr, wantErr: deleteErr},
		{name: "deletion_acknowledged_before_cleanup_failure", cleanupErr: cleanupErr, wantErr: cleanupErr, wantLogs: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func() {
			defer t.assertAndResetAllMock()
			logCore, logs := observer.New(zapcore.InfoLevel)
			ctx := logf.IntoContext(t.ctx, zapr.NewLogger(zap.New(logCore)))
			accountRef := domain.NewNamespacedName("account-namespace", "account-name")
			accountID := testutil.NatsTestAccountA.AccountID()
			t.secretManagerMock.mockGetSecretsMissing(ctx, accountRef, accountID)
			t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock).Once()
			t.natsSysConnMock.On("DeleteAccountJWT", mock.Anything).Return(tt.deleteErr).Once()
			t.natsSysConnMock.mockDisconnect().Once()
			if tt.deleteErr == nil {
				t.secretManagerMock.mockDeleteAll(ctx, accountRef, accountID).Return(tt.cleanupErr).Once()
			}

			err := t.unitUnderTest.Delete(ctx, nauth.AccountReference{
				AccountRef: accountRef, AccountID: nauth.AccountID(accountID), ClusterTarget: t.clusterTarget,
			})
			if tt.wantErr != nil {
				t.Require().ErrorIs(err, tt.wantErr)
			} else {
				t.Require().NoError(err)
			}
			entries := logs.FilterMessage("NATS acknowledged Account JWT deletion").All()
			t.Require().Len(entries, tt.wantLogs)
			for _, entry := range entries {
				t.Equal(zapcore.InfoLevel, entry.Level)
				fields := entry.ContextMap()
				t.Equal(accountRef.Name, fields["name"])
				t.Equal(accountRef.Namespace, fields["namespace"])
				t.Equal(accountID, fields["accountID"])
				t.Equal(t.clusterTarget.UID, fields["natsClusterUID"])
			}
		})
	}
}
