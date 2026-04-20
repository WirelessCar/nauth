package core

import (
	"context"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_mapToJwtExport(t *testing.T) {
	responseThreshold := time.Duration(10)
	tokenPos := uint(2)
	advertise := true
	allowTrace := true

	have := v1alpha1.AccountExportRule{
		Name:              "Name",
		Subject:           "Subject.*",
		Type:              "service",
		ResponseType:      "Singleton",
		ResponseThreshold: &responseThreshold,
		Latency: &v1alpha1.ServiceLatency{
			Sampling: 20,
			Results:  "Results",
		},
		AccountTokenPosition: &tokenPos,
		Advertise:            &advertise,
		AllowTrace:           &allowTrace,
	}
	want := &jwt.Export{
		Name:              "Name",
		Subject:           "Subject.*",
		Type:              jwt.Service,
		TokenReq:          false,
		Revocations:       nil,
		ResponseType:      jwt.ResponseTypeSingleton,
		ResponseThreshold: responseThreshold,
		Latency: &jwt.ServiceLatency{
			Sampling: 20,
			Results:  "Results",
		},
		AccountTokenPosition: tokenPos,
		Advertise:            advertise,
		AllowTrace:           allowTrace,
		Info:                 jwt.Info{},
	}

	got := mapToJwtExport(have)
	assert.Equalf(t, want, got, "should be equal")
}

func Test_mapToJwtExport_requiredFields(t *testing.T) {

	have := v1alpha1.AccountExportRule{
		Subject: "Subject",
		Type:    "service",
	}
	want := &jwt.Export{
		Subject: "Subject",
		Type:    jwt.Service,
	}

	got := mapToJwtExport(have)
	assert.Equalf(t, want, got, "should be equal")
}

func TestAccountExportManager_CreateClaim(t *testing.T) {
	state := &v1alpha1.AccountExport{
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "MyAccount",
			Rules: []v1alpha1.AccountExportRule{
				{
					Name:    "export1",
					Subject: "mysubject",
					Type:    "service",
				},
			},
		},
	}

	claim, err := createClaim(context.Background(), state)

	assert.NoError(t, err)
	assert.NotNil(t, claim)
	assert.Len(t, claim.Rules, 1)

	exportRule := claim.Rules[0]
	assert.Equal(t, "export1", exportRule.Name)
	assert.Equal(t, v1alpha1.Subject("mysubject"), exportRule.Subject)
	assert.Equal(t, v1alpha1.ExportType("service"), exportRule.Type)
}

func TestAccountExportManager_CreateClaim_conflict(t *testing.T) {
	state := &v1alpha1.AccountExport{
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "MyAccount",
			Rules: []v1alpha1.AccountExportRule{
				{
					Name:    "export1",
					Subject: "mysubject",
					Type:    "service",
				},
				{
					Name:    "export2",
					Subject: "mysubject",
					Type:    "service",
				},
			},
		},
	}

	_, err := createClaim(context.Background(), state)
	assert.Error(t, err, "should fail")
}

func Test_mapExportType(t *testing.T) {
	type args struct {
		t v1alpha1.ExportType
	}
	tests := []struct {
		name string
		args args
		want jwt.ExportType
	}{
		{name: "service export type", args: args{t: "service"}, want: jwt.Service},
		{name: "stream export type", args: args{t: "stream"}, want: jwt.Stream},
		{name: "unknown export type", args: args{t: "something"}, want: jwt.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, mapExportType(tt.args.t), "should be equal")
		})
	}
}

