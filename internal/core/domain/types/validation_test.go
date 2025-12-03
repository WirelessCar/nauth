package types

import (
	"fmt"
	"regexp"
	"testing"

	k8s "k8s.io/apimachinery/pkg/types"
)

var (
	ValidNamespacedName = k8s.NamespacedName{
		Namespace: "my-namespace",
		Name:      "my-resource",
	}
)

func TestValidateString(t *testing.T) {
	regex := regexp.MustCompile("^[A-Z]+$")
	cases := []struct {
		description string
		value       string
		name        string
		req         bool
		regex       *regexp.Regexp
		wantErr     error
	}{
		{
			description: "valid req",
			value:       "ABCDEF",
			name:        "someFieldName",
			req:         true,
			regex:       regex,
			wantErr:     nil,
		},
		{
			description: "valid optional empty",
			value:       "",
			name:        "someFieldName",
			req:         false,
			regex:       regex,
			wantErr:     nil,
		},
		{
			description: "invalid required empty",
			value:       "",
			name:        "someFieldName",
			req:         true,
			regex:       regex,
			wantErr:     fmt.Errorf("someFieldName required"),
		},
		{
			description: "invalid format",
			value:       "ABC123",
			name:        "someFieldName",
			req:         true,
			regex:       regex,
			wantErr:     fmt.Errorf("someFieldName malformed"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			// When
			err := ValidateString(tc.value, tc.name, tc.req, tc.regex)

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

func TestValidateNamespacedName(t *testing.T) {
	cases := []struct {
		description string
		value       k8s.NamespacedName
		expectErr   error
	}{
		{
			description: "valid",
			value:       ValidNamespacedName,
			expectErr:   nil,
		},
		{
			description: "empty",
			value:       k8s.NamespacedName{},
			expectErr:   fmt.Errorf("namespace required"),
		},
		{
			description: "namespace empty",
			value: k8s.NamespacedName{
				Name: "my-resource",
			},
			expectErr: fmt.Errorf("namespace required"),
		},
		{
			description: "desc empty",
			value: k8s.NamespacedName{
				Namespace: "my-namespace",
			},
			expectErr: fmt.Errorf("name required"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			// When
			err := ValidateNamespacedName(tc.value)

			// Then
			if tc.expectErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.expectErr != nil && (err == nil || err.Error() != tc.expectErr.Error()) {
				t.Fatalf("expected error %v, got %v", tc.expectErr, err)
			}
		})
	}
}
