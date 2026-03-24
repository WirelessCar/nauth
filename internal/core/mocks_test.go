package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/jwt/v2"
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

func (s *SecretClientMock) mockApplyWithCatch(ctx interface{}, owner interface{}, meta interface{}, valueMap interface{}, catcher func(map[string]string)) {
	s.On("Apply", ctx, owner, meta, valueMap).
		Run(func(args mock.Arguments) {
			catcher(args.Get(3).(map[string]string))
		}).
		Return(nil)
}

func (s *SecretClientMock) Get(ctx context.Context, namespacedName domain.NamespacedName) (map[string]string, bool, error) {
	args := s.Called(ctx, namespacedName)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(map[string]string), args.Bool(1), args.Error(2)
}

func (s *SecretClientMock) mockGet(ctx context.Context, namespacedName domain.NamespacedName, result map[string]string) {
	s.On("Get", ctx, namespacedName).Return(result, true, nil)
}

func (s *SecretClientMock) mockGetNotFound(namespacedName domain.NamespacedName) {
	s.On("Get", mock.Anything, namespacedName).Return(nil, false, nil)
}

func (s *SecretClientMock) mockGetError(namespacedName domain.NamespacedName, err error) {
	s.On("Get", mock.Anything, namespacedName).Return(nil, false, err)
}

func (s *SecretClientMock) GetByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) (*corev1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*corev1.SecretList), args.Error(1)
}

func (s *SecretClientMock) mockGetByLabels(namespace domain.Namespace, labels map[string]string, result *corev1.SecretList) {
	s.On("GetByLabels", mock.Anything, namespace, labels).Return(result, nil)
}

func (s *SecretClientMock) mockGetByLabelsError(namespace domain.Namespace, labels map[string]string, err error) {
	s.On("GetByLabels", mock.Anything, namespace, labels).Return(&corev1.SecretList{}, err)
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

func (s *SecretClientMock) Delete(ctx context.Context, namespacedName domain.NamespacedName) error {
	args := s.Called(ctx, namespacedName)
	return args.Error(0)
}

func (s *SecretClientMock) mockDelete(ctx context.Context, namespacedName domain.NamespacedName) {
	s.On("Delete", ctx, namespacedName).Return(nil)
}

func (s *SecretClientMock) mockDeleteError(ctx context.Context, namespacedName domain.NamespacedName, err error) {
	s.On("Delete", ctx, namespacedName).Return(err)
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

var _ outbound.SecretClient = (*SecretClientMock)(nil)

/* ****************************************************
* User JWT Signer
*****************************************************/

func NewUserJWTSignerMock() *UserJWTSignerMock {
	return &UserJWTSignerMock{}
}

type UserJWTSignerMock struct {
	mock.Mock
}

func (m *UserJWTSignerMock) SignUserJWT(ctx context.Context, accountRef domain.NamespacedName, claims *jwt.UserClaims) (*SignedUserJWT, error) {
	args := m.Called(ctx, accountRef, claims)
	return args.Get(0).(*SignedUserJWT), args.Error(1)
}

func (m *UserJWTSignerMock) mockSignUserJWT(ctx context.Context, accountRef domain.NamespacedName, callback func(claims *jwt.UserClaims) *SignedUserJWT) {
	call := m.On("SignUserJWT", ctx, accountRef, mock.Anything)
	call.RunFn = func(args mock.Arguments) {
		claims := args.Get(2).(*jwt.UserClaims)
		call.Return(callback(claims), nil)
	}
}

var _ UserJWTSigner = (*UserJWTSignerMock)(nil)

/* ****************************************************
* outbound.NatsSysClient mock
*****************************************************/

func NewNatsSysClientMock() *NatsSysClientMock {
	return &NatsSysClientMock{}
}

type NatsSysClientMock struct {
	mock.Mock
}

func (n *NatsSysClientMock) Connect(natsURL string, userCreds domain.NatsUserCreds) (outbound.NatsSysConnection, error) {
	args := n.Called(natsURL, userCreds)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(outbound.NatsSysConnection), args.Error(1)
}

func (n *NatsSysClientMock) mockConnect(natsURL string, userCreds domain.NatsUserCreds, result outbound.NatsSysConnection) *mock.Call {
	return n.On("Connect", natsURL, userCreds).Return(result, nil)
}

func (n *NatsSysClientMock) mockConnectError(natsURL string, userCreds domain.NatsUserCreds, err error) {
	n.On("Connect", natsURL, userCreds).Return(nil, err)
}

var _ outbound.NatsSysClient = (*NatsSysClientMock)(nil)

/* ****************************************************
* outbound.NatsSysConnection mock
*****************************************************/

func NewNatsSysConnectionMock() *NatsSysConnectionMock {
	return &NatsSysConnectionMock{}
}

type NatsSysConnectionMock struct {
	mock.Mock
}

func (n *NatsSysConnectionMock) LookupAccountJWT(accountID string) (string, error) {
	args := n.Called(accountID)
	return args.String(0), args.Error(1)
}

func (n *NatsSysConnectionMock) mockLookupAccountJWT(accountID, result string) {
	n.On("LookupAccountJWT", accountID).Return(result, nil)
}

func (n *NatsSysConnectionMock) HasAccount(accountID string) (bool, error) {
	args := n.Called(accountID)
	return args.Bool(0), args.Error(1)
}

func (n *NatsSysConnectionMock) EnsureConnected() error {
	args := n.Called()
	return args.Error(0)
}

func (n *NatsSysConnectionMock) VerifySystemAccountAccess() error {
	args := n.Called()
	return args.Error(0)
}

func (n *NatsSysConnectionMock) mockVerifySystemAccountAccess() {
	n.On("VerifySystemAccountAccess").Return(nil)
}

func (n *NatsSysConnectionMock) mockVerifySystemAccountAccessError(err error) {
	n.On("VerifySystemAccountAccess").Return(err)
}

func (n *NatsSysConnectionMock) Disconnect() {
	n.Called()
}

func (n *NatsSysConnectionMock) mockDisconnect() *mock.Call {
	return n.On("Disconnect").Return()
}

func (n *NatsSysConnectionMock) UploadAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

func (n *NatsSysConnectionMock) mockUploadAccountJWTCatch(catch func(jwt string)) {
	n.On("UploadAccountJWT", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			catch(args.String(0))
		})
}

func (n *NatsSysConnectionMock) DeleteAccountJWT(jwt string) error {
	args := n.Called(jwt)
	return args.Error(0)
}

func (n *NatsSysConnectionMock) mockDeleteAccountJWTCatch(catch func(jwt string)) *mock.Call {
	return n.On("DeleteAccountJWT", mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			catch(args.String(0))
		})
}