func Test_findAdoptionByUID(t *testing.T) {
	type args struct {
		account *v1alpha1.Account
		export  *v1alpha1.AccountExport
	}

	adoptions := []v1alpha1.AccountAdoption{
		{
			UID:    "1",
			Status: v1alpha1.AccountAdoptionStatus{},
		},
	}
	acc := &v1alpha1.Account{
		Status: v1alpha1.AccountStatus{
			Adoptions: &v1alpha1.AccountAdoptions{
				Exports: adoptions,
			},
		},
	}

	createExport := func(uid string) *v1alpha1.AccountExport {
		a := v1alpha1.AccountExport{}
		a.UID = types.UID(uid)
		return &a
	}

	tests := []struct {
		name         string
		args         args
		wantErr      bool
		wantAdoption *v1alpha1.AccountAdoption
	}{
		{
			name:         "account has adoption with matching UID",
			args:         args{account: acc, export: createExport("1")},
			wantErr:      false,
			wantAdoption: &adoptions[0],
		},
		{
			name:         "account has no adoption with matching UID",
			args:         args{account: acc, export: createExport("2")},
			wantErr:      true,
			wantAdoption: nil,
		},
		{
			name:         "account has no adoptions",
			args:         args{account: &v1alpha1.Account{}, export: createExport("1")},
			wantErr:      true,
			wantAdoption: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findAdoption(tt.args.export, tt.args.account)
			assert.Equalf(t, tt.wantErr, err != nil, "shouldBeEqual")
			assert.Equalf(t, tt.wantAdoption, got, "shouldBeEqual")
		})
	}
}

