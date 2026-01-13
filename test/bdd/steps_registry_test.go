package bdd

import "github.com/cucumber/godog"

func InitializeScenario(sc *godog.ScenarioContext) {
	shared := &bddScenarioState{}
	RegisterCommonSteps(sc, shared)
	RegisterAccountControllerSteps(sc, &shared.account)
	RegisterUserControllerSteps(sc, &shared.user)
}
