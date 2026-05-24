package controller

import (
	"context"
	"os"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type UserControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	userManagerMock *UserManagerMock
	fakeRecorder    *events.FakeRecorder

	userNamespacedName ktypes.NamespacedName
	operatorVersion    string

	unitUnderTest *UserReconciler
}

func TestUserController_TestSuite(t *testing.T) {
	suite.Run(t, new(UserControllerTestSuite))
}

func (t *UserControllerTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.operatorVersion = testOperatorVersion
	t.Require().NoError(os.Setenv(envOperatorVersion, t.operatorVersion))

	testName := t.T().Name()
	userName := testutil.ScopedTestName("test-resource", testName)
	namespace := testutil.ScopedTestName("user", testName)
	t.userNamespacedName = ktypes.NamespacedName{
		Name:      userName,
		Namespace: namespace,
	}

	t.userManagerMock = &UserManagerMock{}
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewUserReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.userManagerMock,
		t.fakeRecorder,
	)

	t.Require().NoError(ensureNamespace(t.ctx, namespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: namespace,
		},
	}))
}

func (t *UserControllerTestSuite) TearDownTest() {
	t.userManagerMock.AssertExpectations(t.T())
	t.Require().NoError(os.Unsetenv(envOperatorVersion))
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenCreatingOrUpdatingUser() {
	// Given
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	user := &v1alpha1.User{}

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)

	for _, c := range user.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(conditionReasonReconciled, c.Reason)
	}
	t.Equal(t.operatorVersion, user.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldFail_WhenCreateOrUpdateFailsBecauseNoAccountExists() {
	// Given
	errAccountNotFound := domain.ErrAccountNotFound
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(errAccountNotFound).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.ErrorIs(err, errAccountNotFound)

	user := &v1alpha1.User{}
	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)

	for _, c := range user.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(conditionReasonErrored, c.Reason)
	}

	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, errAccountNotFound.Error())
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenUserIsDeleted() {
	// Given
	// Note: Expect manager.CreateOrUpdate during setup only
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	user := &v1alpha1.User{}
	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	t.Contains(user.Finalizers, finalizerUser, "User must have the finalizer after first reconcile")

	err = k8sClient.Delete(t.ctx, user)
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	t.NotNil(user.DeletionTimestamp, "User should be in terminating state, held by finalizer")

	// When
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	// Then
	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.True(k8err.IsNotFound(err), "User should be gone after finalizer is removed")
	t.Empty(t.fakeRecorder.Events)
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldRecreateSecret_WhenSecretIsDeleted() {
	// Given
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.userNamespacedName, user))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      user.GetUserSecretName(),
			Namespace: user.Namespace,
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, secret))

	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)
	t.userManagerMock.AssertNotCalled(t.T(), "CreateOrUpdate")

	// When
	t.Require().NoError(k8sClient.Delete(t.ctx, secret))

	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.Require().NoError(err)
	t.Empty(t.fakeRecorder.Events)
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenOperatorVersionChanges() {
	// Given
	// Note: Expect manager.CreateOrUpdate during setup once
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	user := &v1alpha1.User{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	newOperatorVersion := "1.1-SNAPSHOT"
	t.Require().NoError(os.Setenv(envOperatorVersion, newOperatorVersion))

	// Note: assert mock calls during setup and reset for test case
	t.userManagerMock.AssertExpectations(t.T())
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()

	// When
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	for _, c := range user.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(conditionReasonReconciled, c.Reason)
	}
	t.Equal(newOperatorVersion, user.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *UserControllerTestSuite) namespace() string {
	return t.userNamespacedName.Namespace
}

// createUserWithSigningKeyRef creates a User in the test namespace with spec.signingKeyRef
// and spec.accountName set. The default user (from SetupTest) is separate.
func (t *UserControllerTestSuite) createUserWithSigningKeyRef(name, accountName, signingKeyRef string) ktypes.NamespacedName {
	nn := ktypes.NamespacedName{Name: name, Namespace: t.namespace()}
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: t.namespace()},
		Spec: v1alpha1.UserSpec{
			AccountName:   accountName,
			SigningKeyRef: signingKeyRef,
		},
	}))
	return nn
}

