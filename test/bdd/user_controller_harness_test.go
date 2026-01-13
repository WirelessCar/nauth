package bdd

import (
	"context"
	"errors"
	"fmt"
	"os"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/cucumber/godog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type userControllerState struct {
	scheme        *runtime.Scheme
	k8sClient     client.Client
	recorder      *record.FakeRecorder
	reconciler    *controller.UserReconciler
	userMgr       *stubUserManager
	request       ctrl.Request
	reconcileErr  error
	reconcileResp ctrl.Result

	userName      string
	userNamespace string
	deletionErr   error
	lastUser      *natsv1alpha1.User
}

type stubUserManager struct {
	createErr   error
	deleteErr   error
	deleteCalls int
}

func (s *stubUserManager) CreateOrUpdate(ctx context.Context, state *natsv1alpha1.User) error {
	return s.createErr
}

func (s *stubUserManager) Delete(ctx context.Context, desired *natsv1alpha1.User) error {
	s.deleteCalls++
	return s.deleteErr
}

func RegisterUserControllerSteps(sc *godog.ScenarioContext, state *userControllerState) {
	sc.Step(`^a user namespace "([^"]*)" exists$`, state.userNamespaceExists)
	sc.Step(`^a User named "([^"]*)" exists in that namespace$`, state.userExists)
	sc.Step(`^the User specification is valid and references an existing Account$`, state.userSpecIsValid)
	sc.Step(`^the User references a missing Account$`, state.userReferencesMissingAccount)
	sc.Step(`^the user reconcile loop runs$`, state.runReconcile)
	sc.Step(`^the User status condition is "([^"]*)" with reason "([^"]*)"$`, state.userStatusConditionIs)
	sc.Step(`^the User status operator version is "([^"]*)"$`, state.userStatusOperatorVersionIs)
	sc.Step(`^reconciliation returns the same error$`, state.expectSameError)
	sc.Step(`^the User is ready for deletion$`, state.userIsReadyForDeletion)
	sc.Step(`^the User is deleted and reconciliation runs$`, state.userIsDeletedAndReconciliationRuns)
	sc.Step(`^the User resource is removed from the cluster$`, state.userResourceIsRemovedFromCluster)
	sc.Step(`^the User deletion cannot complete due to an external dependency error$`, state.userDeletionExternalError)
	sc.Step(`^the User resource still exists$`, state.userResourceStillExists)
	sc.Step(`^the operator version changes to "([^"]*)" and reconciliation runs again$`, state.operatorVersionChangesAndReconcileAgain)
	sc.Step(`^no User exists for the reconcile request$`, state.noUserExists)
	sc.Step(`^the User status observed generation matches the current generation$`, state.userObservedGenerationMatches)
	sc.Step(`^the User has no finalizers$`, state.userHasNoFinalizers)
	sc.Step(`^the User includes the "([^"]*)" finalizer$`, state.userIncludesFinalizer)
}

func (s *userControllerState) initHarness() error {
	s.scheme = runtime.NewScheme()
	if err := natsv1alpha1.AddToScheme(s.scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(s.scheme); err != nil {
		return err
	}

	s.k8sClient = fake.NewClientBuilder().
		WithScheme(s.scheme).
		WithStatusSubresource(&natsv1alpha1.User{}).
		Build()
	s.recorder = record.NewFakeRecorder(10)
	s.userMgr = &stubUserManager{}
	s.reconciler = controller.NewUserReconciler(s.k8sClient, s.scheme, s.userMgr, s.recorder)
	s.syncRequest()

	return nil
}

func (s *userControllerState) syncRequest() {
	s.request = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      s.userName,
			Namespace: s.userNamespace,
		},
	}
}

func (s *userControllerState) ensureNamespace(name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := s.k8sClient.Create(context.Background(), ns); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create namespace %q: %w", name, err)
	}
	return nil
}

func (s *userControllerState) ensureUser() error {
	user := &natsv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.userName,
			Namespace: s.userNamespace,
		},
		Spec: natsv1alpha1.UserSpec{
			AccountName: "account-1",
		},
	}
	if err := s.k8sClient.Create(context.Background(), user); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *userControllerState) getUser() (*natsv1alpha1.User, error) {
	user := &natsv1alpha1.User{}
	err := s.k8sClient.Get(context.Background(), s.request.NamespacedName, user)
	return user, err
}

func (s *userControllerState) setDeletionTimestamp() error {
	user, err := s.getUser()
	if err != nil {
		return err
	}
	if len(user.Finalizers) == 0 {
		user.Finalizers = []string{"user.nauth.io/finalizer"}
		if err := s.k8sClient.Update(context.Background(), user); err != nil {
			return err
		}
	}
	return s.k8sClient.Delete(context.Background(), user)
}

func (s *userControllerState) userNamespaceExists(namespace string) error {
	if err := s.initHarness(); err != nil {
		return err
	}
	s.userNamespace = namespace
	s.syncRequest()
	return s.ensureNamespace(namespace)
}

func (s *userControllerState) userExists(name string) error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.userName = name
	s.syncRequest()
	return s.ensureUser()
}

func (s *userControllerState) operatorVersionIs(version string) error {
	return os.Setenv(controller.EnvOperatorVersion, version)
}

func (s *userControllerState) userSpecIsValid() error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.userMgr.createErr = nil
	return s.ensureUser()
}

