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

type AccountLimits struct {
	Imports         *int64 `json:"imports,omitempty"`
	Exports         *int64 `json:"exports,omitempty"`
	WildcardExports *bool  `json:"wildcards,omitempty"`
	DisallowBearer  *bool  `json:"disallow_bearer,omitempty"`
	Conn            *int64 `json:"conn,omitempty"`
	LeafNodeConn    *int64 `json:"leaf,omitempty"`
}

type JetStreamLimits struct {
	MemoryStorage        *int64 `json:"memStorage,omitempty"`
	DiskStorage          *int64 `json:"diskStorage,omitempty"`
	Streams              *int64 `json:"streams,omitempty"`
	Consumer             *int64 `json:"consumer,omitempty"`
	MaxAckPending        *int64 `json:"maxAckPending,omitempty"`
	MemoryMaxStreamBytes *int64 `json:"memMaxStreamBytes,omitempty"`
	DiskMaxStreamBytes   *int64 `json:"diskMaxStreamBytes,omitempty"`
	MaxBytesRequired     *bool  `json:"maxBytesRequired,omitempty"`
}

type NatsLimits struct {
	Subs    *int64 `json:"subs,omitempty"`
	Data    *int64 `json:"data,omitempty"`
	Payload *int64 `json:"payload,omitempty"`
}

type ImportGroup struct {
	Name    string  `json:"name,omitempty"`
	Imports Imports `json:"imports,omitempty"`
}

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
	AccountID        AccountID            `json:"accountId,omitempty"`
	DisplayName      string               `json:"displayName,omitempty"`
	AccountLimits    *AccountLimits       `json:"accountLimits,omitempty"`
	JetStreamEnabled *bool                `json:"jetStreamEnabled,omitempty"`
	JetStreamLimits  *JetStreamLimits     `json:"jetStreamLimits,omitempty"`
	NatsLimits       *NatsLimits          `json:"natsLimits,omitempty"`
	SigningKeys      v1alpha1.SigningKeys `json:"signingKeys,omitempty"` // TODO: Migrate to domain SigningKeys
	Exports          v1alpha1.Exports     `json:"exports,omitempty"`     // TODO: Migrate to domain Exports
	Imports          Imports              `json:"imports,omitempty"`
}
