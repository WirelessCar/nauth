package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccountInValidate_ShouldSucceed_WhenRequiredFieldsSupplied(t *testing.T) {
	// Given
	account := &AccountIn{
		NamespacedName: NamespacedName{
			Namespace: "my-namespace",
			Name:      "my-resource",
		},
	}

	// When
	err := account.Validate()

	// Then
	assert.Nil(t, err)
}

func TestAccountInValidate_ShouldSucceed_WhenAllFieldsSupplied(t *testing.T) {
	// Given
	account := &AccountIn{
		NamespacedName{
			Namespace: "my-namespace",
			Name:      "my-resource",
		},
		ValidAcPubKey,
		ValidOpPubKey,
		&AccountLimits{},
		&JetStreamLimits{},
		&NatsLimits{},
	}

	// When
	err := account.Validate()

	// Then
	assert.Nil(t, err)
}

func TestAccountInValidate_ShouldFail_WhenNamespacedNameInvalid(t *testing.T) {
	// Given
	account := &AccountIn{
		NamespacedName: NamespacedName{
			Namespace: "my-namespace",
		},
	}

	// When
	err := account.Validate()

	// Then
	assert.ErrorContains(t, err, "invalid NamespacedName: name required")
}

func TestAccountInValidate_ShouldFail_WhenAccountPubKeyInvalid(t *testing.T) {
	// Given
	account := &AccountIn{
		NamespacedName: NamespacedName{
			Namespace: "my-namespace",
			Name:      "my-resource",
		},
		AccountPubKey: AcPubKey("INVALID"),
	}

	// When
	err := account.Validate()

	// Then
	assert.ErrorContains(t, err, "invalid AccountPubKey: account public key malformed")
}

func TestAccountInValidate_ShouldFail_WhenSignedByInvalid(t *testing.T) {
	// Given
	account := &AccountIn{
		NamespacedName: NamespacedName{
			Namespace: "my-namespace",
			Name:      "my-resource",
		},
		AccountPubKey: ValidAcPubKey,
		SignedBy:      OpPubKey("INVALID"),
	}

	// When
	err := account.Validate()

	// Then
	assert.ErrorContains(t, err, "invalid SignedBy: operator public key malformed")
}
