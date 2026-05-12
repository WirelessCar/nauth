package testutil

import (
	"fmt"

	"github.com/nats-io/nkeys"
)

type NatsTestAccount struct {
	Root NatsTestAccountKey
	Sign NatsTestAccountKey
}

func (k NatsTestAccount) AccountID() string {
	return k.Root.PublicKey
}

type NatsTestOperator struct {
	Root NatsTestOperatorKey
	Sign NatsTestOperatorKey
}

type NatsTestKey struct {
	Key       nkeys.KeyPair
	PublicKey string
	Seed      []byte
}

type NatsTestAccountKey NatsTestKey
type NatsTestOperatorKey NatsTestKey
type NatsTestUserKey NatsTestKey

func CreateNatsTestAccount() NatsTestAccount {
	return NatsTestAccount{
		Root: CreateNatsTestAccountKey(),
		Sign: CreateNatsTestAccountKey(),
	}
}

func CreateNatsTestOperator() NatsTestOperator {
	return NatsTestOperator{
		Root: CreateNatsTestOperatorKey(),
		Sign: CreateNatsTestOperatorKey(),
	}
}

func CreateNatsTestAccountKey() NatsTestAccountKey {
	account, _ := nkeys.CreateAccount()
	pubKey, _ := account.PublicKey()
	seed, _ := account.Seed()
	return NatsTestAccountKey{
		Key:       account,
		PublicKey: pubKey,
		Seed:      seed,
	}
}

func CreateNatsTestOperatorKey() NatsTestOperatorKey {
	operator, _ := nkeys.CreateOperator()
	pubKey, _ := operator.PublicKey()
	seed, _ := operator.Seed()
	return NatsTestOperatorKey{
		Key:       operator,
		PublicKey: pubKey,
		Seed:      seed,
	}
}

func CreateNatsTestOperatorKeyFromSeed(seed string) NatsTestOperatorKey {
	operator, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		panic(fmt.Errorf("failed to create operator key from seed %q: %w", seed, err))
	}
	pubKey, err := operator.PublicKey()
	if err != nil {
		panic(fmt.Errorf("failed to get public key from operator key created from seed %q: %w", seed, err))
	}
	return NatsTestOperatorKey{
		Key:       operator,
		PublicKey: pubKey,
		Seed:      []byte(seed),
	}
}

func CreateNatsTestUserKey() NatsTestUserKey {
	user, _ := nkeys.CreateUser()
	pubKey, _ := user.PublicKey()
	seed, _ := user.Seed()
	return NatsTestUserKey{
		Key:       user,
		PublicKey: pubKey,
		Seed:      seed,
	}
}

func AnyNatsTestAccountID() string {
	return CreateNatsTestAccountKey().PublicKey
}
