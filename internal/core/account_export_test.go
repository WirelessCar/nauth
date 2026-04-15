package core

import (
	"context"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/assert"
)

func Test_mapToJwtExport(t *testing.T) {
	responseThreshold := time.Duration(10)
	tokenPos := uint(3)
	advertise := true
	allowTrace := true

	have := v1alpha1.AccountExportRule{
		Name:              "Name",
		Subject:           "Subject",
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
		Subject:           "Subject",
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
	manager := NewAccountExportManager()

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

	claim, err := manager.CreateClaim(context.Background(), state)

	assert.NoError(t, err)
	assert.NotNil(t, claim)
	assert.Len(t, claim.Rules, 1)

	exportRule := claim.Rules[0]
	assert.Equal(t, "export1", exportRule.Name)
	assert.Equal(t, v1alpha1.Subject("mysubject"), exportRule.Subject)
	assert.Equal(t, v1alpha1.ExportType("service"), exportRule.Type)
}

func TestAccountExportManager_CreateClaim_conflict(t *testing.T) {
	manager := NewAccountExportManager()

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

	_, err := manager.CreateClaim(context.Background(), state)
	assert.Error(t, err, "should fail")
}
