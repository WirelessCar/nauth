package outbound

import (
	"github.com/WirelessCar/nauth/internal/domain"
)

type NatsSysClient interface {
	Connect(natsURL string, userCreds domain.NatsUserCreds) (NatsSysConnection, error)
}

type NatsSysConnection interface {
	Disconnect()
	EnsureConnected() error
	VerifySystemAccountAccess() error
	LookupAccountJWT(accountID string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}
