package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/* ****************************************************
* Secret storer
*****************************************************/

func NewSecretClientMock() *SecretClientMock {
	return &SecretClientMock{}
}

type SecretClientMock struct {
	mock.Mock
}

func (s *SecretClientMock) Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, valueMap map[string]string) error {
	args := s.Called(ctx, owner, meta, valueMap)
	return args.Error(0)
}

func (s *SecretClientMock) mockApply(arguments ...interface{}) *mock.Call {
	return s.On("Apply", arguments...)
}

func (s *SecretClientMock) Get(ctx context.Context, namespacedName domain.NamespacedName) (map[string]string, error) {
	args := s.Called(ctx, namespacedName)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (s *SecretClientMock) mockGet(ctx context.Context, namespacedName domain.NamespacedName, result map[string]string) {
	s.On("Get", ctx, namespacedName).Return(result, nil)
}

func (s *SecretClientMock) mockGetError(namespacedName domain.NamespacedName, err error) {
	s.On("Get", mock.Anything, namespacedName).Return(map[string]string{}, err)
}

func (s *SecretClientMock) GetByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) (*corev1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*corev1.SecretList), args.Error(1)
}

func (s *SecretClientMock) mockGetByLabels(namespace domain.Namespace, labels map[string]string, result *corev1.SecretList) {
	s.On("GetByLabels", mock.Anything, namespace, labels).Return(result, nil)
}

type mockSecret struct {
	Name       string
	SecretType string
	Key        string
	Value      []byte
}

func (s *SecretClientMock) mockGetByLabelsSimplified(namespace domain.Namespace, labels map[string]string, results []mockSecret) {
	secretItems := make([]corev1.Secret, 0, len(results))
	for i, r := range results {
		key := r.Key
		if key == "" {
			key = k8s.DefaultSecretKeyName
		}

		name := r.Name
		if name == "" {
			name = fmt.Sprintf("secret-%d", i)
		}

		// copy labels parameter and add LabelSecretType if missing
		secretLabels := make(map[string]string)
		for k, v := range labels {
			secretLabels[k] = v
		}
		if _, ok := secretLabels[k8s.LabelSecretType]; !ok {
			secretLabels[k8s.LabelSecretType] = r.SecretType
		}

		secretItems = append(secretItems, corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: string(namespace),
				Labels:    secretLabels,
			},
			Data: map[string][]byte{
				key: r.Value,
			},
		})
	}

	secretList := &corev1.SecretList{Items: secretItems}
	s.mockGetByLabels(namespace, labels, secretList)
}

func (s *SecretClientMock) mockGetByLabelsSimple(namespace domain.Namespace, labels map[string]string, key string, value []byte) {
	result := &corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{key: value}}}}
	s.mockGetByLabels(namespace, labels, result)
}

func (s *SecretClientMock) Delete(ctx context.Context, namespacedName domain.NamespacedName) error {
	args := s.Called(ctx, namespacedName)
	return args.Error(0)
}

func (s *SecretClientMock) DeleteByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) error {
	args := s.Called(ctx, namespace, labels)
	return args.Error(0)
}

func (s *SecretClientMock) mockDeleteByLabels(namespace domain.Namespace, labels map[string]string) {
	s.On("DeleteByLabels", mock.Anything, namespace, labels).Return(nil)
}

func (s *SecretClientMock) Label(ctx context.Context, namespacedName domain.NamespacedName, labels map[string]string) error {
	args := s.Called(ctx, namespacedName, labels)
	return args.Error(0)
}

func (s *SecretClientMock) mockLabel(namespacedName domain.NamespacedName, labels map[string]string) {
	s.On("Label", mock.Anything, namespacedName, labels).Return(nil)
}

func (s *SecretClientMock) mockLabelError(namespacedName domain.NamespacedName, labels map[string]string, err error) {
	s.On("Label", mock.Anything, namespacedName, labels).Return(err)
}

var _ ports.SecretClient = (*SecretClientMock)(nil)

/* ****************************************************
* NATS Client
*****************************************************/

func NewNatsClientMock() *NatsClientMock {
	return &NatsClientMock{}
}

type NatsClientMock struct {
	mock.Mock
}

func (n *NatsClientMock) Connect(natsURL string, userCreds domain.NatsUserCreds) (ports.NatsConnection, error) {
	args := n.Called(natsURL, userCreds)
	return args.Get(0).(ports.NatsConnection), args.Error(1)
}

func (n *NatsClientMock) mockConnect(natsURL string, userCreds domain.NatsUserCreds, result ports.NatsConnection) {
	n.On("Connect", natsURL, userCreds).Return(result, nil)
}