var _ outbound.NatsSysConnection = (*NatsSysConnectionMock)(nil)

/* ********
* outbound.NatsAccountClient mock
 */

func NewNatsAccountClientMock() *NatsAccountClientMock {
	return &NatsAccountClientMock{}
}

type NatsAccountClientMock struct {
	mock.Mock
}

func (n *NatsAccountClientMock) Connect(natsURL string, userCreds domain.NatsUserCreds) (outbound.NatsAccountConnection, error) {
	args := n.Called(natsURL, userCreds)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(outbound.NatsAccountConnection), args.Error(1)
}

func (n *NatsAccountClientMock) mockConnectMatchingCreds(natsURL string, credsMatcher func(userCreds domain.NatsUserCreds) bool, result outbound.NatsAccountConnection) *mock.Call {
	return n.On("Connect", natsURL, mock.MatchedBy(credsMatcher)).Return(result, nil)
}

var _ outbound.NatsAccountClient = (*NatsAccountClientMock)(nil)

/* ****************************************************
* outbound.NatsAccountConnection mock
*****************************************************/

func NewNatsAccountConnectionMock() *NatsAccConnectionMock {
	return &NatsAccConnectionMock{}
}

type NatsAccConnectionMock struct {
	mock.Mock
}

func (n *NatsAccConnectionMock) Disconnect() {
	n.Called()
}

func (n *NatsAccConnectionMock) mockDisconnect() *mock.Call {
	return n.On("Disconnect").Return()
}

func (n *NatsAccConnectionMock) EnsureConnected() error {
	args := n.Called()
	return args.Error(0)
}

func (n *NatsAccConnectionMock) ListAccountStreams() ([]string, error) {
	args := n.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (n *NatsAccConnectionMock) mockListAccountStreams(result []string) *mock.Call {
	return n.On("ListAccountStreams").Return(result, nil)
}

var _ outbound.NatsAccountConnection = (*NatsAccConnectionMock)(nil)

/* ****************************************************
* outbound.AccountReader Resolver
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

var _ outbound.AccountReader = &AccountReaderMock{}

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

var _ outbound.NatsClusterReader = (*NatsClusterReaderMock)(nil)

/* ****************************************************
* outbound.ConfigMapReader Mock
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

func (m *ConfigMapReaderMock) mockGetError(ctx context.Context, namespacedName domain.NamespacedName, err error) {
	m.On("Get", ctx, namespacedName).Return(map[string]string{}, err)
}

var _ outbound.ConfigMapReader = (*ConfigMapReaderMock)(nil)
