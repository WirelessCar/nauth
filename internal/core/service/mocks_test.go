package service

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/core/ports"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/* ****************************************************
* Secret storer
*****************************************************/

func NewSecretStorerMock() *SecretStorerMock {
	return &SecretStorerMock{}
}

type SecretStorerMock struct {
	mock.Mock
}

// ApplySecret implements ports.SecretStorer.
func (s *SecretStorerMock) ApplySecret(ctx context.Context, secretOwner *ports.SecretOwner, meta metav1.ObjectMeta, valueMap map[string]string) error {
	args := s.Called(ctx, secretOwner, meta, valueMap)
	return args.Error(0)
}

// GetSecret implements ports.SecretStorer.
func (s *SecretStorerMock) GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error) {
	args := s.Called(ctx, namespace, name)
	return args.Get(0).(map[string]string), args.Error(1)
}

// GetSecretsByLabels implements ports.SecretStorer.
func (s *SecretStorerMock) GetSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*corev1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*corev1.SecretList), args.Error(1)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretStorerMock) DeleteSecret(ctx context.Context, namespace string, name string) error {
	args := s.Called(ctx, namespace, name)
	return args.Error(0)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretStorerMock) DeleteSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) error {
	args := s.Called(ctx, namespace, labels)
	return args.Error(0)
}

// LabelSecret implements ports.SecretStorer.
func (s *SecretStorerMock) LabelSecret(ctx context.Context, namespace, name string, labels map[string]string) error {
	args := s.Called(ctx, namespace, name, labels)
	return args.Error(0)
}

/* ****************************************************
* NATS Client
*****************************************************/

func NewNATSClientMock() *NATSClientMock {
	return &NATSClientMock{}
}

type NATSClientMock struct {
	mock.Mock
}

func (n *NATSClientMock) EnsureConnected(namespace string) error {
	args := n.Called(namespace)
	return args.Error(0)
}

func (n *NATSClientMock) Disconnect() {
	n.Called()
}

func (n *NATSClientMock) UploadAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

func (n *NATSClientMock) DeleteAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

/* ****************************************************
* Account Getter
*****************************************************/

type AccountGetterMock struct {
	mock.Mock
}

func NewAccountGetterMock() *AccountGetterMock {
	return &AccountGetterMock{}
}

// Get implements ports.AccountGetter.
func (a *AccountGetterMock) Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error) {
	args := a.Called(ctx, accountRefName, namespace)
	anAccount := args.Get(0).(v1alpha1.Account)
	return &anAccount, args.Error(1)
}

type ConfigManagerMock struct {
	mock.Mock
}

func NewConfigManagerMock() *ConfigManagerMock {
	return &ConfigManagerMock{}
}

// Get implements ports.AccountGetter.

func (a *ConfigManagerMock) ApplyConfiguration(ctx context.Context, owner *ports.SecretOwner, cm *corev1.ConfigMap) error {
	args := a.Called(owner, cm)
	return args.Error(0)
}
