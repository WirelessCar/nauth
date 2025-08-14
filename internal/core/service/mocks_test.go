package service

import (
	"context"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/ports"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
)

/*****************************************************
* Secret storer
*****************************************************/

func NewSecretStorerMock() *SecretStorerMock {
	return &SecretStorerMock{
		secrets: map[string]map[string]string{},
	}
}

type SecretStorerMock struct {
	mock.Mock
	secrets map[string]map[string]string
}

// ApplySecret implements ports.SecretStorer.
func (s *SecretStorerMock) ApplySecret(ctx context.Context, secretOwner *ports.SecretOwner, namespace string, name string, valueMap map[string]string) error {
	s.secrets[name] = valueMap
	args := s.Called(ctx, secretOwner, namespace, name, valueMap)
	return args.Error(0)
}

// GetSecret implements ports.SecretStorer.
func (s *SecretStorerMock) GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error) {
	args := s.Called(ctx, namespace, name)
	return s.secrets[name], args.Error(0)
}

// DeleteSecret implements ports.SecretStorer.
func (s *SecretStorerMock) DeleteSecret(ctx context.Context, namespace string, name string) error {
	args := s.Called(ctx, namespace, name)
	return args.Error(0)
}

/*****************************************************
* NATS Client
*****************************************************/

func NewNATSClientMock() *NATSClientMock {
	return &NATSClientMock{}
}

type NATSClientMock struct {
	mock.Mock
}

func (n *NATSClientMock) Connect(namespace string, secretName string) error {
	args := n.Called(namespace, secretName)
	return args.Error(0)
}

func (n *NATSClientMock) EnsureConnected(namespace string, secretName string) error {
	args := n.Called(namespace, secretName)
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

/*****************************************************
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
