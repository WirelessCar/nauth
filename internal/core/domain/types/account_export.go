package types

import (
	"fmt"
	"time"

	k8s "k8s.io/apimachinery/pkg/types"
)

type AccountExports []*AccountExport

type AccountExport struct {
	Name                 string
	Subject              Subject
	Type                 ExportType
	TokenReq             bool
	Revocation           RevocationList
	ResponseType         ResponseType
	ResponseThreshold    time.Duration
	Latency              *ServiceLatency
	AccountTokenPosition uint
	Advertise            bool
	AllowTrace           bool
}

type AccountImports []*AccountImport

type AccountImport struct {
	AccountRef   k8s.NamespacedName
	Name         string
	Subject      Subject
	Account      string
	LocalSubject Subject
	Type         ExportType
	Share        bool
	AllowTrace   bool
}

type Subject string
type ExportType string

const (
	// Stream defines the type field value for a stream "stream"
	Stream ExportType = "stream"
	// Service defines the type field value for a service "service"
	Service ExportType = "service"
)

// ToInt converts the ExportType to an int value: Stream=1, Service=2
func (e ExportType) ToInt() (int, error) {
	switch e {
	case Stream:
		return 1, nil
	case Service:
		return 2, nil
	default:
		return 1, fmt.Errorf("unknown ExportType %q", e) // FIXME: Should we really default to 1?
	}
}

type ResponseType string

const (
	// ResponseTypeSingleton is used for a service that sends a single response only
	ResponseTypeSingleton = "Singleton"

	// ResponseTypeStream is used for a service that will send multiple responses
	ResponseTypeStream = "Stream"

	// ResponseTypeChunked is used for a service that sends a single response in chunks (so not quite a stream)
	ResponseTypeChunked = "Chunked"
)

type RevocationList map[string]int64

type ServiceLatency struct {
	Sampling SamplingRate
	Results  Subject
}

type SamplingRate int
