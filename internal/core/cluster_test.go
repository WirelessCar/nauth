package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ClusterTestSuite struct {
	suite.Suite
	ctx               context.Context
	clusterReaderMock *ClusterReaderMock
	natsSysClientMock *NatsSysClientMock
	natsSysConnMock   *NatsSysConnectionMock
}

func (t *ClusterTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.clusterReaderMock = NewClusterReaderMock()
	t.natsSysClientMock = NewNatsSysClientMock()
	t.natsSysConnMock = NewNatsSysConnectionMock()
}

func (t *ClusterTestSuite) TearDownTest() {
	t.clusterReaderMock.AssertExpectations(t.T())
	t.natsSysClientMock.AssertExpectations(t.T())
	t.natsSysConnMock.AssertExpectations(t.T())
}

func TestClusterManager_TestSuite(t *testing.T) {
	suite.Run(t, new(ClusterTestSuite))
}

func (t *ClusterTestSuite) Test_ResolveClusterTarget_ShouldSucceed_WhenOperatorClusterRef() {
	// Given
	opClusterRef := nauth.ClusterRef("my-namespace/my-cluster")
	unitUnderTest := t.newUnitUnderTest(&opClusterRef, false, "nats")

	opClusterTarget := t.generateClusterTarget()
	t.clusterReaderMock.mockGetTarget(t.ctx, "my-namespace/my-cluster", &opClusterTarget)

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, nil)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &opClusterTarget, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenAccountClusterRef() {
	// Given´
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats")

	acClusterRef, err := nauth.NewClusterRef("ac-namespace/ac-cluster")
	require.NoError(t.T(), err)
	acClusterTarget := t.generateClusterTarget()
	t.clusterReaderMock.mockGetTarget(t.ctx, "ac-namespace/ac-cluster", &acClusterTarget)

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, &acClusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), acClusterTarget, *result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenAccountClusterRefSameAsNonOptionalOperatorClusterRef() {
	// Given
	clusterRef := nauth.ClusterRef("my-namespace/my-cluster")
	clusterTarget := t.generateClusterTarget()
	t.clusterReaderMock.mockGetTarget(t.ctx, "my-namespace/my-cluster", &clusterTarget).Once()

	unitUnderTest := t.newUnitUnderTest(&clusterRef, false, "nats")

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, &clusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &clusterTarget, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenAccountClusterRefDifferentFromOptionalOperatorClusterRef() {
	// Given
	opClusterRef := nauth.ClusterRef("op-namespace/op-cluster")
	unitUnderTest := t.newUnitUnderTest(&opClusterRef, true, "nats")

	acClusterRef := nauth.ClusterRef("ac-namespace/ac-cluster")
	acClusterTarget := t.generateClusterTarget()
	t.clusterReaderMock.mockGetTarget(t.ctx, acClusterRef, &acClusterTarget)

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, &acClusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &acClusterTarget, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenAccountClusterRefDifferentFromNonOptionalOperatorClusterRef() {
	// Given
	opClusterRef := nauth.ClusterRef("op-namespace/op-cluster")
	unitUnderTest := t.newUnitUnderTest(&opClusterRef, false, "nats")
	acClusterRef := nauth.ClusterRef("ac-namespace/ac-cluster")

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, &acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "account cluster reference ac-namespace/ac-cluster does not match required operator cluster op-namespace/op-cluster")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenOperatorClusterNotFound() {
	// Given
	opClusterRef := nauth.ClusterRef("my-namespace/my-cluster")
	unitUnderTest := t.newUnitUnderTest(&opClusterRef, false, "nats")
	acClusterRef := nauth.ClusterRef("my-namespace/my-cluster")
	t.clusterReaderMock.mockGetTargetError(t.ctx, acClusterRef, fmt.Errorf("fake not found"))

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target \"my-namespace/my-cluster\": fake not found")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenNeitherAccountNorOperatorClusterRefDefined() {
	// Given
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats")

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "no cluster reference provided and no operator cluster configured")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenAccountClusterNotFound() {
	// Given
	opClusterRef := nauth.ClusterRef("op-namespace/op-cluster")
	unitUnderTest := t.newUnitUnderTest(&opClusterRef, true, "nats")
	acClusterRef := nauth.ClusterRef("ac-namespace/ac-cluster")
	t.clusterReaderMock.mockGetTargetError(t.ctx, acClusterRef, fmt.Errorf("fake not found"))

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, &acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target \"ac-namespace/ac-cluster\": fake not found")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenAccountClusterRefDoesNotContainNamespace() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	acClusterRef := nauth.ClusterRef("ac-cluster")

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, &acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "invalid account cluster ref:")
	require.ErrorContains(t.T(), err, "invalid cluster ref \"ac-cluster\" (expected NamespacedName):")
	require.ErrorContains(t.T(), err, "invalid Namespaced Name format \"ac-cluster\": expected namespace/name")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_Validate_ShouldSucceed() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	clusterTarget := t.generateClusterTarget()
	t.natsSysClientMock.mockConnect(clusterTarget.NatsURL, clusterTarget.SystemAdminCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockVerifySystemAccountAccess()
	t.natsSysConnMock.mockDisconnect()

	// When
	err := unitUnderTest.Validate(t.ctx, clusterTarget)

	// Then
	t.NoError(err)
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenOperatorSigningKeySecretMissing() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	clusterTarget := t.generateClusterTarget()
	clusterTarget.OperatorSigningKey = nil

	// When
	err := unitUnderTest.Validate(t.ctx, clusterTarget)

	// Then
	t.ErrorContains(err, "invalid cluster target: operator signing key is required")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenSystemAccountUserCredsSecretMissing() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	clusterTarget := t.generateClusterTarget()
	clusterTarget.SystemAdminCreds = domain.NatsUserCreds{}

	// When
	err := unitUnderTest.Validate(t.ctx, clusterTarget)

	// Then
	t.ErrorContains(err, "invalid cluster target: invalid system admin credentials: credentials cannot be empty")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenNatsConnectionFails() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	clusterTarget := t.generateClusterTarget()
	t.natsSysClientMock.mockConnectError(clusterTarget.NatsURL, clusterTarget.SystemAdminCreds, fmt.Errorf("authentication failed"))

	// When
	err := unitUnderTest.Validate(t.ctx, clusterTarget)

	// Then
	t.ErrorContains(err, "connect to NATS cluster using System Account User Credentials: authentication failed")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenVerifySystemAccountAccessFails() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	clusterTarget := t.generateClusterTarget()
	t.natsSysClientMock.mockConnect(clusterTarget.NatsURL, clusterTarget.SystemAdminCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockVerifySystemAccountAccessError(fmt.Errorf("permission denied"))
	t.natsSysConnMock.mockDisconnect()

	// When
	err := unitUnderTest.Validate(t.ctx, clusterTarget)

	// Then
	t.ErrorContains(err, "verify NATS System Account access: permission denied")
}

func (t *ClusterTestSuite) newUnitUnderTestWithDefaults() *ClusterManager {
	return t.newUnitUnderTest(nil, false, "")
}

func (t *ClusterTestSuite) newUnitUnderTest(opClusterRef *nauth.ClusterRef, opClusterOptional bool, opNamespace domain.Namespace) *ClusterManager {
	var operatorNatsCluster *OperatorNatsCluster
	var err error
	if opClusterRef != nil {
		operatorNatsCluster, err = NewOperatorNatsCluster(*opClusterRef, opClusterOptional)
		if err != nil {
			t.Failf("failed to create operator NATS cluster config", "error: %v", err)
			return nil
		}
	}

	config, err := NewConfig(operatorNatsCluster, opNamespace)
	if err != nil {
		t.Failf("failed to create operator config", "error: %v", err)
		return nil
	}

	u, err := NewClusterManager(
		t.clusterReaderMock,
		t.natsSysClientMock,
		config,
	)
	if err != nil {
		t.Failf("failed to create ClusterManager", "error: %v", err)
		return nil
	}

	return u
}

func (t *ClusterTestSuite) generateClusterTarget() nauth.ClusterTarget {
	opSign, _ := nkeys.CreateOperator()

	acKey, _ := nkeys.CreateAccount()
	acKeyPub, _ := acKey.PublicKey()

	sauKey, _ := nkeys.CreateUser()
	sauKeySeed, _ := sauKey.Seed()
	sauKeyPub, _ := sauKey.PublicKey()

	sauClaims := jwt.NewUserClaims(sauKeyPub)
	sauClaims.IssuerAccount = acKeyPub

	sauJwt, err := sauClaims.Encode(acKey)
	if err != nil {
		t.Failf("failed to encode SAU JWT", "error: %v", err)
	}
	sauCreds, err := jwt.FormatUserConfig(sauJwt, sauKeySeed)
	if err != nil {
		t.Failf("failed to format SAU creds", "error: %v", err)
	}
	sauNatsUserCreds, err := domain.NewNatsUserCreds(sauCreds)
	if err != nil {
		t.Failf("failed to create NatsUserCreds from SAU creds", "error: %v", err)
	}

	return nauth.ClusterTarget{
		NatsURL:            "nats://my-cluster:4222",
		OperatorSigningKey: opSign,
		SystemAdminCreds:   *sauNatsUserCreds,
	}
}
