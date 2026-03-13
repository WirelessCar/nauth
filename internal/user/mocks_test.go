package user

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/* ****************************************************
* ports.SecretClient Mock
*****************************************************/

func NewSecretClientMock() *SecretClientMock {
	return &SecretClientMock{}
}

type SecretClientMock struct {
	mock.Mock
}

// Apply implements ports.SecretStorer.
func (s *SecretClientMock) Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, valueMap map[string]string) error {
	args := s.Called(ctx, owner, meta, valueMap)
	return args.Error(0)
}

func (s *SecretClientMock) mockApply(ctx context.Context, owner interface{}, meta interface{}, valueMap interface{}) {
	s.On("Apply", ctx, owner, meta, valueMap).Return(nil)
}

// Get implements ports.SecretStorer.
func (s *SecretClientMock) Get(ctx context.Context, secretRef domain.NamespacedName) (map[string]string, error) {
	args := s.Called(ctx, secretRef)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (s *SecretClientMock) mockGet(ctx context.Context, namespacedName domain.NamespacedName, result map[string]string) {
	s.On("Get", ctx, namespacedName).Return(result, nil)
}

// GetByLabels implements ports.SecretStorer.
func (s *SecretClientMock) GetByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) (*corev1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*corev1.SecretList), args.Error(1)
}

func (s *SecretClientMock) mockGetByLabels(ctx context.Context, namespace domain.Namespace, labels interface{}, list *corev1.SecretList) {
	s.On("GetByLabels", ctx, namespace, labels).Return(list, nil)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretClientMock) Delete(ctx context.Context, secretRef domain.NamespacedName) error {
	args := s.Called(ctx, secretRef)
	return args.Error(0)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretClientMock) DeleteByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) error {
	args := s.Called(ctx, namespace, labels)
	return args.Error(0)
}

// LabelSecret implements ports.SecretStorer.
func (s *SecretClientMock) Label(ctx context.Context, secretRef domain.NamespacedName, labels map[string]string) error {
	args := s.Called(ctx, secretRef, labels)
	return args.Error(0)
}

func (s *SecretClientMock) mockLabel(namespacedName domain.NamespacedName, labels map[string]string) {
	s.On("Label", mock.Anything, namespacedName, labels).Return(nil)
}

// Compile-time assertion that implementation satisfies the ports interface
var _ ports.SecretClient = &SecretClientMock{}

/* ****************************************************
* ports.AccountReader Mock
*****************************************************/

type AccountReaderMock struct {
	mock.Mock
}

func NewAccountReaderMock() *AccountReaderMock {
	return &AccountReaderMock{}
}

func (a *AccountReaderMock) Get(ctx context.Context, accountRef domain.NamespacedName) (account *v1alpha1.Account, err error) {
	args := a.Called(ctx, accountRef)
	anAccount := args.Get(0).(v1alpha1.Account)
	return &anAccount, args.Error(1)
}

func (a *AccountReaderMock) mockGet(ctx context.Context, accountRef domain.NamespacedName, result v1alpha1.Account) *mock.Call {
	return a.On("Get", ctx, accountRef).Return(result, nil)
}

// Compile-time assertion that implementation satisfies the ports interface
var _ ports.AccountReader = &AccountReaderMock{}
