package nauth

import "github.com/WirelessCar/nauth/api/v1alpha1"

type AccountID string
type Subject string

type ExportType string

const (
	ExportTypeUnknown ExportType = "unknown"
	ExportTypeStream  ExportType = "stream"
	ExportTypeService ExportType = "service"
)

type Imports []*Import

type Import struct {
	AccountID    AccountID  `json:"accountId,omitempty"`
	Name         string     `json:"name,omitempty"`
	Subject      Subject    `json:"subject,omitempty"`
	LocalSubject Subject    `json:"localSubject,omitempty"`
	Type         ExportType `json:"type,omitempty"`
	Share        bool       `json:"share,omitempty"`
	AllowTrace   bool       `json:"allowTrace,omitempty"`
}

type AccountClaims struct {
	AccountID        AccountID                 `json:"accountId,omitempty"`
	DisplayName      string                    `json:"displayName,omitempty"`
	AccountLimits    *v1alpha1.AccountLimits   `json:"accountLimits,omitempty"` // TODO: Migrate to domain AccountLimits
	JetStreamEnabled *bool                     `json:"jetStreamEnabled,omitempty"`
	JetStreamLimits  *v1alpha1.JetStreamLimits `json:"jetStreamLimits,omitempty"` // TODO: Migrate to domain JetStreamLimits
	NatsLimits       *v1alpha1.NatsLimits      `json:"natsLimits,omitempty"`      // TODO: Migrate to domain NatsLimits
	SigningKeys      v1alpha1.SigningKeys `json:"signingKeys,omitempty"` // TODO: Migrate to domain SigningKeys
	Exports          v1alpha1.Exports     `json:"exports,omitempty"`     // TODO: Migrate to domain Exports
	Imports          Imports              `json:"imports,omitempty"`
}
