package nauth

import (
	"fmt"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
)

type AccountRequest struct {
	AccountRef       domain.NamespacedName `json:"accountRef,omitempty"`
	AccountID        AccountID             `json:"accountId,omitempty"`
	ClaimsHash       string                `json:"claimsHash,omitempty"`
	DisplayName      string                `json:"displayName,omitempty"`
	ClusterRef       *ClusterRef           `json:"clusterRef,omitempty"`
	AccountLimits    *AccountLimits        `json:"accountLimits,omitempty"`
	JetStreamEnabled *bool                 `json:"jetStreamEnabled,omitempty"`
	JetStreamLimits  *JetStreamLimits      `json:"jetStreamLimits,omitempty"`
	NatsLimits       *NatsLimits           `json:"natsLimits,omitempty"`
	ExportGroups     ExportGroups          `json:"exportGroups,omitempty"`
	ImportGroups     ImportGroups          `json:"importGroups,omitempty"`
}

func (r AccountRequest) Validate() error {
	if err := r.AccountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference: %w", err)
	}

	if r.ClusterRef != nil {
		if err := r.ClusterRef.Validate(); err != nil {
			return fmt.Errorf("invalid cluster reference: %w", err)
		}
	}

	exportGroupNames := make(map[Ref]struct{})
	for _, exportGroup := range r.ExportGroups {
		if exportGroup.Ref == "" {
			return fmt.Errorf("export group ref is required")
		}
		if _, exists := exportGroupNames[exportGroup.Ref]; exists {
			return fmt.Errorf("duplicate export group ref: %q", exportGroup.Ref)
		}
		exportGroupNames[exportGroup.Ref] = struct{}{}
	}

	importGroupNames := make(map[Ref]struct{})
	for _, importGroup := range r.ImportGroups {
		if importGroup.Ref == "" {
			return fmt.Errorf("import group ref is required")
		}
		if _, exists := importGroupNames[importGroup.Ref]; exists {
			return fmt.Errorf("duplicate import group ref: %q", importGroup.Ref)
		}
		importGroupNames[importGroup.Ref] = struct{}{}
	}
	return nil
}

type AccountReference struct {
	AccountRef     domain.NamespacedName
	AccountID      AccountID
	NatsClusterRef *ClusterRef
}

type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *AccountClaims
	ClaimsHash      string
	Adoptions       *AccountAdoptions
}

type Ref string

type AccountID string

type PublicKey string
type Subject string

type ExportType string

const (
	ExportTypeUnknown ExportType = "unknown"
	ExportTypeStream  ExportType = "stream"
	ExportTypeService ExportType = "service"
)

type RevocationList map[string]int64

type ResponseType string

const (
	ResponseTypeSingleton ResponseType = "singleton"
	ResponseTypeStream    ResponseType = "stream"
	ResponseTypeChunked   ResponseType = "chunked"
)

type SamplingRate int

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

type SigningKeys []*SigningKey

type SigningKey struct {
	Key string `json:"key,omitempty"`
	// TODO: [#140] Add signing key scope
}

type ImportGroups []*ImportGroup

type ImportGroup struct {
	Ref      Ref     `json:"ref"`
	Required bool    `json:"required,omitempty"`
	Name     string  `json:"name,omitempty"`
	Imports  Imports `json:"imports,omitempty"`
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

type ExportGroups []*ExportGroup
type ExportGroup struct {
	Ref      Ref     `json:"ref"`
	Required bool    `json:"required,omitempty"`
	Name     string  `json:"name,omitempty"`
	Exports  Exports `json:"exports"`
}

type Exports []*Export

type Export struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +required
	Subject Subject `json:"subject,omitempty"`
	// +required
	Type ExportType `json:"type,omitempty"`
	// +optional
	TokenReq bool `json:"tokenReq,omitempty"`
	// +optional
	Revocations RevocationList `json:"revocations,omitempty"`
	// +optional
	ResponseType ResponseType `json:"responseType,omitempty"`
	// +optional
	ResponseThreshold time.Duration `json:"responseThreshold,omitempty"`
	// +optional
	Latency *ServiceLatency `json:"serviceLatency,omitempty"`
	// +optional
	AccountTokenPosition uint `json:"accountTokenPosition,omitempty"`
	// +optional
	Advertise bool `json:"advertise,omitempty"`
	// +optional
	AllowTrace bool `json:"allowTrace,omitempty"`
}
type ServiceLatency struct {
	Sampling SamplingRate `json:"sampling"`
	Results  Subject      `json:"results"`
}

type AccountClaims struct {
	AccountID        AccountID        `json:"accountId,omitempty"`
	DisplayName      string           `json:"displayName,omitempty"`
	AccountLimits    *AccountLimits   `json:"accountLimits,omitempty"`
	JetStreamEnabled *bool            `json:"jetStreamEnabled,omitempty"`
	JetStreamLimits  *JetStreamLimits `json:"jetStreamLimits,omitempty"`
	NatsLimits       *NatsLimits      `json:"natsLimits,omitempty"`
	SigningKeys      SigningKeys      `json:"signingKeys,omitempty"`
	Exports          Exports          `json:"exports,omitempty"`
	Imports          Imports          `json:"imports,omitempty"`
}

type AccountAdoptions struct {
	Exports *AdoptionResults `json:"exports,omitempty"`
	Imports *AdoptionResults `json:"imports,omitempty"`
}

func NewAccountAdoptions() *AccountAdoptions {
	return &AccountAdoptions{
		Exports: &AdoptionResults{},
		Imports: &AdoptionResults{},
	}
}

type AdoptionResults map[Ref]AdoptionResult

func (r AdoptionResults) Add(result AdoptionResult) error {
	if _, exists := r[result.Ref]; exists {
		return fmt.Errorf("adoption result for Ref %q already exists", result.Ref)
	}
	r[result.Ref] = result
	return nil
}

func (r AdoptionResults) Get(ref Ref) *AdoptionResult {
	result, found := r[ref]
	if !found {
		return nil
	}
	return &result
}

// AdoptionFailure must be TitleCased to comply with k8s metav1.StatusReason
type AdoptionFailure string

const (
	AdoptionFailureConflict AdoptionFailure = "Conflict"
)

type AdoptionResult struct {
	Ref     Ref             `json:"-"`
	Failure AdoptionFailure `json:"failure,omitempty"`
	Message string          `json:"message,omitempty"`
}

func (a *AdoptionResult) IsSuccessful() bool {
	return a.Failure == ""
}
