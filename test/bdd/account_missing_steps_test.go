package bdd

import (
	"context"
	"errors"
	"fmt"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/cucumber/godog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type accountMissingState struct {
	scheme       *runtime.Scheme
	k8sClient    client.Client
	recorder     *record.FakeRecorder
	reconciler   *controller.AccountReconciler
	request      ctrl.Request
	reconcileErr error
}

type noopAccountManager struct{}

func (n *noopAccountManager) Create(ctx context.Context, state *natsv1alpha1.Account) (*controller.AccountResult, error) {
	return nil, errors.New("unexpected Create call")
}

func (n *noopAccountManager) Update(ctx context.Context, state *natsv1alpha1.Account) (*controller.AccountResult, error) {
	return nil, errors.New("unexpected Update call")
}

func (n *noopAccountManager) Import(ctx context.Context, state *natsv1alpha1.Account) (*controller.AccountResult, error) {
	return nil, errors.New("unexpected Import call")
}

func (n *noopAccountManager) Delete(ctx context.Context, desired *natsv1alpha1.Account) error {
	return errors.New("unexpected Delete call")
}

func RegisterAccountMissingSteps(sc *godog.ScenarioContext) {
	state := &accountMissingState{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*state = accountMissingState{}
		return ctx, nil
	})

	sc.Step(`^no Account exists for the reconcile request$`, state.noAccountExists)
	sc.Step(`^the account reconcile loop runs$`, state.runReconcile)
	sc.Step(`^reconciliation completes without error$`, state.expectNoError)
	sc.Step(`^no warning events are recorded$`, state.expectNoWarningEvents)
}

func (s *accountMissingState) noAccountExists() error {
	scheme := runtime.NewScheme()
	if err := natsv1alpha1.AddToScheme(scheme); err != nil {
		return err
	}

	s.scheme = scheme
	s.k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	s.recorder = record.NewFakeRecorder(5)
	s.reconciler = controller.NewAccountReconciler(s.k8sClient, scheme, &noopAccountManager{}, s.recorder)
	s.request = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "missing-account",
			Namespace: "default",
		},
	}

	return nil
}

func (s *accountMissingState) runReconcile() error {
	if s.reconciler == nil {
		return errors.New("reconciler is not initialized")
	}

	_, err := s.reconciler.Reconcile(context.Background(), s.request)
	s.reconcileErr = err
	return nil
}

func (s *accountMissingState) expectNoError() error {
	if s.reconcileErr != nil {
		return fmt.Errorf("expected no error, got %v", s.reconcileErr)
	}
	return nil
}

func (s *accountMissingState) expectNoWarningEvents() error {
	if s.recorder == nil {
		return errors.New("recorder is not initialized")
	}

	select {
	case event := <-s.recorder.Events:
		return fmt.Errorf("unexpected event recorded: %s", event)
	default:
		return nil
	}
}