// markUserReconciled sets ObservedGeneration and OperatorVersion so the early-return
// would normally skip reconciliation, and optionally stamps the SignedBy label.
func (t *UserControllerTestSuite) markUserReconciled(nn ktypes.NamespacedName, signedBy string) {
	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, nn, user))

	if signedBy != "" {
		user.Labels = map[string]string{string(v1alpha1.UserLabelSignedBy): signedBy}
		t.Require().NoError(k8sClient.Update(t.ctx, user))
		t.Require().NoError(k8sClient.Get(t.ctx, nn, user))
	}

	user.Status.ObservedGeneration = user.Generation
	user.Status.OperatorVersion = t.operatorVersion
	t.Require().NoError(k8sClient.Status().Update(t.ctx, user))
}

// createSecret creates a creds secret for a User (so secretMissing = false).
func (t *UserControllerTestSuite) createSecret(nn ktypes.NamespacedName) {
	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, nn, user))
	t.Require().NoError(k8sClient.Create(t.ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      user.GetUserSecretName(),
			Namespace: user.Namespace,
		},
	}))
}

// createReadyAccount creates an Account with the given signing keys in spec and status.
func (t *UserControllerTestSuite) createReadyAccount(name string, signingKeyRefs []string, claimsSigningKeys []string) {
	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: t.namespace()},
		Spec:       v1alpha1.AccountSpec{SigningKeyRefs: signingKeyRefs},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, account))

	updated := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, ktypes.NamespacedName{Name: name, Namespace: t.namespace()}, updated))

	signingKeys := make(v1alpha1.SigningKeys, 0, len(claimsSigningKeys))
	for _, k := range claimsSigningKeys {
		key := k
		signingKeys = append(signingKeys, &v1alpha1.SigningKey{Key: key})
	}
	updated.Status.Claims = &v1alpha1.AccountClaims{SigningKeys: signingKeys}
	t.Require().NoError(k8sClient.Status().Update(t.ctx, updated))
}

// createReadyAccountSigningKey creates an AccountSigningKey with Ready=True and the given public key.
func (t *UserControllerTestSuite) createReadyAccountSigningKey(name, publicKey string) {
	ask := &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: t.namespace()},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, ask))

	updated := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, ktypes.NamespacedName{Name: name, Namespace: t.namespace()}, updated))
	updated.Status.PublicKey = publicKey
	updated.Status.SecretName = name + "-secret"
	updated.Status.Conditions = []metav1.Condition{
		{Type: conditionTypeReady, Status: metav1.ConditionTrue, Reason: conditionReasonOK, LastTransitionTime: metav1.Now()},
	}
	t.Require().NoError(k8sClient.Status().Update(t.ctx, updated))
}

func (t *UserControllerTestSuite) Test_Reconcile_SigningKeyRef_DoesNotEarlyReturn_WhenAccountNoLongerTrustsRef() {
	// Given: a User with signingKeyRef that was previously reconciled successfully
	// (secret exists, ObservedGeneration == Generation, SignedBy label matches ASK pubKey),
	// but Account.spec.signingKeyRefs no longer lists the ref.
	// Without the stale check the early-return would skip reconciliation; with it the
	// reconcile must proceed and surface the NotReady/requeue condition.
	const askName = "ask-revoked"
	const askPubKey = "APUBKEYREVOKED"
	const accountName = "acct-revoked"

	userNN := t.createUserWithSigningKeyRef("user-revoked", accountName, askName)
	t.createReadyAccountSigningKey(askName, askPubKey)
	t.createReadyAccount(accountName, []string{askName}, []string{askPubKey})
	t.markUserReconciled(userNN, askPubKey)
	t.createSecret(userNN)

	// Remove the signingKeyRef from Account.spec — trust is now revoked
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, ktypes.NamespacedName{Name: accountName, Namespace: t.namespace()}, account))
	account.Spec.SigningKeyRefs = nil
	t.Require().NoError(k8sClient.Update(t.ctx, account))

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: userNN})

	// Then
	t.NoError(err)
	t.Equal(requeueDependencyNotReady, result.RequeueAfter)

	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, userNN, user))
	cond := meta.FindStatusCondition(user.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(cond)
	t.Equal(metav1.ConditionFalse, cond.Status)
	t.Equal(conditionReasonNotReady, cond.Reason)
}

