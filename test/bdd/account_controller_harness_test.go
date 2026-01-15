package bdd

import (
	"context"
	"errors"
	"fmt"
	"os"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/account"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	"github.com/cucumber/godog"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
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

type accountControllerState struct {
	scheme        *runtime.Scheme
	k8sClient     client.Client
	recorder      *record.FakeRecorder
	reconciler    *controller.AccountReconciler
	accountMgr    *account.Manager
	secretClient  *testSecretClient
	natsClient    *fakeNatsClient
	operatorKey   nkeys.KeyPair
	request       ctrl.Request
	reconcileErr  error
	reconcileResp ctrl.Result

	accountName       string
	accountNamespace  string
	operatorNamespace string
	deletionErrMsg    string
}

func RegisterAccountControllerSteps(sc *godog.ScenarioContext, state *accountControllerState) {
	sc.Step(`^the operator namespace "([^"]*)" exists$`, state.operatorNamespaceExists)
	sc.Step(`^an account namespace "([^"]*)" exists$`, state.accountNamespaceExists)
	sc.Step(`^an Account named "([^"]*)" exists in that namespace$`, state.accountExists)
	sc.Step(`^the Account specification is valid$`, state.accountSpecIsValid)
	sc.Step(`^the Account specification is invalid$`, state.accountSpecIsInvalid)
	sc.Step(`^no Account exists for the reconcile request$`, state.noAccountExists)
	sc.Step(`^the account manager returns a created account result for "([^"]*)"$`, state.accountManagerReturnsCreateResult)
	sc.Step(`^the account manager returns an updated account result for "([^"]*)"$`, state.accountManagerReturnsUpdateResult)
	sc.Step(`^the account manager returns an error "([^"]*)" during create$`, state.accountManagerCreateError)
	sc.Step(`^the Account status condition is "([^"]*)" with reason "([^"]*)"$`, state.accountStatusConditionIs)

	sc.Step(`^the Account status observed generation matches the current generation$`, state.accountObservedGenerationMatches)
	sc.Step(`^the Account status operator version is "([^"]*)"$`, state.accountStatusOperatorVersionIs)
	sc.Step(`^the Account has no finalizers$`, state.accountHasNoFinalizers)

	sc.Step(`^the Account is labeled with management policy "([^"]*)"$`, state.accountIsLabeledWithManagementPolicy)
	sc.Step(`^the Account is ready for deletion$`, state.accountIsReadyForDeletion)
	sc.Step(`^the Account is deleted and reconciliation runs$`, state.accountIsDeletedAndReconciliationRuns)
	sc.Step(`^the Account resource is removed from the cluster$`, state.accountResourceIsRemovedFromCluster)
	sc.Step(`^the Account resource still exists$`, state.accountResourceStillExists)
	sc.Step(`^no managed resources are deleted by the controller$`, state.noManagedResourcesDeleted)
	sc.Step(`^the Account deletion cannot complete due to an external dependency error$`, state.accountDeletionExternalError)
	sc.Step(`^the Account has associated Users in the same namespace$`, state.accountHasAssociatedUsers)
	sc.Step(`^the operator version changes to "([^"]*)" and reconciliation runs$`, state.operatorVersionChangesAndReconcile)

	sc.Step(`^the account reconcile loop runs$`, state.runReconcile)
	sc.Step(`^the Account includes the "([^"]*)" finalizer$`, state.accountIncludesFinalizer)
}

func (s *accountControllerState) initHarness() error {
	if s.operatorNamespace == "" {
		s.operatorNamespace = "nauth-account-system"
	}
	s.scheme = runtime.NewScheme()
	if err := natsv1alpha1.AddToScheme(s.scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(s.scheme); err != nil {
		return err
	}

	s.k8sClient = fake.NewClientBuilder().
		WithScheme(s.scheme).
		WithStatusSubresource(&natsv1alpha1.Account{}).
		Build()
	s.recorder = record.NewFakeRecorder(10)
	s.natsClient = &fakeNatsClient{}
	s.secretClient = newTestSecretClient(secret.NewClient(s.k8sClient, secret.WithControllerNamespace(s.operatorNamespace)))
	accountGetter := k8s.NewAccountClient(s.k8sClient)
	s.accountMgr = account.NewManager(accountGetter, s.natsClient, s.secretClient, account.WithNamespace(s.operatorNamespace))
	s.reconciler = controller.NewAccountReconciler(s.k8sClient, s.scheme, s.accountMgr, s.recorder)
	s.syncRequest()

	return nil
}

func (s *accountControllerState) syncRequest() {
	s.request = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      s.accountName,
			Namespace: s.accountNamespace,
		},
	}
}

