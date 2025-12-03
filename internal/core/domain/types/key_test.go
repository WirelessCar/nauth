package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	ValidAcPubKey AcPubKey = "A1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789"
	ValidOpPubKey OpPubKey = "O1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789"
)

func TestValidate_AcPubKey(t *testing.T) {
	cases := []struct {
		desc    string
		key     AcPubKey
		req     bool
		wantErr error
	}{
		{
			desc:    "valid key",
			key:     ValidAcPubKey,
			req:     true,
			wantErr: nil,
		},
		{
			desc:    "invalid key - wrong prefix",
			key:     "B1234567890123456789012345678901234567890123456789012345",
			req:     true,
			wantErr: fmt.Errorf("account public key malformed"),
		},
		{
			desc:    "invalid key - wrong prefix optional",
			key:     "B1234567890123456789012345678901234567890123456789012345",
			req:     false,
			wantErr: fmt.Errorf("account public key malformed"),
		},
		{"invalid key - too short", "A12345", true, fmt.Errorf("account public key malformed")},
		{"empty key - req", "", true, fmt.Errorf("account public key required")},
		{"empty key - not req", "", false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// When
			errInt := tc.key.validate(tc.req)
			var errExt error
			if tc.req {
				errExt = tc.key.ValidateRequired()
			} else {
				errExt = tc.key.ValidateOptional()
			}

			// Then
			if tc.wantErr == nil && errInt != nil {
				t.Fatalf("expected no error, got %v", errInt)
			}
			if tc.wantErr != nil && (errInt == nil || errInt.Error() != tc.wantErr.Error()) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, errInt)
			}
			assert.Equal(t, errInt, errExt)
		})
	}
}

func TestValidate_OpPubKey(t *testing.T) {
	cases := []struct {
		desc    string
		key     OpPubKey
		req     bool
		wantErr error
	}{
		{
			desc:    "valid key",
			key:     ValidOpPubKey,
			req:     true,
			wantErr: nil,
		},
		{
			desc:    "invalid key - wrong prefix",
			key:     "B1234567890123456789012345678901234567890123456789012345",
			req:     true,
			wantErr: fmt.Errorf("operator public key malformed"),
		},
		{
			desc:    "invalid key - wrong prefix optional",
			key:     "B1234567890123456789012345678901234567890123456789012345",
			req:     false,
			wantErr: fmt.Errorf("operator public key malformed"),
		},
		{"invalid key - too short", "A12345", true, fmt.Errorf("operator public key malformed")},
		{"empty key - req", "", true, fmt.Errorf("operator public key required")},
		{"empty key - not req", "", false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// When
			errInt := tc.key.validate(tc.req)
			var errExt error
			if tc.req {
				errExt = tc.key.ValidateRequired()
			} else {
				errExt = tc.key.ValidateOptional()
			}

			// Then
			if tc.wantErr == nil && errInt != nil {
				t.Fatalf("expected no error, got %v", errInt)
			}
			if tc.wantErr != nil && (errInt == nil || errInt.Error() != tc.wantErr.Error()) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, errInt)
			}
			assert.Equal(t, errInt, errExt)
		})
	}
}
