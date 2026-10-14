package v1alpha1

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUserList_UnmarshalMalformedExpiresAt(t *testing.T) {
	var users UserList
	err := json.Unmarshal([]byte(`{"items":[{"spec":{"expiresAt":"2026-10-13T08:00:00x5Z"}}]}`), &users)
	if err != nil {
		t.Fatalf("unmarshal malformed timestamp: %v", err)
	}
	if len(users.Items) != 1 {
		t.Fatalf("unmarshal returned %d users, want 1", len(users.Items))
	}

	if _, err := users.Items[0].Spec.ExpiresAtAsTime(); err == nil {
		t.Fatal("ExpiresAtAsTime() accepted malformed timestamp")
	}
}

func TestUserSpec_ExpiresAtAsTime(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt string
		wantTime  time.Time
		wantNil   bool
		wantError bool
	}{
		{
			name:    "unset",
			wantNil: true,
		},
		{
			name:      "utc",
			expiresAt: "2026-10-13T08:00:00Z",
			wantTime:  time.Date(2026, time.October, 13, 8, 0, 0, 0, time.UTC),
		},
		{
			name:      "fractional_seconds_with_offset",
			expiresAt: "2026-10-13T08:00:00.125+02:30",
			wantTime:  time.Date(2026, time.October, 13, 8, 0, 0, 125_000_000, time.FixedZone("", 2*60*60+30*60)),
		},
		{
			name:      "invalid_timezone_suffix",
			expiresAt: "2026-10-13T08:00:00x5Z",
			wantError: true,
		},
		{
			name:      "invalid_calendar_date",
			expiresAt: "2026-02-30T08:00:00Z",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (UserSpec{ExpiresAt: tt.expiresAt}).ExpiresAtAsTime()
			if tt.wantError {
				if err == nil {
					t.Fatal("ExpiresAtAsTime() expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpiresAtAsTime() error = %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ExpiresAtAsTime() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ExpiresAtAsTime() returned nil time")
			}
			if !got.Equal(tt.wantTime) {
				t.Errorf("ExpiresAtAsTime() = %v, want %v", got, tt.wantTime)
			}
		})
	}
}
