package outbound

import (
	"github.com/WirelessCar/nauth/internal/domain"
)

type NatsClient interface {
	Connect(natsURL string, userCreds domain.NatsUserCreds) (NatsConnection, error)
}

type NatsConnection interface {
	Disconnect()
	EnsureConnected() error
	LookupAccountJWT(accountID string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}
