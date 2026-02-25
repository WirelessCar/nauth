package account

import (
	"context"
	"fmt"
	"strings"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	natsc "github.com/WirelessCar/nauth/internal/nats"
	k8sval "k8s.io/apimachinery/pkg/api/validation"
)

type ClusterGetter interface {
	GetNatsCluster(ctx context.Context, namespace, name string) (*v1alpha1.NatsCluster, error)
}

type ManagerFactory struct {
	clusters              ClusterGetter
	accounts              AccountGetter
	secretClient          SecretClient
	configmapClient       ConfigMapClient
	defaultNatsClusterRef string
	nauthNamespace        string
	defaultNatsURL        string
}

func NewManagerFactory(
	clusters ClusterGetter,
	accounts AccountGetter,
	secretClient SecretClient,
	configmapClient ConfigMapClient,
	defaultNatsClusterRef string,
	nauthNamespace string,
	defaultNatsURL string,
) *ManagerFactory {
	return &ManagerFactory{
		clusters:              clusters,
		accounts:              accounts,
		secretClient:          secretClient,
		configmapClient:       configmapClient,
		defaultNatsClusterRef: defaultNatsClusterRef,
		nauthNamespace:        nauthNamespace,
		defaultNatsURL:        defaultNatsURL,
	}
}

func (f *ManagerFactory) ForAccount(ctx context.Context, acct *v1alpha1.Account) (controller.AccountManager, error) {
	mgrOpts := make([]func(*Manager), 0, 2)
	if f.nauthNamespace != "" {
		mgrOpts = append(mgrOpts, WithNamespace(f.nauthNamespace))
	}

	clusterRef := acct.Spec.NatsClusterRef
	if clusterRef == nil && f.defaultNatsClusterRef != "" {
		var err error
		clusterRef, err = parseNatsClusterRef(f.defaultNatsClusterRef)
		if err != nil {
			return nil, fmt.Errorf("parse default NATS cluster reference: %w", err)
		}
	}
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
	clusterRef *v1alpha1.NatsClusterRef,
	nsDefault string,
) (*v1alpha1.NatsCluster, error) {
	ns := clusterRef.Namespace
	if ns == "" {
		ns = nsDefault
	}

	cluster, err := f.clusters.GetNatsCluster(ctx, ns, clusterRef.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get NatsCluster %s/%s: %w", ns, clusterRef.Name, err)
	}

	return cluster, nil
}

func (f *ManagerFactory) resolveNatsURL(ctx context.Context, cluster *v1alpha1.NatsCluster) (string, error) {
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
	case v1alpha1.URLFromKindConfigMap:
		data, err := f.configmapClient.Get(ctx, namespace, urlFromRef.Name)
		if err != nil {
			return "", fmt.Errorf("get ConfigMap %s/%s: %w", namespace, urlFromRef.Name, err)
		}
		if value, ok := data[urlFromRef.Key]; ok {
			return value, nil
		}
		return "", fmt.Errorf("configMap %s/%s has no key %q", namespace, urlFromRef.Name, urlFromRef.Key)
	case v1alpha1.URLFromKindSecret:
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

func parseNatsClusterRef(val string) (*v1alpha1.NatsClusterRef, error) {
	var namespace, name string
	switch parts := strings.Split(val, "/"); len(parts) {
	case 1:
		name = parts[0]
	case 2:
		namespace = parts[0]
		name = parts[1]
	default:
		return nil, fmt.Errorf("invalid cluster reference pattern %q, expected [namespace/]name", val)
	}
	if name == "" || (namespace == "" && strings.Contains(val, "/")) {
		return nil, fmt.Errorf("invalid cluster reference pattern %q, expected [namespace/]name", val)
	}
	if errs := k8sval.NameIsDNSSubdomain(name, false); len(errs) > 0 {
		return nil, fmt.Errorf("invalid cluster reference name %q: %s", name, strings.Join(errs, ", "))
	}
	if namespace != "" {
		if errs := k8sval.ValidateNamespaceName(namespace, false); len(errs) > 0 {
			return nil, fmt.Errorf("invalid cluster reference namespace %q: %s", namespace, strings.Join(errs, ", "))
		}
	}

	return &v1alpha1.NatsClusterRef{Name: name, Namespace: namespace}, nil
}

// Compile-time assertion that ManagerFactory implements the controller.AccountManagerFactory interface
var _ controller.AccountManagerFactory = (*ManagerFactory)(nil)
