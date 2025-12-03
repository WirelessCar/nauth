package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	k8s "k8s.io/apimachinery/pkg/types"
)

func TestNewAccountIn_ShouldSucceed_WhenRequiredFieldsSupplied(t *testing.T) {
	// When
	account, err := NewAccountIn(ValidNamespacedName)

	// Then
	assert.Nil(t, err, "expected successful creation of AccountIn")
	assert.EqualValues(
		t,
		&AccountIn{namespacedName: ValidNamespacedName},
		account,
	)
	assert.Equal(t, account.NamespacedName(), ValidNamespacedName)
	assert.Empty(t, account.AccountPubKey())
	assert.Empty(t, account.SignedBy())
}

func TestNewAccountIn_ShouldSucceed_WhenAllFieldsSupplied(t *testing.T) {
	// When
	account, err := NewAccountIn(ValidNamespacedName,
		WithAccountPubKey(ValidAcPubKey),
		WithSignedBy(ValidOpPubKey))

	// Then
	assert.Nil(t, err, "expected successful creation of AccountIn")
	assert.EqualValues(
		t,
		&AccountIn{namespacedName: ValidNamespacedName, accountPubKey: ValidAcPubKey, signedBy: ValidOpPubKey},
		account,
	)
	assert.Equal(t, account.NamespacedName(), ValidNamespacedName)
	assert.Equal(t, account.AccountPubKey(), ValidAcPubKey)
	assert.Equal(t, account.SignedBy(), ValidOpPubKey)
}

func TestNewAccountIn_ShouldFail_WhenNamespacedNameInvalid(t *testing.T) {
	// When
	_, err := NewAccountIn(k8s.NamespacedName{Namespace: "my-namespace"})

	// Then
	assert.ErrorContains(t, err, "invalid AccountIn: invalid NamespacedName: name required")
}

func TestNewAccountIn_ShouldFail_WhenAccountPubKeyInvalid(t *testing.T) {
	// When
	_, err := NewAccountIn(ValidNamespacedName, WithAccountPubKey("OMGINVALIDKEY"))

	// Then
	assert.ErrorContains(t, err, "invalid AccountIn: invalid AccountPubKey: account public key malformed")
}

func TestNewAccountIn_ShouldFail_WhenSignedByInvalid(t *testing.T) {
	// When
	_, err := NewAccountIn(ValidNamespacedName, WithSignedBy("OMGINVALIDKEY"))

	// Then
	assert.ErrorContains(t, err, "invalid AccountIn: invalid SignedBy: operator public key malformed")
}

func TestNewAccountOut_ShouldFail_WhenInvalidAccountPubKeySupplied(t *testing.T) {
	// Given
	accountPubKey := AcPubKey("INVALIDKEY")
	signedBy := OpPubKey("O1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	claims := &AccountClaims{}

	// When
	_, err := NewAccountOut(accountPubKey, signedBy, claims)

	// Then
	assert.ErrorContains(t, err, "invalid AccountOut: invalid AccountPubKey: account public key malformed")
}

func TestNewAccountOut_ShouldFail_WhenInvalidSignedBySupplied(t *testing.T) {
	// Given
	accountPubKey := AcPubKey("A1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	signedBy := OpPubKey("INVALIDKEY")
	claims := &AccountClaims{}

	// When
	_, err := NewAccountOut(accountPubKey, signedBy, claims)

	// Then
	assert.ErrorContains(t, err, "invalid AccountOut: invalid SignedBy: operator public key malformed")
}

func TestNewAccountOut_ShouldFail_WhenClaimsNotSupplied(t *testing.T) {
	// Given
	accountPubKey := AcPubKey("A1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")
	signedBy := OpPubKey("O1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789")

	// When
	_, err := NewAccountOut(accountPubKey, signedBy, nil)

	// Then
	assert.ErrorContains(t, err, "invalid AccountOut: invalid Claims: required")
}
