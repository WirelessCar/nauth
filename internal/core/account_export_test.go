package core

import (
	"context"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/assert"
)

func Test_mapToJwtExport(t *testing.T) {
	responseThreshold := time.Duration(10)

	have := nauth.ExportRule{
		Name:              "Name",
		Subject:           "Subject.*",
		Type:              nauth.ExportTypeService,
		ResponseType:      "Singleton",
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

	got := mapToJwtExport(have)
	assert.Equalf(t, want, got, "should be equal")
}

func Test_mapToJwtExport_requiredFields(t *testing.T) {
	have := nauth.ExportRule{
		Subject: "Subject",
		Type:    nauth.ExportTypeService,
	}
	want := &jwt.Export{
		Subject: "Subject",
		Type:    jwt.Service,
	}

	got := mapToJwtExport(have)
	assert.Equalf(t, want, got, "should be equal")
}

func TestAccountExportManager_ValidateRules(t *testing.T) {
	responseThreshold := 50 * time.Millisecond

	tests := []struct {
		name    string
		rules   nauth.ExportRules
		wantErr bool
	}{
		{
			name: "valid_rules",
			rules: nauth.ExportRules{
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
			rules: nauth.ExportRules{
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
			err := manager.ValidateRules(context.Background(), tt.rules)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_mapExportType(t *testing.T) {
	type args struct {
		t nauth.ExportType
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
