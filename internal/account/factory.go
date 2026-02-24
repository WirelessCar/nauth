package account

import (
	"context"
	"fmt"

	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	natsc "github.com/WirelessCar/nauth/internal/nats"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerFactory struct {
	reader          client.Reader
	accounts        AccountGetter
	secretClient    SecretClient
	configmapClient ConfigMapClient
	nauthNamespace  string
	defaultNatsURL  string
}

func NewManagerFactory(
	reader client.Reader,
	accounts AccountGetter,
	secretClient SecretClient,
	configmapClient ConfigMapClient,
	nauthNamespace string,
	defaultNatsURL string,
) *ManagerFactory {
	return &ManagerFactory{
		reader:          reader,
		accounts:        accounts,
		secretClient:    secretClient,
		configmapClient: configmapClient,
		nauthNamespace:  nauthNamespace,
		defaultNatsURL:  defaultNatsURL,
	}
}

func (f *ManagerFactory) ForAccount(ctx context.Context, acct *nauthv1alpha1.Account) (controller.AccountManager, error) {
	mgrOpts := make([]func(*Manager), 0, 2)
	if f.nauthNamespace != "" {
		mgrOpts = append(mgrOpts, WithNamespace(f.nauthNamespace))
	}

	clusterRef := acct.Spec.NatsClusterRef
	if clusterRef == nil {
		natsClient := natsc.NewClient(f.defaultNatsURL, f.secretClient)
		return NewManager(f.accounts, natsClient, f.secretClient, mgrOpts...), nil
	}

	cluster, err := f.resolveNatsClusterForAccount(ctx, clusterRef, acct.GetNamespace())
	if err != nil {
		return nil, err
	}

	natsURL, err := f.resolveNatsURL(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve NATS URL for NatsCluster %s/%s: %w", cluster.GetNamespace(), cluster.GetName(), err)
	}

	systemCredsSecretRef := &natsc.SecretRef{
		Namespace: cluster.GetNamespace(),
		Name:      cluster.Spec.SystemAccountUserCredsSecretRef.Name,
		Key:       cluster.Spec.SystemAccountUserCredsSecretRef.Key,
	}

	natsClient := natsc.NewClientWithSecretRef(natsURL, f.secretClient, systemCredsSecretRef)
	mgrOpts = append(mgrOpts, WithNatsCluster(cluster))
	return NewManager(f.accounts, natsClient, f.secretClient, mgrOpts...), nil
}

func (f *ManagerFactory) resolveNatsClusterForAccount(
	ctx context.Context,
	clusterRef *nauthv1alpha1.NatsClusterRef,
	nsDefault string,
) (*nauthv1alpha1.NatsCluster, error) {
	if clusterRef.APIVersion != "" && clusterRef.APIVersion != nauthv1alpha1.GroupVersion.String() {
		return nil, fmt.Errorf("unsupported NatsCluster apiVersion %q, expected %q", clusterRef.APIVersion, nauthv1alpha1.GroupVersion.String())
	}
	if clusterRef.Kind != "" && clusterRef.Kind != "NatsCluster" {
		return nil, fmt.Errorf("unsupported NatsCluster kind %q", clusterRef.Kind)
	}

	ns := clusterRef.Namespace
	if ns == "" {
		ns = nsDefault
	}

	cluster := &nauthv1alpha1.NatsCluster{}
	if err := f.reader.Get(ctx, types.NamespacedName{Name: clusterRef.Name, Namespace: ns}, cluster); err != nil {
		return nil, fmt.Errorf("failed to get NatsCluster %s/%s: %w", ns, clusterRef.Name, err)
	}

	return cluster, nil
}

func (f *ManagerFactory) resolveNatsURL(ctx context.Context, cluster *nauthv1alpha1.NatsCluster) (string, error) {
	if cluster.Spec.URL != "" {
		return cluster.Spec.URL, nil
	}

	if cluster.Spec.URLFrom == nil {
		return "", fmt.Errorf("neither url nor urlFrom is set")
	}

	urlFromRef := cluster.Spec.URLFrom
	namespace := urlFromRef.Namespace
	if namespace == "" {
		namespace = cluster.GetNamespace()
	}

	switch urlFromRef.Kind {
	case nauthv1alpha1.URLFromKindConfigMap:
		data, err := f.configmapClient.Get(ctx, namespace, urlFromRef.Name)
		if err != nil {
			return "", fmt.Errorf("get ConfigMap %s/%s: %w", namespace, urlFromRef.Name, err)
		}
		if value, ok := data[urlFromRef.Key]; ok {
			return value, nil
		}
		return "", fmt.Errorf("configMap %s/%s has no key %q", namespace, urlFromRef.Name, urlFromRef.Key)
	case nauthv1alpha1.URLFromKindSecret:
		data, err := f.secretClient.Get(ctx, namespace, urlFromRef.Name)
		if err != nil {
			return "", fmt.Errorf("get Secret %s/%s: %w", namespace, urlFromRef.Name, err)
		}
		if value, ok := data[urlFromRef.Key]; ok {
			return value, nil
		}
		return "", fmt.Errorf("secret %s/%s has no key %q", namespace, urlFromRef.Name, urlFromRef.Key)
	default:
		return "", fmt.Errorf("unsupported urlFrom.kind %q", urlFromRef.Kind)
	}
}

// Compile-time assertion that ManagerFactory implements the controller.AccountManagerFactory interface
var _ controller.AccountManagerFactory = (*ManagerFactory)(nil)
