package core

import (
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_mapToJwtExport(t *testing.T) {
	responseThreshold := time.Duration(10)

	have := nauth.Export{
		Name:              "Name",
		Subject:           "Subject.*",
		Type:              nauth.ExportTypeService,
		ResponseType:      nauth.ResponseTypeSingleton,
		ResponseThreshold: responseThreshold,
		Latency: &nauth.ServiceLatency{
			Sampling: 20,
			Results:  "Results",
		},
		AccountTokenPosition: 2,
		Advertise:            true,
		AllowTrace:           true,
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
		AccountTokenPosition: 2,
		Advertise:            true,
		AllowTrace:           true,
		Info:                 jwt.Info{},
	}

	got, err := toJWTExport(have)
	assert.NoError(t, err)
	assert.Equalf(t, want, got, "should be equal")
}

func Test_mapToJwtExport_requiredFields(t *testing.T) {
	have := nauth.Export{
		Subject: "Subject",
		Type:    nauth.ExportTypeService,
	}
	want := &jwt.Export{
		Subject: "Subject",
		Type:    jwt.Service,
	}

	got, err := toJWTExport(have)
	require.NoError(t, err)
	assert.Equalf(t, want, got, "should be equal")
}

func TestAccountExportManager_ValidateRules(t *testing.T) {
	responseThreshold := 50 * time.Millisecond

	tests := []struct {
		name    string
		exports nauth.Exports
		wantErr bool
	}{
		{
			name: "valid_rules",
			exports: nauth.Exports{
				{
					Name:              "export1",
					Subject:           "my.subject",
					Type:              nauth.ExportTypeService,
					ResponseType:      "Singleton",
					ResponseThreshold: responseThreshold,
					Latency: &nauth.ServiceLatency{
						Sampling: 100,
						Results:  "results.subject",
					},
					Advertise:  true,
					AllowTrace: true,
				},
				{
					Name:    "export2",
					Subject: "stream.subject",
					Type:    nauth.ExportTypeStream,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_subject",
			exports: nauth.Exports{
				{
					Name:    "invalid",
					Subject: ".",
					Type:    nauth.ExportTypeStream,
				},
			},
			wantErr: true,
		},
	}

	manager := NewAccountExportManager()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateExports(tt.exports)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
