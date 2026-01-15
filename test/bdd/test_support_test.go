package bdd

import (
	"context"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type testSecretClient struct {
	delegate           *secret.Client
	applyErr           error
	deleteErr          error
	deleteByLabelsErr  error
	labelErr           error
	applyCalls         int
	deleteCalls        int
	deleteByLabelsCall int
}

func newTestSecretClient(delegate *secret.Client) *testSecretClient {
	return &testSecretClient{delegate: delegate}
}

func (t *testSecretClient) Apply(ctx context.Context, owner *secret.Owner, meta metav1.ObjectMeta, valueMap map[string]string) error {
	t.applyCalls++
	if t.applyErr != nil {
		return t.applyErr
	}
	return t.delegate.Apply(ctx, owner, meta, valueMap)
}

func (t *testSecretClient) Get(ctx context.Context, namespace string, name string) (map[string]string, error) {
	return t.delegate.Get(ctx, namespace, name)
}

func (t *testSecretClient) GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*corev1.SecretList, error) {
	return t.delegate.GetByLabels(ctx, namespace, labels)
}

func (t *testSecretClient) Delete(ctx context.Context, namespace string, name string) error {
	t.deleteCalls++
	if t.deleteErr != nil {
		return t.deleteErr
	}
	return t.delegate.Delete(ctx, namespace, name)
}

func (t *testSecretClient) DeleteByLabels(ctx context.Context, namespace string, labels map[string]string) error {
	t.deleteByLabelsCall++
	if t.deleteByLabelsErr != nil {
		return t.deleteByLabelsErr
	}
	return t.delegate.DeleteByLabels(ctx, namespace, labels)
}

func (t *testSecretClient) Label(ctx context.Context, namespace, name string, labels map[string]string) error {
	if t.labelErr != nil {
		return t.labelErr
	}
	return t.delegate.Label(ctx, namespace, name, labels)
}

type fakeNatsClient struct {
	ensureErr   error
	lookupErr   error
	uploadErr   error
	deleteErr   error
	lookupJWT   string
	ensureCalls int
	uploadCalls int
	deleteCalls int
	lookupCalls int
}

func (f *fakeNatsClient) EnsureConnected(namespace string) error {
	f.ensureCalls++
	return f.ensureErr
}

func (f *fakeNatsClient) Disconnect() {}

func (f *fakeNatsClient) LookupAccountJWT(accountID string) (string, error) {
	f.lookupCalls++
	if f.lookupErr != nil {
		return "", f.lookupErr
	}
	return f.lookupJWT, nil
}

func (f *fakeNatsClient) UploadAccountJWT(jwt string) error {
	f.uploadCalls++
	return f.uploadErr
}

func (f *fakeNatsClient) DeleteAccountJWT(jwt string) error {
	f.deleteCalls++
	return f.deleteErr
}

type accountGetterWithNotFound struct {
	delegate *k8s.AccountClient
}

func (a *accountGetterWithNotFound) Get(ctx context.Context, accountRefName string, namespace string) (*natsv1alpha1.Account, error) {
	account, err := a.delegate.Get(ctx, accountRefName, namespace)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, k8s.ErrNoAccountFound
	}
	return account, err
}

func ensureSecret(ctx context.Context, k8sClient client.Client, secret *corev1.Secret) error {
	if err := k8sClient.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}