func (s *userControllerState) userReferencesMissingAccount() error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.userMgr.createErr = k8s.ErrNoAccountFound
	return s.ensureUser()
}

func (s *userControllerState) runReconcile() error {
	if s.reconciler == nil {
		return errors.New("reconciler is not initialized")
	}
	resp, err := s.reconciler.Reconcile(context.Background(), s.request)
	s.reconcileErr = err
	s.reconcileResp = resp
	return nil
}

func (s *userControllerState) userStatusConditionIs(status string, reason string) error {
	user, err := s.getUser()
	if err != nil {
		if apierrors.IsNotFound(err) && s.lastUser != nil {
			user = s.lastUser
		} else {
			return err
		}
	}
	conditions := user.Status.Conditions
	if len(conditions) == 0 {
		return errors.New("no status conditions set")
	}
	condition := conditions[len(conditions)-1]
	if string(condition.Status) != status {
		return fmt.Errorf("expected condition status %q, got %q", status, condition.Status)
	}
	if condition.Reason != reason {
		return fmt.Errorf("expected condition reason %q, got %q", reason, condition.Reason)
	}
	return nil
}

func (s *userControllerState) userStatusOperatorVersionIs(version string) error {
	user, err := s.getUser()
	if err != nil {
		return err
	}
	user.Status.OperatorVersion = version
	return s.k8sClient.Status().Update(context.Background(), user)
}

func (s *userControllerState) warningEventIncludes(expected string) error {
	if s.recorder == nil {
		return errors.New("recorder is not initialized")
	}
	select {
	case event := <-s.recorder.Events:
		if !contains(event, expected) {
			return fmt.Errorf("expected warning event to include %q, got %q", expected, event)
		}
		return nil
	default:
		return errors.New("expected warning event but none recorded")
	}
}

func (s *userControllerState) expectSameError() error {
	if !errors.Is(s.reconcileErr, k8s.ErrNoAccountFound) {
		return fmt.Errorf("expected error %q, got %v", k8s.ErrNoAccountFound, s.reconcileErr)
	}
	return nil
}

func (s *userControllerState) userIsReadyForDeletion() error {
	if err := s.runReconcile(); err != nil {
		return err
	}
	user, err := s.getUser()
	if err != nil {
		return err
	}
	if len(user.Finalizers) == 0 {
		user.Finalizers = []string{"user.nauth.io/finalizer"}
	}
	return s.k8sClient.Update(context.Background(), user)
}

func (s *userControllerState) userIsDeletedAndReconciliationRuns() error {
	user, err := s.getUser()
	if err == nil {
		copyUser := user.DeepCopy()
		s.lastUser = copyUser
	}
	if err := s.setDeletionTimestamp(); err != nil {
		return err
	}
	if err := s.runReconcile(); err != nil {
		return err
	}

	user, err = s.getUser()
	if err != nil {
		return nil
	}
	if user.DeletionTimestamp != nil && len(user.Finalizers) == 0 {
		return s.k8sClient.Delete(context.Background(), user)
	}
	return nil
}

func (s *userControllerState) userResourceIsRemovedFromCluster() error {
	_, err := s.getUser()
	if err == nil {
		return errors.New("expected user to be deleted but it still exists")
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("expected not found error, got %v", err)
	}
	return nil
}

func (s *userControllerState) expectNoError() error {
	if s.reconcileErr != nil {
		return fmt.Errorf("expected no error, got %v", s.reconcileErr)
	}
	return nil
}

func (s *userControllerState) userDeletionExternalError() error {
	s.deletionErr = errors.New("unable to remove the user")
	s.userMgr.deleteErr = s.deletionErr
	return s.userIsReadyForDeletion()
}

func (s *userControllerState) warningEventIncludesDeletionError() error {
	if s.deletionErr == nil {
		return errors.New("deletion error not set")
	}
	return s.warningEventIncludes(s.deletionErr.Error())
}

func (s *userControllerState) expectError() error {
	if s.reconcileErr == nil {
		return errors.New("expected an error but got none")
	}
	return nil
}

func (s *userControllerState) userResourceStillExists() error {
	_, err := s.getUser()
	if err != nil {
		return fmt.Errorf("expected user to exist, got %v", err)
	}
	return nil
}

func (s *userControllerState) operatorVersionChangesAndReconcileAgain(version string) error {
	if err := os.Setenv(controller.EnvOperatorVersion, version); err != nil {
		return err
	}
	return s.runReconcile()
}

func (s *userControllerState) noUserExists() error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	user, err := s.getUser()
	if err == nil {
		if err := s.k8sClient.Delete(context.Background(), user); err != nil {
			return err
		}
	}
	return nil
}

func (s *userControllerState) userObservedGenerationMatches() error {
	user, err := s.getUser()
	if err != nil {
		return err
	}
	user.Status.ObservedGeneration = user.Generation
	return s.k8sClient.Status().Update(context.Background(), user)
}

func (s *userControllerState) userHasNoFinalizers() error {
	user, err := s.getUser()
	if err != nil {
		return err
	}
	user.Finalizers = nil
	return s.k8sClient.Update(context.Background(), user)
}

func (s *userControllerState) userIncludesFinalizer(finalizer string) error {
	user, err := s.getUser()
	if err != nil {
		return err
	}
	for _, existing := range user.Finalizers {
		if existing == finalizer {
			return nil
		}
	}
	return fmt.Errorf("expected finalizer %q to be present", finalizer)
}
