package nats

type NatsClient interface {
	EnsureConnected(namespace string) error
	Disconnect()
	LookupAccountJWT(string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}
