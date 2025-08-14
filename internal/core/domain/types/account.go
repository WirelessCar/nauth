package types

import "github.com/nats-io/nkeys"

type Jwt string

type Account struct {
	AccountJwt    Jwt
	SigningKey    nkeys.KeyPair
	SystemAccount nkeys.KeyPair
}
