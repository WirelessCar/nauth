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