func (s *accountControllerState) ensureNamespace(name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := s.k8sClient.Create(context.Background(), ns); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create namespace %q: %w", name, err)
	}
	return nil
}

func (s *accountControllerState) ensureAccount() error {
	account := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.accountName,
			Namespace: s.accountNamespace,
		},
	}
	if err := s.k8sClient.Create(context.Background(), account); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (s *accountControllerState) getAccount() (*natsv1alpha1.Account, error) {
	account := &natsv1alpha1.Account{}
	err := s.k8sClient.Get(context.Background(), s.request.NamespacedName, account)
	return account, err
}

func (s *accountControllerState) setDeletionTimestamp() error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	if len(account.Finalizers) == 0 {
		account.Finalizers = []string{"account.nauth.io/finalizer"}
		if err := s.k8sClient.Update(context.Background(), account); err != nil {
			return err
		}
	}
	return s.k8sClient.Delete(context.Background(), account)
}

func (s *accountControllerState) runReconcile() error {
	if s.reconciler == nil {
		return errors.New("reconciler is not initialized")
	}
	resp, err := s.reconciler.Reconcile(context.Background(), s.request)
	s.reconcileErr = err
	s.reconcileResp = resp
	return nil
}

func (s *accountControllerState) operatorNamespaceExists(namespace string) error {
	s.operatorNamespace = namespace
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	return s.ensureNamespace(namespace)
}

func (s *accountControllerState) accountNamespaceExists(namespace string) error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.accountNamespace = namespace
	s.syncRequest()
	return s.ensureNamespace(namespace)
}

func (s *accountControllerState) accountExists(name string) error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.accountName = name
	s.syncRequest()
	return s.ensureAccount()
}

func (s *accountControllerState) operatorVersionIs(version string) error {
	return os.Setenv(controller.EnvOperatorVersion, version)
}

func (s *accountControllerState) noAccountExists() error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.syncRequest()
	account, err := s.getAccount()
	if err == nil {
		if err := s.k8sClient.Delete(context.Background(), account); err != nil {
			return err
		}
	}
	return nil
}

func (s *accountControllerState) accountSpecIsValid() error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.syncRequest()
	if err := s.ensureNamespace(s.operatorNamespace); err != nil {
		return err
	}
	if err := s.ensureNamespace(s.accountNamespace); err != nil {
		return err
	}
	if err := s.ensureOperatorSigningSecret(); err != nil {
		return err
	}
	if err := s.ensureSystemAccountCredsSecret(); err != nil {
		return err
	}
	s.secretClient.applyErr = nil
	s.secretClient.deleteByLabelsErr = nil
	s.natsClient.ensureErr = nil
	s.natsClient.uploadErr = nil
	s.natsClient.deleteErr = nil
	return s.ensureAccount()
}

func (s *accountControllerState) accountSpecIsInvalid() error {
	if s.k8sClient == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.syncRequest()
	if err := s.ensureNamespace(s.operatorNamespace); err != nil {
		return err
	}
	if err := s.ensureNamespace(s.accountNamespace); err != nil {
		return err
	}
	if err := s.ensureOperatorSigningSecret(); err != nil {
		return err
	}
	s.secretClient.applyErr = errors.New("a test error")
	return s.ensureAccount()
}

func (s *accountControllerState) accountManagerReturnsCreateResult(name string) error {
	if s.reconciler == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.secretClient.applyErr = nil
	s.accountName = name
	s.syncRequest()
	return nil
}

func (s *accountControllerState) accountManagerReturnsUpdateResult(name string) error {
	if s.reconciler == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.secretClient.applyErr = nil
	s.accountName = name
	s.syncRequest()
	return nil
}

func (s *accountControllerState) accountManagerCreateError(message string) error {
	if s.reconciler == nil {
		if err := s.initHarness(); err != nil {
			return err
		}
	}
	s.secretClient.applyErr = errors.New(message)
	return nil
}

func (s *accountControllerState) accountStatusConditionIs(status string, reason string) error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	conditions := account.Status.Conditions
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

