package v1alpha1

import (
	"encoding/json"
	"strings"
	"time"
)

const MaxInfoLength = 8 * 1024

// ExportType defines the type of import/export.
// +kubebuilder:validation:Enum=stream;service
// +kubebuilder:default=stream
type ExportType string

const (
	// Stream defines the type field value for a stream "stream"
	Stream ExportType = "stream"
	// Service defines the type field value for a service "service"
	Service ExportType = "service"
)

// ToInt converts the ExportType to an int value: Stream=1, Service=2
func (e ExportType) ToInt() int {
	switch e {
	case Stream:
		return 1
	case Service:
		return 2
	default:
		return 1
	}
}

type RenamingSubject Subject

// ResponseType is used to store an export response type
// +kubebuilder:validation:Enum=Singleton;Stream;Chunked
type ResponseType string

const (
	// ResponseTypeSingleton is used for a service that sends a single response only
	ResponseTypeSingleton = "Singleton"

	// ResponseTypeStream is used for a service that will send multiple responses
	ResponseTypeStream = "Stream"

	// ResponseTypeChunked is used for a service that sends a single response in chunks (so not quite a stream)
	ResponseTypeChunked = "Chunked"
)

type Info struct {
	Description string `json:"description,omitempty"`
	InfoURL     string `json:"info_url,omitempty"`
}

// Subject is a string that represents a NATS subject
type Subject string

// TimeRange is used to represent a start and end time
type TimeRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type NatsLimits struct {
	// +optional
	// +kubebuilder:default=-1
	Subs *int64 `json:"subs,omitempty"` // Max number of subscriptions
	// +optional
	// +kubebuilder:default=-1
	Data *int64 `json:"data,omitempty"` // Max number of bytes
	// +optional
	// +kubebuilder:default=-1
	Payload *int64 `json:"payload,omitempty"` // Max message payload
}

// Permission defines allow/deny subjects
type Permission struct {
	// +optional
	Allow StringList `json:"allow,omitempty"`
	// +optional
	Deny StringList `json:"deny,omitempty"`
}

func (p *Permission) Empty() bool {
	return len(p.Allow) == 0 && len(p.Deny) == 0
}

// ResponsePermission can be used to allow responses to any reply subject
// that is received on a valid subscription.
type ResponsePermission struct {
	// +optional
	MaxMsgs int `json:"max"`
	// +optional
	Expires time.Duration `json:"ttl"`
}

// Permissions are used to restrict subject access, either on a user or for everyone on a server by default
type Permissions struct {
	// +optional
	Pub Permission `json:"pub,omitempty"`
	// +optional
	Sub Permission `json:"sub,omitempty"`
	// +optional
	Resp *ResponsePermission `json:"resp,omitempty"`
}

// StringList is a wrapper for an array of strings
type StringList []string

// Contains returns true if the list contains the string
func (u *StringList) Contains(p string) bool {
	for _, t := range *u {
		if t == p {
			return true
		}
	}
	return false
}

// Add appends 1 or more strings to a list
func (u *StringList) Add(p ...string) {
	for _, v := range p {
		if !u.Contains(v) && v != "" {
			*u = append(*u, v)
		}
	}
}

// Remove removes 1 or more strings from a list
func (u *StringList) Remove(p ...string) {
	for _, v := range p {
		for i, t := range *u {
			if t == v {
				a := *u
				*u = append(a[:i], a[i+1:]...)
				break
			}
		}
	}
}

// TagList is a unique array of lower case strings
// All tag list methods lower case the strings in the arguments
type TagList []string

// Contains returns true if the list contains the tags
func (u *TagList) Contains(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	for _, t := range *u {
		if t == p {
			return true
		}
	}
	return false
}

// Add appends 1 or more tags to a list
func (u *TagList) Add(p ...string) {
	for _, v := range p {
		v = strings.ToLower(strings.TrimSpace(v))
		if !u.Contains(v) && v != "" {
			*u = append(*u, v)
		}
	}
}

// Remove removes 1 or more tags from a list
func (u *TagList) Remove(p ...string) {
	for _, v := range p {
		v = strings.ToLower(strings.TrimSpace(v))
		for i, t := range *u {
			if t == v {
				a := *u
				*u = append(a[:i], a[i+1:]...)
				break
			}
		}
	}
}

type CIDRList TagList

func (c *CIDRList) Contains(p string) bool {
	return (*TagList)(c).Contains(p)
}

func (c *CIDRList) Add(p ...string) {
	(*TagList)(c).Add(p...)
}

func (c *CIDRList) Remove(p ...string) {
	(*TagList)(c).Remove(p...)
}

func (c *CIDRList) Set(values string) {
	*c = CIDRList{}
	c.Add(strings.Split(strings.ToLower(values), ",")...)
}

func (c *CIDRList) UnmarshalJSON(body []byte) (err error) {
	// parse either as array of strings or comma separate list
	var request []string
	var list string
	if err := json.Unmarshal(body, &request); err == nil {
		*c = request
		return nil
	} else if err := json.Unmarshal(body, &list); err == nil {
		c.Set(list)
		return nil
	} else {
		return err
	}
}
