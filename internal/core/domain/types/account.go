package types

import (
	"fmt"
)

type AccountIn struct {
	NamespacedName  NamespacedName
	AccountPubKey   AcPubKey
	SignedBy        OpPubKey
	AccountLLimits  *AccountLimits
	JetStreamLimits *JetStreamLimits
	NatsLimits      *NatsLimits
}

func (a *AccountIn) Validate() error {
	if err := a.NamespacedName.Validate(); err != nil {
		return fmt.Errorf("invalid NamespacedName: %w", err)
	}
	if a.AccountPubKey != "" {
		if err := a.AccountPubKey.Validate(); err != nil {
			return fmt.Errorf("invalid AccountPubKey: %w", err)
		}
	}
	if a.SignedBy != "" {
		if err := a.SignedBy.Validate(); err != nil {
			return fmt.Errorf("invalid SignedBy: %w", err)
		}
	}
	return nil
}

type AccountOut struct {
	AccountPubKey AcPubKey
	SignedBy      OpPubKey
	Claims        *AccountClaims
}

func (a *AccountOut) Validate() error {
	if err := a.AccountPubKey.Validate(); err != nil {
		return fmt.Errorf("invalid AccountPubKey: %w", err)
	}
	if err := a.SignedBy.Validate(); err != nil {
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