func (s *accountControllerState) warningEventIncludes(expected string) error {
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

func (s *accountControllerState) warningEventIncludesDeletionError() error {
	if s.deletionErrMsg == "" {
		return errors.New("deletion error message not set")
	}
	return s.warningEventIncludes(s.deletionErrMsg)
}

func (s *accountControllerState) expectError() error {
	if s.reconcileErr == nil {
		return errors.New("expected an error but got none")
	}
	return nil
}

func (s *accountControllerState) accountObservedGenerationMatches() error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	account.Status.ObservedGeneration = account.Generation
	return s.k8sClient.Status().Update(context.Background(), account)
}

func (s *accountControllerState) accountStatusOperatorVersionIs(version string) error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	account.Status.OperatorVersion = version
	return s.k8sClient.Status().Update(context.Background(), account)
}

func (s *accountControllerState) accountHasNoFinalizers() error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	account.Finalizers = nil
	return s.k8sClient.Update(context.Background(), account)
}

func (s *accountControllerState) expectNoError() error {
	if s.reconcileErr != nil {
		return fmt.Errorf("expected no error, got %v", s.reconcileErr)
	}
	return nil
}

func (s *accountControllerState) expectNoWarningEvents() error {
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

func (s *accountControllerState) accountIncludesFinalizer(finalizer string) error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	for _, existing := range account.Finalizers {
		if existing == finalizer {
			return nil
		}
	}
	return fmt.Errorf("expected finalizer %q to be present", finalizer)
}

func (s *accountControllerState) accountIsLabeledWithManagementPolicy(value string) error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	if account.Labels == nil {
		account.Labels = make(map[string]string)
	}
	account.Labels[k8s.LabelManagementPolicy] = value
	if value == k8s.LabelManagementPolicyObserveValue {
		if err := s.ensureOperatorSigningSecret(); err != nil {
			return err
		}
		accountID, accountJWT, err := s.prepareAccountForImport()
		if err != nil {
			return err
		}
		account.Labels[k8s.LabelAccountID] = accountID
		s.natsClient.lookupJWT = accountJWT
	}
	return s.k8sClient.Update(context.Background(), account)
}

func (s *accountControllerState) accountIsReadyForDeletion() error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	if err := s.ensureOperatorSigningSecret(); err != nil {
		return err
	}
	if len(account.Finalizers) == 0 {
		account.Finalizers = []string{"account.nauth.io/finalizer"}
	}
	if account.Labels == nil {
		account.Labels = make(map[string]string)
	}
	if account.Labels[k8s.LabelAccountID] == "" {
		accountKeyPair, _ := nkeys.CreateAccount()
		accountID, _ := accountKeyPair.PublicKey()
		account.Labels[k8s.LabelAccountID] = accountID
	}
	return s.k8sClient.Update(context.Background(), account)
}

func (s *accountControllerState) accountIsDeletedAndReconciliationRuns() error {
	if err := s.setDeletionTimestamp(); err != nil {
		return err
	}
	if err := s.runReconcile(); err != nil {
		return err
	}

	account, err := s.getAccount()
	if err != nil {
		return nil
	}
	if account.DeletionTimestamp != nil && len(account.Finalizers) == 0 {
		return s.k8sClient.Delete(context.Background(), account)
	}
	return nil
}

func (s *accountControllerState) accountResourceIsRemovedFromCluster() error {
	_, err := s.getAccount()
	if err == nil {
		return errors.New("expected account to be deleted but it still exists")
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("expected not found error, got %v", err)
	}
	return nil
}

func (s *accountControllerState) accountResourceStillExists() error {
	_, err := s.getAccount()
	if err != nil {
		return fmt.Errorf("expected account to exist, got %v", err)
	}
	return nil
}

func (s *accountControllerState) noManagedResourcesDeleted() error {
	if s.secretClient == nil || s.natsClient == nil {
		return errors.New("account manager not initialized")
	}
	if s.secretClient.deleteByLabelsCall != 0 || s.natsClient.deleteCalls != 0 {
		return fmt.Errorf("expected no deletes, got secrets=%d nats=%d", s.secretClient.deleteByLabelsCall, s.natsClient.deleteCalls)
	}
	return nil
}

func (s *accountControllerState) accountDeletionExternalError() error {
	if s.secretClient == nil {
		return errors.New("account manager not initialized")
	}
	s.deletionErrMsg = "unable to delete account"
	s.secretClient.deleteByLabelsErr = errors.New(s.deletionErrMsg)
	return s.accountIsReadyForDeletion()
}

