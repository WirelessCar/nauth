package types

import (
	"regexp"
)

var (
	acPubKeyRegex = regexp.MustCompile("^A[0-9A-Z]{55}$")
	opPubKeyRegex = regexp.MustCompile("^O[0-9A-Z]{55}$")
)

type AcPubKey string

func (a AcPubKey) ValidateOptional() error {
	return a.validate(false)
}

func (a AcPubKey) ValidateRequired() error {
	return a.validate(true)
}

func (a AcPubKey) validate(required bool) error {
	return ValidateString(string(a), "account public key", required, acPubKeyRegex)
}

type OpPubKey string

func (a OpPubKey) ValidateOptional() error {
	return a.validate(false)
}

func (a OpPubKey) ValidateRequired() error {
	return a.validate(true)
}

func (a OpPubKey) validate(required bool) error {
	return ValidateString(string(a), "operator public key", required, opPubKeyRegex)
}
