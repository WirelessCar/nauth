package account

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

func NewSecretClientMock() *SecretClientMock {
	return &SecretClientMock{}
}

type SecretClientMock struct {
	mock.Mock
}

func (s *SecretClientMock) Apply(ctx context.Context, secretOwner *ports.Owner, meta metav1.ObjectMeta, valueMap map[string]string) error {
	args := s.Called(ctx, secretOwner, meta, valueMap)
	return args.Error(0)
}

func (s *SecretClientMock) Get(ctx context.Context, namespace string, name string) (map[string]string, error) {
	args := s.Called(ctx, namespace, name)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (s *SecretClientMock) OnGetReturn(ctx context.Context, namespace string, name string, result map[string]string) *mock.Call {
	return s.On("Get", ctx, namespace, name).Return(result, nil)
}

func (s *SecretClientMock) GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*corev1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*corev1.SecretList), args.Error(1)
}

func (s *SecretClientMock) OnGetByLabelsReturn(namespace string, labels map[string]string, result *corev1.SecretList) *mock.Call {
	return s.On("GetByLabels", mock.Anything, namespace, labels).Return(result, nil)
}

func (s *SecretClientMock) OnGetByLabelsReturnSimple(namespace string, labels map[string]string, key string, value []byte) *mock.Call {
	result := &corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{key: value}}}}
	return s.OnGetByLabelsReturn(namespace, labels, result)
}

func (s *SecretClientMock) Delete(ctx context.Context, namespace string, name string) error {
	args := s.Called(ctx, namespace, name)
	return args.Error(0)
}

func (s *SecretClientMock) DeleteByLabels(ctx context.Context, namespace string, labels map[string]string) error {
	args := s.Called(ctx, namespace, labels)
	return args.Error(0)
}

func (s *SecretClientMock) Label(ctx context.Context, namespace, name string, labels map[string]string) error {
	args := s.Called(ctx, namespace, name, labels)
	return args.Error(0)
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

func (n *NatsClientMock) Connect(natsURL string, userCreds ports.NatsUserCreds) (ports.NatsConnection, error) {
	args := n.Called(natsURL, userCreds)
	return args.Get(0).(ports.NatsConnection), args.Error(1)
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

func (n *NatsConnectionMock) UploadAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

func (n *NatsConnectionMock) DeleteAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

var _ ports.NatsConnection = (*NatsConnectionMock)(nil)

/* ****************************************************
* Account Resolver
*****************************************************/

type AccountResolverMock struct {
	mock.Mock
}

func NewAccountResolverMock() *AccountResolverMock {
	return &AccountResolverMock{}
}

// Get implements ports.AccountGetter.
func (a *AccountResolverMock) Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error) {
	args := a.Called(ctx, accountRefName, namespace)
	anAccount := args.Get(0).(v1alpha1.Account)
	return &anAccount, args.Error(1)
}

var _ ports.NauthAccountResolver = &AccountResolverMock{}

/* ****************************************************
* NatsCluster Resolver
*****************************************************/
type NatsClusterResolverMock struct {
	mock.Mock
}

func NewNatsClusterResolverMock() *NatsClusterResolverMock {
	return &NatsClusterResolverMock{}
}

func (m *NatsClusterResolverMock) GetNatsCluster(ctx context.Context, clusterRef ports.NamespacedName) (*v1alpha1.NatsCluster, error) {
	args := m.Called(ctx, clusterRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*v1alpha1.NatsCluster), args.Error(1)
}

func (m *NatsClusterResolverMock) OnGetNatsClusterReturn(ctx context.Context, clusterRef ports.NamespacedName, result *v1alpha1.NatsCluster) *mock.Call {
	return m.On("GetNatsCluster", ctx, clusterRef).Return(result, nil)
}

func (m *NatsClusterResolverMock) OnGetNatsClusterReturnError(ctx context.Context, clusterRef ports.NamespacedName, err error) *mock.Call {
	return m.On("GetNatsCluster", ctx, clusterRef).Return(nil, err)
}

var _ ports.NauthNatsClusterResolver = (*NatsClusterResolverMock)(nil)

/* ****************************************************
* ConfigMap Resolver
*****************************************************/
type ConfigMapResolverMock struct {
	mock.Mock
}

func NewConfigMapResolverMock() *ConfigMapResolverMock {
	return &ConfigMapResolverMock{}
}

func (m *ConfigMapResolverMock) Get(ctx context.Context, namespace string, name string) (map[string]string, error) {
	args := m.Called(ctx, namespace, name)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *ConfigMapResolverMock) OnGetReturn(ctx context.Context, namespace string, name string, result map[string]string) *mock.Call {
	return m.On("Get", ctx, namespace, name).Return(result, nil)
}

var _ ports.ConfigMapResolver = (*ConfigMapResolverMock)(nil)
