package types

import (
	"fmt"
	"testing"
)

const (
	ValidAcPubKey AcPubKey = "A1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789"
	ValidOpPubKey OpPubKey = "O1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890123456789"
)

func TestValidate_AcPubKey(t *testing.T) {
	cases := []struct {
		desc    string
		key     AcPubKey
		wantErr error
	}{
		{
			desc:    "valid key",
			key:     ValidAcPubKey,
			wantErr: nil,
		},
		{
			desc:    "wrong prefix",
			key:     "B1234567890123456789012345678901234567890123456789012345",
			wantErr: fmt.Errorf("account public key malformed"),
		},
		{
			desc:    "too short",
			key:     "A0123456789",
			wantErr: fmt.Errorf("account public key malformed"),
		},
		{
			desc:    "empty",
			key:     "",
			wantErr: fmt.Errorf("account public key required"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// When
			err := tc.key.Validate()

			// Then
			if tc.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr != nil && (err == nil || err.Error() != tc.wantErr.Error()) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidate_OpPubKey(t *testing.T) {
	cases := []struct {
		desc    string
		key     OpPubKey
		wantErr error
	}{
		{
			desc:    "valid key",
			key:     ValidOpPubKey,
			wantErr: nil,
		},
		{
			desc:    "wrong prefix",
			key:     "B1234567890123456789012345678901234567890123456789012345",
			wantErr: fmt.Errorf("operator public key malformed"),
		},
		{
			desc:    "too short",
			key:     "O1234567890",
			wantErr: fmt.Errorf("operator public key malformed"),
		},
		{
			desc:    "empty",
			key:     "",
			wantErr: fmt.Errorf("operator public key required"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// When
			err := tc.key.Validate()

			// Then
			if tc.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr != nil && (err == nil || err.Error() != tc.wantErr.Error()) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}
