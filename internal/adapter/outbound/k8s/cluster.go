package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nkeys"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterClient struct {
	k8sReader       client.Reader
	secretReader    outbound.SecretReader
	configMapReader outbound.ConfigMapReader
}

func NewClusterClient(
	k8sReader client.Reader,
	secretReader outbound.SecretReader,
	configMapReader outbound.ConfigMapReader,
) *ClusterClient {
	return &ClusterClient{
		k8sReader:       k8sReader,
		secretReader:    secretReader,
		configMapReader: configMapReader,
	}
}

func (c *ClusterClient) GetTarget(ctx context.Context, clusterRef nauth.ClusterRef) (*nauth.ClusterTarget, error) {
	namespacedNameRef, err := clusterRef.AsNamespacedName()
	if err != nil {
		return nil, fmt.Errorf("invalid cluster reference %q (expected NamespacedName): %w", clusterRef, err)
	}

	cluster := &v1alpha1.NatsCluster{}
	if err = c.k8sReader.Get(ctx, client.ObjectKey{Namespace: namespacedNameRef.Namespace, Name: namespacedNameRef.Name}, cluster); err != nil {
		return nil, fmt.Errorf("failed getting NatsCluster resource %s: %w", namespacedNameRef.String(), err)
	}

	return c.ResolveClusterTarget(ctx, cluster)
}

// ResolveClusterTarget implements the controller.ClusterResolver interface.
func (c *ClusterClient) ResolveClusterTarget(ctx context.Context, cluster *v1alpha1.NatsCluster) (*nauth.ClusterTarget, error) {
	clusterRef := domain.NewNamespacedName(cluster.GetNamespace(), cluster.GetName())
	natsURL, err := c.resolveNatsURL(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve NATS URL for NatsCluster %s: %w", clusterRef, err)
	}
	sysAdminCreds, err := c.resolveSysAdminCreds(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve system account user creds for NatsCluster %s: %w", clusterRef, err)
	}
	opSigningKey, err := c.resolveOperatorSigningKey(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve operator signing key for NatsCluster %s: %w", clusterRef, err)
	}
	target, err := nauth.NewClusterTarget(natsURL, *sysAdminCreds, opSigningKey)
	if err != nil {
		return nil, fmt.Errorf("create cluster target for NatsCluster %s: %w", clusterRef, err)
	}
	return target, nil
}

func (c *ClusterClient) resolveSysAdminCreds(ctx context.Context, cluster *v1alpha1.NatsCluster) (*domain.NatsUserCreds, error) {
	secretKeyRef := cluster.Spec.SystemAccountUserCredsSecretRef
	secretRef := domain.NewNamespacedName(cluster.GetNamespace(), secretKeyRef.Name)
	creds, err := c.resolveSecret(ctx, secretRef, secretKeyRef.Key)
	if err != nil {
		return nil, err
	}
	userCreds, err := domain.NewNatsUserCreds(creds)
	if err != nil {
		return nil, fmt.Errorf("invalid user creds: %w", err)
	}
	return userCreds, nil
}

func (c *ClusterClient) resolveOperatorSigningKey(ctx context.Context, cluster *v1alpha1.NatsCluster) (domain.NatsOperatorSigningKey, error) {
	secretKeyRef := cluster.Spec.OperatorSigningKeySecretRef
	secretRef := domain.NewNamespacedName(cluster.GetNamespace(), secretKeyRef.Name)
	keyData, err := c.resolveSecret(ctx, secretRef, secretKeyRef.Key)
	if err != nil {
		return nil, err
	}
	opSigningKey, err := nkeys.FromSeed(keyData)
	if err != nil {
		return nil, fmt.Errorf("invalid operator signing key: %w", err)
	}
	return opSigningKey, nil
}

func (c *ClusterClient) resolveSecret(ctx context.Context, namespacedName domain.NamespacedName, key string) ([]byte, error) {
	secretData, found, err := c.secretReader.Get(ctx, namespacedName)
	if err != nil {
		return nil, fmt.Errorf("resolve secret %s: %w", namespacedName, err)
	}
	if !found {
		return nil, fmt.Errorf("secret %s not found", namespacedName)
	}

	if key == "" {
		key = DefaultSecretKeyName
	}

	value, ok := secretData[key]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %q", namespacedName, key)
	}

	return []byte(value), nil
}

func (c *ClusterClient) resolveNatsURL(ctx context.Context, cluster *v1alpha1.NatsCluster) (string, error) {
	url := cluster.Spec.URL
	urlFrom := ""
	if cluster.Spec.URLFrom != nil {
		var err error
		urlFrom, err = c.resolveNatsURLFromResource(ctx, cluster.Spec.URLFrom, cluster.GetNamespace())
		if err != nil {
			return "", fmt.Errorf("resolve NATS URL from urlFrom reference: %w", err)
		}
		if url != "" && urlFrom != url {
			return "", fmt.Errorf("ambiguous NATS URL, url and urlFrom reference resolve to different URLs (%q vs %q)", url, urlFrom)
		}
		return urlFrom, nil
	}
	if url == "" {
		return "", fmt.Errorf("NATS URL must be specified via url or urlFrom")
	}
	return url, nil
}

func (c *ClusterClient) resolveNatsURLFromResource(ctx context.Context, urlFromRef *v1alpha1.URLFromReference, fallbackNamespace string) (string, error) {
	resourceRef := domain.NewNamespacedName(urlFromRef.Namespace, urlFromRef.Name)
	if resourceRef.Namespace == "" {
		resourceRef.Namespace = fallbackNamespace
	}
	if err := resourceRef.Validate(); err != nil {
		return "", fmt.Errorf("invalid resource reference for urlFrom: %w", err)
	}

	switch urlFromRef.Kind {
	case v1alpha1.URLFromKindConfigMap:
		data, err := c.configMapReader.Get(ctx, resourceRef)
		if err != nil {
			return "", fmt.Errorf("get ConfigMap %s: %w", resourceRef, err)
		}
		if natsURL, ok := data[urlFromRef.Key]; ok {
			return natsURL, nil
		}
		return "", fmt.Errorf("configMap %s has no key %q", resourceRef, urlFromRef.Key)
	case v1alpha1.URLFromKindSecret:
		data, err := c.resolveSecret(ctx, resourceRef, urlFromRef.Key)
		if err != nil {
			return "", fmt.Errorf("get URL secret: %w", err)
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported urlFrom.kind %q", urlFromRef.Kind)
	}
}

// Compile-time assertion that implementation satisfies the port interface
var _ outbound.ClusterReader = (*ClusterClient)(nil)
