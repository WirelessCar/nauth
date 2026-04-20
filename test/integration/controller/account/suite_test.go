package account_test

import (
	"os"
	"testing"

	envtestkit "github.com/WirelessCar/nauth/test/integration/testkit/envtest"
)

var sharedEnv *envtestkit.Environment

func TestMain(m *testing.M) {
	var err error
	sharedEnv, err = envtestkit.Start()
	if err != nil {
		panic(err)
	}

	code := m.Run()

	if err := sharedEnv.Stop(); err != nil {
		panic(err)
	}

	os.Exit(code)
}
