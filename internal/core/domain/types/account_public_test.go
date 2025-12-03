package types_test

import (
	"testing"

	. "github.com/WirelessCar/nauth/internal/core/domain/types"
	k8s "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
)

func TestNewAccount_FromPublic_ShouldSucceed_WhenRequiredFieldsSupplied(t *testing.T) {
	// Given
	namespacedName := k8s.NamespacedName{Namespace: "my-namespace", Name: "my-account"}

	// When
	account, err := NewAccountIn(namespacedName)

	// Then
	assert.Nil(t, err, "expected successful creation of AccountIn")
	assert.Equal(t, account.NamespacedName(), namespacedName)
	assert.Empty(t, account.AccountPubKey())
	assert.Empty(t, account.SignedBy())
}

func TestNewAccountIn_FromPublic_ShouldSucceed_WhenAllFieldsSupplied(t *testing.T) {
	// Given
	namespacedName := k8s.NamespacedName{Namespace: "my-namespace", Name: "my-account"}
	acPubKey := AcPubKey("A1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	opPubKey := OpPubKey("O1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	accountLimits := &AccountLimits{
		Conn: ptr.To[int64](200),
	}
	jetStreamLimits := &JetStreamLimits{
		Consumer: ptr.To[int64](500),
	}
	natsLimits := &NatsLimits{
		Data: ptr.To[int64](104857600),
	}

	// When
	account, err := NewAccountIn(namespacedName,
		WithAccountPubKey(acPubKey),
		WithSignedBy(opPubKey),
		WithAccountLimits(accountLimits),
		WithJetStreamLimits(jetStreamLimits),
		WithNatsLimits(natsLimits))

	// Then
	assert.Nil(t, err, "expected successful creation of AccountIn")
	assert.Equal(t, account.NamespacedName(), namespacedName)
	assert.Equal(t, account.AccountPubKey(), acPubKey)
	assert.Equal(t, account.SignedBy(), opPubKey)
	assert.Same(t, account.AccountLimits(), accountLimits)
	assert.Same(t, account.JetStreamLimits(), jetStreamLimits)
	assert.Same(t, account.NatsLimits(), natsLimits)
}

func TestNewAccountOut_FromPublic_ShouldSucceed_WhenRequiredFieldsSupplied(t *testing.T) {
	// Given
	accountPubKey := AcPubKey("A1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	signedBy := OpPubKey("O1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	claims := &AccountClaims{}

	// When
	account, err := NewAccountOut(accountPubKey, signedBy, claims)

	// Then
	assert.Nil(t, err, "expected successful creation of AccountOut")
	assert.Equal(t, account.AccountPubKey, accountPubKey)
	assert.Equal(t, account.SignedBy, signedBy)
	assert.Same(t, account.Claims, claims)
}