func TestAccountExportManager_Resolve(t *testing.T) {
	createExport := func() *v1alpha1.AccountExport {
		export := &v1alpha1.AccountExport{
			ObjectMeta: metav1.ObjectMeta{
				UID:        "UID1",
				Generation: 1,
			},
			Spec: v1alpha1.AccountExportSpec{
				AccountName: "MyAccount",
				Rules: []v1alpha1.AccountExportRule{
					{
						Name:    "Rule1",
						Subject: "my.subject",
						Type:    "stream",
					},
				},
			},
		}
		return export
	}
	createAccount := func() *v1alpha1.Account {
		account := &v1alpha1.Account{
			Spec: v1alpha1.AccountSpec{},
			Status: v1alpha1.AccountStatus{
				Adoptions: &v1alpha1.AccountAdoptions{},
			},
		}
		return account
	}

	aem := NewAccountExportManager()

	t.Run("observedGeneration should match spec.Generation", func(t *testing.T) {
		resolution := aem.Resolve(context.Background(), createExport(), createAccount())

		assert.Equal(t, int64(1), resolution.ObservedGeneration)
	})

	t.Run("valid rules should generate desiredClaim", func(t *testing.T) {
		export := createExport()
		resolution := aem.Resolve(context.Background(), export, createAccount())

		assert.NoError(t, resolution.ValidationError)
		assert.NotNil(t, resolution.DesiredClaim)
		assert.Equal(t, export.Spec.Rules, resolution.DesiredClaim.Rules)
	})

	t.Run("invalid rules should not generate desiredClaim", func(t *testing.T) {
		export := createExport()
		export.Spec.Rules[0].Subject = ""
		resolution := aem.Resolve(context.Background(), export, createAccount())

		assert.Error(t, resolution.ValidationError)
		assert.Nil(t, resolution.DesiredClaim)
	})

	t.Run("account not found should set bindingState missing", func(t *testing.T) {
		resolution := aem.Resolve(context.Background(), createExport(), nil)

		assert.Equal(t, domain.AccountBindingStateMissing, resolution.BindingState)
	})

	t.Run("account not found should set adoptionState missing", func(t *testing.T) {
		resolution := aem.Resolve(context.Background(), createExport(), nil)

		assert.Equal(t, domain.AccountAdoptionStateMissing, resolution.AdoptionState)
	})

	t.Run("account found should set accountID", func(t *testing.T) {
		account := createAccount()
		account.SetLabel(v1alpha1.AccountLabelAccountID, "ACC1")
		resolution := aem.Resolve(context.Background(), createExport(), account)

		assert.Equal(t, "ACC1", resolution.AccountID)
	})

	t.Run("account found should set accountID", func(t *testing.T) {
		account := createAccount()
		account.SetLabel(v1alpha1.AccountLabelAccountID, "ACC1")
		resolution := aem.Resolve(context.Background(), createExport(), account)

		assert.Equal(t, "ACC1", resolution.AccountID)
	})

	t.Run("No boundAccountID should set bindingState missing", func(t *testing.T) {
		resolution := aem.Resolve(context.Background(), createExport(), createAccount())

		assert.Equal(t, domain.AccountBindingStateMissing, resolution.BindingState)
		assert.Empty(t, resolution.BoundAccountID)
	})

	t.Run("matching boundAccountID should set bindingState bound", func(t *testing.T) {
		accountExport := createExport()
		accountExport.SetLabel(v1alpha1.AccountExportLabelAccountID, "ACC1")
		account := createAccount()
		account.SetLabel(v1alpha1.AccountLabelAccountID, "ACC1")

		resolution := aem.Resolve(context.Background(), accountExport, account)

		assert.Equal(t, domain.AccountBindingStateBound, resolution.BindingState)
		assert.Equal(t, "ACC1", resolution.BoundAccountID)
	})

	t.Run("non matching boundAccountID should set bindingState conflict", func(t *testing.T) {
		accountExport := createExport()
		accountExport.SetLabel(v1alpha1.AccountExportLabelAccountID, "ACC2")
		account := createAccount()
		account.SetLabel(v1alpha1.AccountLabelAccountID, "ACC1")

		resolution := aem.Resolve(context.Background(), accountExport, account)

		assert.Equal(t, domain.AccountBindingStateConflict, resolution.BindingState)
		assert.Equal(t, "ACC2", resolution.BoundAccountID)
	})

	t.Run("adoption not found should set adoptionState missing", func(t *testing.T) {
		accountExport := createExport()
		account := createAccount()

		resolution := aem.Resolve(context.Background(), accountExport, account)

		assert.Equal(t, domain.AccountAdoptionStateMissing, resolution.AdoptionState)
		assert.Error(t, resolution.AdoptionError)
	})

	t.Run("adoption found should set adoptionState adopted", func(t *testing.T) {
		accountExport := createExport()
		accountExport.Generation = 2

		account := createAccount()
		desiredGen := int64(2)
		account.Status.Adoptions.Exports = []v1alpha1.AccountAdoption{
			{
				Name: "Rule1",
				UID:  accountExport.UID,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionTrue,
					DesiredClaimObservedGeneration: &desiredGen,
				},
			},
		}

		resolution := aem.Resolve(context.Background(), accountExport, account)

		assert.Equal(t, domain.AccountAdoptionStateAdopted, resolution.AdoptionState)
		assert.NoError(t, resolution.AdoptionError)
	})

	t.Run("adoption wrong generation should set adoptionState notAdopted", func(t *testing.T) {
		accountExport := createExport()
		accountExport.Generation = 2

		account := createAccount()
		desiredGen := int64(1)
		account.Status.Adoptions.Exports = []v1alpha1.AccountAdoption{
			{
				Name: "Rule1",
				UID:  accountExport.UID,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionTrue,
					DesiredClaimObservedGeneration: &desiredGen,
				},
			},
		}

		resolution := aem.Resolve(context.Background(), accountExport, account)

		assert.Equal(t, domain.AccountAdoptionStateNotAdopted, resolution.AdoptionState)
		assert.Error(t, resolution.AdoptionError)
		assert.Equal(t, "generation 2 has not been adopted yet", resolution.AdoptionError.Error())
	})

	t.Run("adoption error should set adoptionState notAdopted", func(t *testing.T) {
		accountExport := createExport()
		accountExport.Generation = 2

		account := createAccount()
		desiredGen := int64(2)
		account.Status.Adoptions.Exports = []v1alpha1.AccountAdoption{
			{
				Name: "Rule1",
				UID:  accountExport.UID,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionFalse,
					DesiredClaimObservedGeneration: &desiredGen,
					Reason:                         "ConflictReason",
					Message:                        "conflict with",
				},
			},
		}

		resolution := aem.Resolve(context.Background(), accountExport, account)

		assert.Equal(t, domain.AccountAdoptionStateNotAdopted, resolution.AdoptionState)
		assert.Error(t, resolution.AdoptionError)
		assert.Equal(t, "ConflictReason: conflict with", resolution.AdoptionError.Error())
	})
}
