package bdd

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	opts := godog.Options{
		Format: "pretty",
		Paths:  []string{"controller/account_missing.feature", "controller/account_controller.feature", "controller/user_controller.feature"},
	}

	status := godog.TestSuite{
		Name:                "nauth-bdd",
		ScenarioInitializer: InitializeScenario,
		Options:             &opts,
	}.Run()

	if status != 0 {
		t.Fatalf("godog suite failed with status %d", status)
	}
}
