package controller

import (
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	ktypes "k8s.io/apimachinery/pkg/types"
)

func Test_findAdoptionByUID(t *testing.T) {
	adoptions := []v1alpha1.AccountAdoption{
		{UID: "export-1", Name: "first"},
		{UID: "export-2", Name: "second"},
	}

	tests := []struct {
		name    string
		account *v1alpha1.Account
		uid     ktypes.UID
		want    *v1alpha1.AccountAdoption
	}{
		{
			name:    "nil_adoptions",
			account: &v1alpha1.Account{},
			uid:     "export-1",
			want:    nil,
		},
		{
			name: "matching_uid",
			account: &v1alpha1.Account{
				Status: v1alpha1.AccountStatus{
					Adoptions: &v1alpha1.AccountAdoptions{Exports: adoptions},
				},
			},
			uid:  "export-2",
			want: &adoptions[1],
		},
		{
			name: "missing_uid",
			account: &v1alpha1.Account{
				Status: v1alpha1.AccountStatus{
					Adoptions: &v1alpha1.AccountAdoptions{Exports: adoptions},
				},
			},
			uid:  "export-3",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAdoptionByUID(tt.account, tt.uid)
			assert.Equal(t, tt.want, got)
		})
	}
}