func (s *accountControllerState) ensureOperatorSigningSecret() error {
	if s.operatorKey != nil {
		return nil
	}
	operatorKeyPair, err := nkeys.CreateOperator()
	if err != nil {
		return err
	}
	operatorSeed, err := operatorKeyPair.Seed()
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operator-signing-key",
			Namespace: s.operatorNamespace,
			Labels: map[string]string{
				k8s.LabelSecretType: k8s.SecretTypeOperatorSign,
			},
		},
		Data: map[string][]byte{
			k8s.DefaultSecretKeyName: operatorSeed,
		},
	}
	if err := ensureSecret(context.Background(), s.k8sClient, secret); err != nil {
		return err
	}
	s.operatorKey = operatorKeyPair
	return nil
}

func (s *accountControllerState) ensureSystemAccountCredsSecret() error {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds,
	}
	existing, err := s.secretClient.GetByLabels(context.Background(), s.operatorNamespace, labels)
	if err == nil && len(existing.Items) > 0 {
		return nil
	}

	sysAccountKeyPair, err := nkeys.CreateAccount()
	if err != nil {
		return err
	}
	sysAccountPub, err := sysAccountKeyPair.PublicKey()
	if err != nil {
		return err
	}
	sysUserKeyPair, err := nkeys.CreateUser()
	if err != nil {
		return err
	}
	sysUserPub, err := sysUserKeyPair.PublicKey()
	if err != nil {
		return err
	}
	sysUserSeed, err := sysUserKeyPair.Seed()
	if err != nil {
		return err
	}
	sysUserClaims := jwt.NewUserClaims(sysUserPub)
	sysUserClaims.IssuerAccount = sysAccountPub
	sysUserJWT, err := sysUserClaims.Encode(sysAccountKeyPair)
	if err != nil {
		return err
	}
	sysUserCreds, err := jwt.FormatUserConfig(sysUserJWT, sysUserSeed)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "system-account-creds",
			Namespace: s.operatorNamespace,
			Labels: map[string]string{
				k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds,
			},
		},
		Data: map[string][]byte{
			k8s.DefaultSecretKeyName: sysUserCreds,
		},
	}
	return ensureSecret(context.Background(), s.k8sClient, secret)
}

func (s *accountControllerState) prepareAccountForImport() (string, string, error) {
	accountKeyPair, err := nkeys.CreateAccount()
	if err != nil {
		return "", "", err
	}
	accountID, err := accountKeyPair.PublicKey()
	if err != nil {
		return "", "", err
	}
	accountSeed, err := accountKeyPair.Seed()
	if err != nil {
		return "", "", err
	}
	signingKeyPair, err := nkeys.CreateAccount()
	if err != nil {
		return "", "", err
	}
	signingSeed, err := signingKeyPair.Seed()
	if err != nil {
		return "", "", err
	}

	rootSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-import-root", s.accountName),
			Namespace: s.accountNamespace,
			Labels: map[string]string{
				k8s.LabelAccountID:  accountID,
				k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			},
		},
		Data: map[string][]byte{
			k8s.DefaultSecretKeyName: accountSeed,
		},
	}
	if err := ensureSecret(context.Background(), s.k8sClient, rootSecret); err != nil {
		return "", "", err
	}
	signSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-import-sign", s.accountName),
			Namespace: s.accountNamespace,
			Labels: map[string]string{
				k8s.LabelAccountID:  accountID,
				k8s.LabelSecretType: k8s.SecretTypeAccountSign,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			},
		},
		Data: map[string][]byte{
			k8s.DefaultSecretKeyName: signingSeed,
		},
	}
	if err := ensureSecret(context.Background(), s.k8sClient, signSecret); err != nil {
		return "", "", err
	}

	claims := jwt.NewAccountClaims(accountID)
	accountJWT, err := claims.Encode(s.operatorKey)
	if err != nil {
		return "", "", err
	}
	return accountID, accountJWT, nil
}

func (s *accountControllerState) accountHasAssociatedUsers() error {
	account, err := s.getAccount()
	if err != nil {
		return err
	}
	if account.Labels == nil {
		account.Labels = make(map[string]string)
	}
	account.Labels[k8s.LabelAccountID] = "ACC_TEST_ID"
	if err := s.k8sClient.Update(context.Background(), account); err != nil {
		return err
	}

	user := &natsv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: s.accountNamespace,
			Labels: map[string]string{
				k8s.LabelUserAccountID: "ACC_TEST_ID",
			},
		},
	}
	return s.k8sClient.Create(context.Background(), user)
}

func (s *accountControllerState) operatorVersionChangesAndReconcile(version string) error {
	if err := os.Setenv(controller.EnvOperatorVersion, version); err != nil {
		return err
	}
	return s.runReconcile()
}