func (t *UserControllerTestSuite) Test_Reconcile_SigningKeyRef_MarksNotReady_WhenRefNotInAccountSpec() {
	// Given: ASK is ready and has a public key, but spec.signingKeyRefs does not list the ref.
	const askName = "ask-not-in-spec"
	const askPubKey = "APUBKEYNOTINSPEC"
	const accountName = "acct-not-in-spec"

	userNN := t.createUserWithSigningKeyRef("user-not-in-spec", accountName, askName)
	t.createReadyAccountSigningKey(askName, askPubKey)
	// Account does NOT include askName in signingKeyRefs
	t.createReadyAccount(accountName, nil, []string{askPubKey})

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: userNN})

	// Then
	t.NoError(err)
	t.Equal(requeueDependencyNotReady, result.RequeueAfter)

	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, userNN, user))
	cond := meta.FindStatusCondition(user.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(cond)
	t.Equal(metav1.ConditionFalse, cond.Status)
	t.Equal(conditionReasonNotReady, cond.Reason)
}

func (t *UserControllerTestSuite) Test_Reconcile_SigningKeyRef_MarksNotReady_WhenAccountClaimsIsNil() {
	// Given: Account exists with signingKeyRef listed in spec, but status.claims is nil.
	const askName = "ask-claims-nil"
	const askPubKey = "APUBKEYCLAIMSNIL"
	const accountName = "acct-claims-nil"

	userNN := t.createUserWithSigningKeyRef("user-claims-nil", accountName, askName)
	t.createReadyAccountSigningKey(askName, askPubKey)

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: accountName, Namespace: t.namespace()},
		Spec:       v1alpha1.AccountSpec{SigningKeyRefs: []string{askName}},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, account))
	// status.claims intentionally left nil

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: userNN})

	// Then
	t.NoError(err)
	t.Equal(requeueDependencyNotReady, result.RequeueAfter)

	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, userNN, user))
	cond := meta.FindStatusCondition(user.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(cond)
	t.Equal(metav1.ConditionFalse, cond.Status)
}

func (t *UserControllerTestSuite) Test_Reconcile_SigningKeyRef_MarksNotReady_WhenClaimsSigningKeysDoesNotContainASKPubKey() {
	// Given: Account has claims but the ASK public key is absent from signingKeys.
	const askName = "ask-missing-key"
	const askPubKey = "APUBKEYMISSINGKEY"
	const accountName = "acct-missing-key"

	userNN := t.createUserWithSigningKeyRef("user-missing-key", accountName, askName)
	t.createReadyAccountSigningKey(askName, askPubKey)
	// Account has signingKeyRef in spec but claims list is empty
	t.createReadyAccount(accountName, []string{askName}, nil)

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: userNN})

	// Then
	t.NoError(err)
	t.Equal(requeueDependencyNotReady, result.RequeueAfter)

	user := &v1alpha1.User{}
	t.Require().NoError(k8sClient.Get(t.ctx, userNN, user))
	cond := meta.FindStatusCondition(user.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(cond)
	t.Equal(metav1.ConditionFalse, cond.Status)
}

func (t *UserControllerTestSuite) Test_Reconcile_SigningKeyRef_IsStale_WhenSignedByLabelDiffersFromASKPublicKey() {
	// Given: User was reconciled with an old public key; ASK now has a different key.
	// The creds secret exists and the generation/version match — only the stale check differs.
	const askName = "ask-rotated"
	const oldPubKey = "AOLDPUBKEY"
	const newPubKey = "ANEWPUBKEY"
	const accountName = "acct-rotated"

	userNN := t.createUserWithSigningKeyRef("user-rotated", accountName, askName)
	// ASK now exposes the new key
	t.createReadyAccountSigningKey(askName, newPubKey)
	t.createReadyAccount(accountName, []string{askName}, []string{newPubKey})
	// Mark the User as already reconciled with the OLD key
	t.markUserReconciled(userNN, oldPubKey)
	t.createSecret(userNN)

	// When
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: userNN})

	// Then
	t.NoError(err)
}

type UserManagerMock struct {
	mock.Mock
}

func (u *UserManagerMock) CreateOrUpdate(ctx context.Context, request nauth.UserRequest) (*nauth.UserResult, error) {
	args := u.Called(ctx, request)
	if err := args.Error(0); err != nil {
		return nil, err
	}
	return &nauth.UserResult{}, nil
}
