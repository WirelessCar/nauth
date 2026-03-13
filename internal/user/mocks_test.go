package user

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/* ****************************************************
* Secret storer
*****************************************************/

func NewSecretStorerMock() *SecretClientMock {
	return &SecretClientMock{}
}

type SecretClientMock struct {
	mock.Mock
}

// ApplySecret implements ports.SecretStorer.
func (s *SecretClientMock) Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, valueMap map[string]string) error {
	args := s.Called(ctx, owner, meta, valueMap)
	return args.Error(0)
}

// GetSecret implements ports.SecretStorer.
func (s *SecretClientMock) Get(ctx context.Context, namespace string, name string) (map[string]string, error) {
	args := s.Called(ctx, namespace, name)
	return args.Get(0).(map[string]string), args.Error(1)
}

// GetByLabels implements ports.SecretStorer.
func (s *SecretClientMock) GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*corev1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*corev1.SecretList), args.Error(1)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretClientMock) Delete(ctx context.Context, namespace string, name string) error {
	args := s.Called(ctx, namespace, name)
	return args.Error(0)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretClientMock) DeleteByLabels(ctx context.Context, namespace string, labels map[string]string) error {
	args := s.Called(ctx, namespace, labels)
	return args.Error(0)
}

// LabelSecret implements ports.SecretStorer.
func (s *SecretClientMock) Label(ctx context.Context, namespace, name string, labels map[string]string) error {
	args := s.Called(ctx, namespace, name, labels)
	return args.Error(0)
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

func (a *AccountReaderMock) Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error) {
	args := a.Called(ctx, accountRefName, namespace)
	anAccount := args.Get(0).(v1alpha1.Account)
	return &anAccount, args.Error(1)
}

// Compile-time assertion that implementation satisfies the ports interface
var _ ports.AccountReader = &AccountReaderMock{}