var _ ports.NatsClient = (*NatsClientMock)(nil)

func NewNatsConnectionMock() *NatsConnectionMock {
	return &NatsConnectionMock{}
}

type NatsConnectionMock struct {
	mock.Mock
}

func (n *NatsConnectionMock) LookupAccountJWT(accountID string) (string, error) {
	args := n.Called(accountID)
	return args.String(0), args.Error(1)
}

func (n *NatsConnectionMock) mockLookupAccountJWT(accountID, result string) {
	n.On("LookupAccountJWT", accountID).Return(result, nil)
}

func (n *NatsConnectionMock) HasAccount(accountID string) (bool, error) {
	args := n.Called(accountID)
	return args.Bool(0), args.Error(1)
}

func (n *NatsConnectionMock) EnsureConnected() error {
	args := n.Called()
	return args.Error(0)
}

func (n *NatsConnectionMock) Disconnect() {
	n.Called()
}

func (n *NatsConnectionMock) mockDisconnect() {
	n.On("Disconnect").Return()
}

func (n *NatsConnectionMock) UploadAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

func (n *NatsConnectionMock) mockUploadAccountJWTCatch(catch func(jwt string)) {
	n.On("UploadAccountJWT", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			jwt := args.String(0)
			catch(jwt)
		})
}

func (n *NatsConnectionMock) DeleteAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

func (n *NatsConnectionMock) mockDeleteAccountJWTCatch(catch func(jwt string)) {
	n.On("DeleteAccountJWT", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			jwt := args.String(0)
			catch(jwt)
		})
}

var _ ports.NatsConnection = (*NatsConnectionMock)(nil)

/* ****************************************************
* ports.AccountReader Resolver
*****************************************************/

type AccountReaderMock struct {
	mock.Mock
}

func NewAccountReaderMock() *AccountReaderMock {
	return &AccountReaderMock{}
}

func (a *AccountReaderMock) Get(ctx context.Context, accountRef domain.NamespacedName) (account *v1alpha1.Account, err error) {
	args := a.Called(ctx, accountRef)
	return args.Get(0).(*v1alpha1.Account), args.Error(1)
}

func (a *AccountReaderMock) mockGet(ctx context.Context, accountRef domain.NamespacedName, result *v1alpha1.Account) {
	a.On("Get", ctx, accountRef).Return(result, nil)
}

func (a *AccountReaderMock) mockGetCallback(ctx interface{}, accountRef interface{}, generator func(accountRef domain.NamespacedName) (*v1alpha1.Account, error)) *mock.Call {
	call := a.On("Get", ctx, accountRef)
	call.RunFn = func(args mock.Arguments) {
		call.Return(generator(args.Get(1).(domain.NamespacedName)))
	}
	return call
}

var _ ports.AccountReader = &AccountReaderMock{}

/* ****************************************************
* NatsCluster Resolver
*****************************************************/
type NatsClusterReaderMock struct {
	mock.Mock
}

func NewNatsClusterReaderMock() *NatsClusterReaderMock {
	return &NatsClusterReaderMock{}
}

func (m *NatsClusterReaderMock) Get(ctx context.Context, clusterRef domain.NamespacedName) (*v1alpha1.NatsCluster, error) {
	args := m.Called(ctx, clusterRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*v1alpha1.NatsCluster), args.Error(1)
}

func (m *NatsClusterReaderMock) mockGetNatsCluster(ctx context.Context, clusterRef domain.NamespacedName, result *v1alpha1.NatsCluster) {
	m.On("Get", ctx, clusterRef).Return(result, nil)
}

func (m *NatsClusterReaderMock) mockGetNatsClusterError(ctx context.Context, clusterRef domain.NamespacedName, err error) {
	m.On("Get", ctx, clusterRef).Return(nil, err)
}

var _ ports.NatsClusterReader = (*NatsClusterReaderMock)(nil)

/* ****************************************************
* ports.ConfigMapReader Mock
*****************************************************/
type ConfigMapReaderMock struct {
	mock.Mock
}

func NewConfigMapReaderMock() *ConfigMapReaderMock {
	return &ConfigMapReaderMock{}
}

func (m *ConfigMapReaderMock) Get(ctx context.Context, namespacedName domain.NamespacedName) (map[string]string, error) {
	args := m.Called(ctx, namespacedName)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *ConfigMapReaderMock) mockGet(ctx context.Context, namespacedName domain.NamespacedName, result map[string]string) {
	m.On("Get", ctx, namespacedName).Return(result, nil)
}

var _ ports.ConfigMapReader = (*ConfigMapReaderMock)(nil)
