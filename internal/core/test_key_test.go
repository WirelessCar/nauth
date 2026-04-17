package core

import (
	"fmt"

	"github.com/nats-io/nkeys"
)

type testKeys struct {
	OpRoot testKey
	OpSign testKey
	AcRoot testKey
	AcSign testKey
}

func (k testKeys) String() string {
	return fmt.Sprintf("OpRoot: %s, OpSign: %s, AcRoot: %s, AcSign: %s", k.OpRoot, k.OpSign, k.AcRoot, k.AcSign)
}

type testKey struct {
	KeyPair   nkeys.KeyPair
	Seed      string
	PublicKey string
}

func (k testKey) String() string {
	return fmt.Sprintf("[S:%s, P:%s]", k.Seed, k.PublicKey)
}

func testKeys1() testKeys {
	return testKeys{
		OpRoot: testKeyFixed("SOACDATEBXKVKM32VHLGU4574XUZNUOZ6GVD45J7HVC4D74KJWCR52PZYY", "OAZQ4BE3XWWQZXMZNAJUXUL33QR3JEMGNYUOVRTOSIHZS24GR5OB7GCQ"),
		OpSign: testKeyFixed("SOAHBKSH6IERVYYRYFF3XD7L6N3FJKQGDK3VVNO5HYVS3HEZIJZTKG32ZI", "ODSQ3FLLTVD4O3K4BAXXOFPURAFMOSNPB74DBTLPDD5NAXSBOIC6M3M5"),
		AcRoot: testKeyFixed("SAAKZTYWR5QQQJOQ3HQMYPPDH2LIDGFS6USLW3P4K47HZEHR6AKVTJYPGQ", "ABVIZMZGIFNQNOEMNHPGQLSL5NW7SUMTPBWT3HD65DQDNDKOU4XGBTL4"),
		AcSign: testKeyFixed("SAABWTQAYJ7BEI65HLX5F4GSWHZL6DH6UQOGWYCEV5OQ63XQT2BNQERQKY", "ADZUBQ2ZAWRNON6VNSZHGLOJ5SOYE6GY2YDBQV3I2ZBQIWWP5YBR3KWT"),
	}
}

func testKeyFixed(seed, optExpPub string) testKey {
	key, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		panic(fmt.Sprintf("failed to generate nkey.KeyPair from seed %q: %s", seed, err.Error()))
	}
	pub, err := key.PublicKey()
	if err != nil {
		panic(fmt.Sprintf("failed to extract public key from seed %q: %s", seed, err.Error()))
	}
	if optExpPub != "" && optExpPub != pub {
		panic(fmt.Sprintf("unexpected public key generated from seed %q: got %q, want %q", seed, pub, optExpPub))
	}
	return testKey{
		KeyPair:   key,
		Seed:      seed,
		PublicKey: pub,
	}
}
