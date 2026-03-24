package outbound

import "github.com/WirelessCar/nauth/internal/domain"

type NatsConnection interface {
	Disconnect()
	EnsureConnected() error
}

// NatsSysClient is used for connecting to a NATS SYS account
type NatsSysClient interface {
	Connect(natsURL string, userCreds domain.NatsUserCreds) (NatsSysConnection, error)
}

// NatsSysConnection represents a NATS connection bound to a SYS account
type NatsSysConnection interface {
	NatsConnection
	VerifySystemAccountAccess() error
	LookupAccountJWT(accountID string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}

// NatsAccountClient is used for connecting to a regular NATS account
type NatsAccountClient interface {
	Connect(natsURL string, userCreds domain.NatsUserCreds) (NatsAccountConnection, error)
}

// NatsAccountConnection represents a NATS connection bound to a regular (non-sys) account
type NatsAccountConnection interface {
	NatsConnection
	ListAccountStreams() ([]string, error)
}
