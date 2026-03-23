package controller

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	t.Require().NoError(os.Setenv(EnvOperatorVersion, t.operatorVersion))

	testName := t.T().Name()
	userName := scopedTestName("test-resource", testName)
	namespace := scopedTestName("user", testName)
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
	t.Require().NoError(os.Unsetenv(EnvOperatorVersion))
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
		t.Equal(controllerReasonReconciled, c.Reason)
	}
	t.Equal(t.operatorVersion, user.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldFail_WhenCreateOrUpdateFailsBecauseNoAccountExists() {
	// Given
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(k8s.ErrNoAccountFound).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.Error(err)
	t.Equal(k8s.ErrNoAccountFound, err)

	user := &v1alpha1.User{}
	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)

	for _, c := range user.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(controllerReasonErrored, c.Reason)
	}

	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, k8s.ErrNoAccountFound.Error())
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldDeleteUserMarkedForDeletion() {
	// Given
	// Note: Expect manager.CreateOrUpdate during setup only
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	user := &v1alpha1.User{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	t.True(controllerutil.ContainsFinalizer(user, controllerUserFinalizer))

	err = k8sClient.Delete(t.ctx, user)
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	t.False(user.DeletionTimestamp.IsZero())

	// Note: assert mock calls during setup and reset for test case
	t.userManagerMock.AssertExpectations(t.T())
	t.userManagerMock.On("Delete", mock.Anything, mock.Anything).Return(nil)

	// When (expect manager.Delete)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.NoError(err)

	for _, c := range user.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(controllerReasonReconciled, c.Reason)
	}

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Error(err)
	t.True(k8err.IsNotFound(err))
	t.Empty(t.fakeRecorder.Events)
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldFail_WhenDeleteFails() {
	// Given
	userDeleteError := fmt.Errorf("unable to remove the user")
	// Note: Expect manager.CreateOrUpdate during setup only
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	user := &v1alpha1.User{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	t.True(controllerutil.ContainsFinalizer(user, controllerUserFinalizer))

	err = k8sClient.Delete(t.ctx, user)
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	t.False(user.DeletionTimestamp.IsZero())

	// Note: assert mock calls during setup and reset for test case
	t.userManagerMock.AssertExpectations(t.T())
	t.userManagerMock.On("Delete", mock.Anything, mock.Anything).Return(userDeleteError).Once()

	// When (expect manager.Delete)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})

	// Then
	t.Error(err)
	t.Contains(err.Error(), userDeleteError.Error())

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.Require().NoError(err)
	for _, c := range user.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(controllerReasonErrored, c.Reason)
	}

	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, userDeleteError.Error())

	err = k8sClient.Get(t.ctx, t.userNamespacedName, user)
	t.NoError(err)
}

func (t *UserControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenOperatorVersionChanges() {
	// Given
	// Note: Expect manager.CreateOrUpdate during setup once
	t.userManagerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).Return(nil).Once()
	user := &v1alpha1.User{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.userNamespacedName})
	t.Require().NoError(err)

	newOperatorVersion := "1.1-SNAPSHOT"
	t.Require().NoError(os.Setenv(EnvOperatorVersion, newOperatorVersion))

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
		t.Equal(controllerReasonReconciled, c.Reason)
	}
	t.Equal(newOperatorVersion, user.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

type UserManagerMock struct {
	mock.Mock
}

func (u *UserManagerMock) CreateOrUpdate(ctx context.Context, state *v1alpha1.User) error {
	state.Status.ObservedGeneration = state.Generation
	args := u.Called(state)
	return args.Error(0)
}

func (u *UserManagerMock) Delete(ctx context.Context, desired *v1alpha1.User) error {
	args := u.Called(desired)
	return args.Error(0)
}
