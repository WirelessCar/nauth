package nauth

import "time"

type ExportRules []ExportRule

type ExportRule struct {
	// +optional
	Name string
	// +required
	Subject Subject
	// +required
	Type ExportType
	// +optional
	ResponseType ResponseType
	// +optional
	ResponseThreshold time.Duration
	// +optional
	Latency *ServiceLatency
	// +optional
	AccountTokenPosition uint
	// +optional
	Advertise bool
	// +optional
	AllowTrace bool
}
