package types

import (
	"fmt"

	k8s "k8s.io/apimachinery/pkg/types"
)

// Type: AccountIn

type AccountIn struct {
	namespacedName  k8s.NamespacedName
	accountPubKey   AcPubKey
	signedBy        OpPubKey
	accountLLimits  *AccountLimits
	jetStreamLimits *JetStreamLimits
	natsLimits      *NatsLimits
}

func (a *AccountIn) NamespacedName() k8s.NamespacedName {
	return a.namespacedName
}

func (a *AccountIn) AccountPubKey() AcPubKey {
	return a.accountPubKey
}

func (a *AccountIn) SignedBy() OpPubKey {
	return a.signedBy
}

func (a *AccountIn) AccountLimits() *AccountLimits {
	return a.accountLLimits
}

func (a *AccountIn) JetStreamLimits() *JetStreamLimits {
	return a.jetStreamLimits
}

func (a *AccountIn) NatsLimits() *NatsLimits {
	return a.natsLimits
}

func (a *AccountIn) Validate() error {
	if err := ValidateNamespacedName(a.namespacedName); err != nil {
		return fmt.Errorf("invalid NamespacedName: %w", err)
	}
	if err := a.accountPubKey.ValidateOptional(); err != nil {
		return fmt.Errorf("invalid AccountPubKey: %w", err)
	}
	if err := a.signedBy.ValidateOptional(); err != nil {
		return fmt.Errorf("invalid SignedBy: %w", err)
	}
	return nil
}

type AccountInOption func(*AccountIn)

func NewAccountIn(namespacedName k8s.NamespacedName, options ...AccountInOption) (*AccountIn, error) {
	result := &AccountIn{
		namespacedName: namespacedName,
	}

	for _, option := range options {
		option(result)
	}

	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("invalid AccountIn: %w", err)
	}

	return result, nil
}

func WithAccountPubKey(accountPubKey AcPubKey) AccountInOption {
	return func(a *AccountIn) {
		a.accountPubKey = accountPubKey
	}
}

func WithSignedBy(signedBy OpPubKey) AccountInOption {
	return func(a *AccountIn) {
		a.signedBy = signedBy
	}
}

func WithAccountLimits(accountLLimits *AccountLimits) AccountInOption {
	return func(a *AccountIn) {
		a.accountLLimits = accountLLimits
	}
}

func WithJetStreamLimits(jetStreamLimits *JetStreamLimits) AccountInOption {
	return func(a *AccountIn) {
		a.jetStreamLimits = jetStreamLimits
	}
}

func WithNatsLimits(natsLimits *NatsLimits) AccountInOption {
	return func(a *AccountIn) {
		a.natsLimits = natsLimits
	}
}

// Type: AccountOut

type AccountOut struct {
	AccountPubKey AcPubKey
	SignedBy      OpPubKey
	Claims        *AccountClaims
}

func NewAccountOut(accountPubKey AcPubKey, signedBy OpPubKey, claims *AccountClaims) (*AccountOut, error) {
	result := &AccountOut{
		AccountPubKey: accountPubKey,
		SignedBy:      signedBy,
		Claims:        claims,
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("invalid AccountOut: %w", err)
	}
	return result, nil
}

func (a *AccountOut) Validate() error {
	if err := a.AccountPubKey.ValidateRequired(); err != nil {
		return fmt.Errorf("invalid AccountPubKey: %w", err)
	}
	if err := a.SignedBy.ValidateRequired(); err != nil {
		return fmt.Errorf("invalid SignedBy: %w", err)
	}
	if a.Claims == nil {
		return fmt.Errorf("invalid Claims: required")
	}
	return nil
}

type AccountClaims struct {
	AccountLimits   *AccountLimits
	JetStreamLimits *JetStreamLimits
	NatsLimits      *NatsLimits
	Exports         AccountExports
	Imports         AccountImports
}

type AccountLimits struct {
	Imports         *int64
	Exports         *int64
	WildcardExports *bool
	Conn            *int64
	LeafNodeConn    *int64
}

type JetStreamLimits struct {
	MemoryStorage        *int64
	DiskStorage          *int64
	Streams              *int64
	Consumer             *int64
	MaxAckPending        *int64
	MemoryMaxStreamBytes *int64
	DiskMaxStreamBytes   *int64
	MaxBytesRequired     *bool
}

type NatsLimits struct {
	Subs    *int64
	Data    *int64
	Payload *int64
}
