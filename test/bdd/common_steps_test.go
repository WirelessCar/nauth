package bdd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/cucumber/godog"
	"k8s.io/client-go/tools/record"
)

type bddScenarioState struct {
	account accountControllerState
	user    userControllerState
	active  string
}

func RegisterCommonSteps(sc *godog.ScenarioContext, shared *bddScenarioState) {
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		shared.active = ""
		if scenarioHasTag(scenario, "@account") {
			shared.active = "account"
			shared.account = accountControllerState{
				accountName:       "test-resource",
				accountNamespace:  "test-namespace",
				operatorNamespace: "nauth-account-system",
			}
		}
		if scenarioHasTag(scenario, "@user") {
			shared.active = "user"
			shared.user = userControllerState{
				userName:      "test-resource",
				userNamespace: "test-namespace",
			}
		}
		return ctx, nil
	})

	sc.Step(`^the operator version is "([^"]*)"$`, shared.operatorVersionIs)
	sc.Step(`^reconciliation completes without error$`, shared.expectNoError)
	sc.Step(`^no warning events are recorded$`, shared.expectNoWarningEvents)
	sc.Step(`^a warning event includes "([^"]*)"$`, shared.warningEventIncludes)
	sc.Step(`^a warning event includes the deletion error$`, shared.warningEventIncludesDeletionError)
	sc.Step(`^reconciliation returns an error$`, shared.expectError)
}

func scenarioHasTag(scenario *godog.Scenario, tag string) bool {
	if scenario == nil {
		return false
	}
	for _, scenarioTag := range scenario.Tags {
		if scenarioTag.Name == tag {
			return true
		}
	}
	return false
}

func (s *bddScenarioState) operatorVersionIs(version string) error {
	return os.Setenv(controller.EnvOperatorVersion, version)
}

func (s *bddScenarioState) activeRecorder() (*record.FakeRecorder, error) {
	switch s.active {
	case "account":
		if s.account.recorder == nil {
			return nil, errors.New("recorder is not initialized")
		}
		return s.account.recorder, nil
	case "user":
		if s.user.recorder == nil {
			return nil, errors.New("recorder is not initialized")
		}
		return s.user.recorder, nil
	default:
		return nil, errors.New("active scenario not set")
	}
}

func (s *bddScenarioState) activeReconcileErr() (error, error) {
	switch s.active {
	case "account":
		return s.account.reconcileErr, nil
	case "user":
		return s.user.reconcileErr, nil
	default:
		return nil, errors.New("active scenario not set")
	}
}

func (s *bddScenarioState) warningEventIncludes(expected string) error {
	recorder, err := s.activeRecorder()
	if err != nil {
		return err
	}
	select {
	case event := <-recorder.Events:
		if !contains(event, expected) {
			return fmt.Errorf("expected warning event to include %q, got %q", expected, event)
		}
		return nil
	default:
		return errors.New("expected warning event but none recorded")
	}
}

func (s *bddScenarioState) warningEventIncludesDeletionError() error {
	switch s.active {
	case "account":
		if s.account.deletionErrMsg == "" {
			return errors.New("deletion error message not set")
		}
		return s.warningEventIncludes(s.account.deletionErrMsg)
	case "user":
		if s.user.deletionErr == nil {
			return errors.New("deletion error not set")
		}
		return s.warningEventIncludes(s.user.deletionErr.Error())
	default:
		return errors.New("active scenario not set")
	}
}

func (s *bddScenarioState) expectNoError() error {
	err, errLookup := s.activeReconcileErr()
	if errLookup != nil {
		return errLookup
	}
	if err != nil {
		return fmt.Errorf("expected no error, got %v", err)
	}
	return nil
}

func (s *bddScenarioState) expectError() error {
	err, errLookup := s.activeReconcileErr()
	if errLookup != nil {
		return errLookup
	}
	if err == nil {
		return errors.New("expected an error but got none")
	}
	return nil
}

func (s *bddScenarioState) expectNoWarningEvents() error {
	recorder, err := s.activeRecorder()
	if err != nil {
		return err
	}
	select {
	case event := <-recorder.Events:
		return fmt.Errorf("unexpected event recorded: %s", event)
	default:
		return nil
	}
}
