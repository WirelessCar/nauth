package testutil

import (
	"fmt"

	"github.com/nats-io/nkeys"
)

var (
	NatsTestOperatorA = CreateNatsTestOperatorFromValues(
		"SOACDATEBXKVKM32VHLGU4574XUZNUOZ6GVD45J7HVC4D74KJWCR52PZYY", "OAZQ4BE3XWWQZXMZNAJUXUL33QR3JEMGNYUOVRTOSIHZS24GR5OB7GCQ",
		"SOAHBKSH6IERVYYRYFF3XD7L6N3FJKQGDK3VVNO5HYVS3HEZIJZTKG32ZI", "ODSQ3FLLTVD4O3K4BAXXOFPURAFMOSNPB74DBTLPDD5NAXSBOIC6M3M5",
	)
	NatsTestAccountA = CreateNatsTestAccountFromValues(
		"SAAKZTYWR5QQQJOQ3HQMYPPDH2LIDGFS6USLW3P4K47HZEHR6AKVTJYPGQ", "ABVIZMZGIFNQNOEMNHPGQLSL5NW7SUMTPBWT3HD65DQDNDKOU4XGBTL4",
		"SAABWTQAYJ7BEI65HLX5F4GSWHZL6DH6UQOGWYCEV5OQ63XQT2BNQERQKY", "ADZUBQ2ZAWRNON6VNSZHGLOJ5SOYE6GY2YDBQV3I2ZBQIWWP5YBR3KWT",
	)
)

type NatsTestAccount struct {
	Root NatsTestAccountKey
	Sign NatsTestAccountKey
}

func (k NatsTestAccount) AccountID() string {
	return k.Root.PublicKey
}

func (k NatsTestAccount) String() string {
	return fmt.Sprintf("AcRoot: %s, AcSign: %s", k.Root, k.Sign)
}

type NatsTestOperator struct {
	Root NatsTestOperatorKey
	Sign NatsTestOperatorKey
}

func (k NatsTestOperator) String() string {
	return fmt.Sprintf("OpRoot: %s, OpSign: %s", k.Root, k.Sign)
}

type NatsTestKey struct {
	Key       nkeys.KeyPair
	PublicKey string
	Seed      []byte
}

func (k NatsTestKey) String() string {
	return fmt.Sprintf("[S:%s, P:%s]", k.Seed, k.PublicKey)
}

type NatsTestAccountKey NatsTestKey

func (k NatsTestAccountKey) String() string {
	return fmt.Sprintf("Ac[S:%s, P:%s]", k.Seed, k.PublicKey)
}

type NatsTestOperatorKey NatsTestKey

func (k NatsTestOperatorKey) String() string {
	return fmt.Sprintf("Op[S:%s, P:%s]", k.Seed, k.PublicKey)
}

func CreateNatsTestAccount() NatsTestAccount {
	return NatsTestAccount{
		Root: CreateNatsTestAccountKey(),
		Sign: CreateNatsTestAccountKey(),
	}
}

func CreateNatsTestAccountFromValues(rootSeed, rootPub, signSeed, signPub string) NatsTestAccount {
	return NatsTestAccount{
		Root: NatsTestAccountKey(CreateNatsTestKeyFromValues(rootSeed, rootPub)),
		Sign: NatsTestAccountKey(CreateNatsTestKeyFromValues(signSeed, signPub)),
	}
}

func CreateNatsTestOperator() NatsTestOperator {
	return NatsTestOperator{
		Root: CreateNatsTestOperatorKey(),
		Sign: CreateNatsTestOperatorKey(),
	}
}

func CreateNatsTestOperatorFromValues(rootSeed, rootPub, signSeed, signPub string) NatsTestOperator {
	return NatsTestOperator{
		Root: NatsTestOperatorKey(CreateNatsTestKeyFromValues(rootSeed, rootPub)),
		Sign: NatsTestOperatorKey(CreateNatsTestKeyFromValues(signSeed, signPub)),
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

func CreateNatsTestKeyFromSeed(seed string) NatsTestKey {
	operator, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		panic(fmt.Errorf("failed to create key from seed %q: %w", seed, err))
	}
	pubKey, err := operator.PublicKey()
	if err != nil {
		panic(fmt.Errorf("failed to get public key from key created from seed %q: %w", seed, err))
	}
	return NatsTestKey{
		Key:       operator,
		PublicKey: pubKey,
		Seed:      []byte(seed),
	}
}

func CreateNatsTestKeyFromValues(seed, pub string) NatsTestKey {
	result := CreateNatsTestKeyFromSeed(seed)
	if result.PublicKey != pub {
		panic(fmt.Errorf("unexpected public key generated from seed %q: got %q, want %q", seed, result.PublicKey, pub))
	}
	return result
}

func CreateNatsTestUserKey() NatsTestKey {
	user, _ := nkeys.CreateUser()
	pubKey, _ := user.PublicKey()
	seed, _ := user.Seed()
	return NatsTestKey{
		Key:       user,
		PublicKey: pubKey,
		Seed:      seed,
	}
}

func AnyNatsTestAccountID() string {
	return CreateNatsTestAccountKey().PublicKey
}
