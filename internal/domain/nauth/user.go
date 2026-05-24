package nauth

import (
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UserRef domain.NamespacedName

// UserRequest carries all inputs needed to reconcile a User resource.
type UserRequest struct {
	// UserRef identifies the User resource being reconciled.
	UserRef domain.NamespacedName
	// AccountRef references the Account resource this user belongs to.
	AccountRef domain.NamespacedName
	// AccountID is the NATS public key of the account (used as IssuerAccount in the JWT).
	AccountID AccountID
	// DisplayName is the NATS display name for this user.
	DisplayName string
	// Permissions defines the pub/sub permissions for this user.
	Permissions *UserPermissions
	// Limits constrains when and where this user may connect.
	Limits *UserLimits
	// NatsLimits places caps on subscriptions, data, and payload.
	NatsLimits *NatsLimits
	// Owner is the controlling owner for resources created during reconciliation.
	// When set, created resources are garbage-collected when the owner is deleted.
	Owner metav1.Object
}

// UserResult is returned by UserManager.CreateOrUpdate upon success.
type UserResult struct {
	// UserPublicKey is the freshly generated NATS user public key (JWT subject).
	UserPublicKey string
	// AccountID is the NATS account public key this user belongs to.
	AccountID string
	// SignedBy is the public key of the key that signed this user's JWT.
	SignedBy string
}

// UserPermissions defines pub/sub subject access rules for a user.
type UserPermissions struct {
	Pub  UserSubjectPermission
	Sub  UserSubjectPermission
	Resp *UserResponsePermission
}

// UserSubjectPermission lists allowed and denied NATS subjects.
type UserSubjectPermission struct {
	Allow []string
	Deny  []string
}

// UserResponsePermission allows responses to any reply subject on a valid subscription.
type UserResponsePermission struct {
	MaxMsgs int
	Expires time.Duration
}

// UserLimits constrains when and from where a user may connect.
type UserLimits struct {
	// Src is a list of CIDR specifications restricting source IPs.
	Src []string
	// Times defines permitted connection time windows.
	Times []UserTimeRange
	// Locale is the IANA timezone name for evaluating Times.
	Locale string
}

// UserTimeRange is a start/end time window (HH:MM format).
type UserTimeRange struct {
	Start string
	End   string
}
