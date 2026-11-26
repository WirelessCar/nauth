package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nkeys"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaultCABundleKey is the caRef key used when none is set. It matches the key cert-manager writes.
const defaultCABundleKey = "ca.crt"

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
	target, err := nauth.NewClusterTarget(string(cluster.UID), natsURL, *sysAdminCreds, opSigningKey)
	if err != nil {
		return nil, fmt.Errorf("create cluster target for NatsCluster %s: %w", clusterRef, err)
	}
	if err = target.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cluster target resolved for NatsCluster %s: %w", clusterRef, err)
	}
	tlsConfig, err := c.resolveTLS(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve TLS for NatsCluster %s: %w", clusterRef, err)
	}
	target.TLS = tlsConfig
	return target, nil
}

// resolveTLS reads spec.tls material from the NatsCluster's namespace. It returns nil when spec.tls is unset.
func (c *ClusterClient) resolveTLS(ctx context.Context, cluster *v1alpha1.NatsCluster) (*domain.NatsTLSConfig, error) {
	spec := cluster.Spec.TLS
	if spec == nil {
		return nil, nil
	}
	result := &domain.NatsTLSConfig{HandshakeFirst: spec.HandshakeFirst}

	if spec.CARef != nil {
		bundle, source, err := c.resolveCABundle(ctx, cluster.GetNamespace(), spec.CARef)
		if err != nil {
			return nil, fmt.Errorf("resolve caRef: %w", err)
		}
		if _, _, err := domain.ParseCABundle(bundle); err != nil {
			return nil, fmt.Errorf("resolve caRef: %s: %w", source, err)
		}
		result.CABundlePEM = bundle
	}

	if spec.ClientCertSecretRef != nil {
		secretRef := domain.NewNamespacedName(cluster.GetNamespace(), spec.ClientCertSecretRef.Name)
		certPEM, err := c.resolveSecret(ctx, secretRef, corev1.TLSCertKey)
		if err != nil {
			return nil, fmt.Errorf("resolve clientCertSecretRef: %w", err)
		}
		keyPEM, err := c.resolveSecret(ctx, secretRef, corev1.TLSPrivateKeyKey)
		if err != nil {
			return nil, fmt.Errorf("resolve clientCertSecretRef: %w", err)
		}
		// A referenced Secret must hold a real pair. Empty values would otherwise read as "no client
		// certificate" downstream and the connection would proceed without presenting one.
		if len(certPEM) == 0 || len(keyPEM) == 0 {
			return nil, fmt.Errorf("resolve clientCertSecretRef: secret %s: %s and %s must not be empty",
				secretRef, corev1.TLSCertKey, corev1.TLSPrivateKeyKey)
		}
		result.ClientCertPEM = certPEM
		result.ClientKeyPEM = keyPEM
		if err := (&domain.NatsTLSConfig{ClientCertPEM: certPEM, ClientKeyPEM: keyPEM}).Validate(); err != nil {
			return nil, fmt.Errorf("resolve clientCertSecretRef: secret %s: %w", secretRef, err)
		}
	}

	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

// resolveCABundle reads the PEM CA bundle that ref points to in namespace. It also returns a
// description of the source, such as `configMap ns/name key "ca.crt"`, for error messages.
func (c *ClusterClient) resolveCABundle(ctx context.Context, namespace string, ref *v1alpha1.TLSCAReference) ([]byte, string, error) {
	resourceRef := domain.NewNamespacedName(namespace, ref.Name)
	key := caBundleKey(ref.Key)
	switch ref.Kind {
	case v1alpha1.TLSCARefKindConfigMap:
		source := fmt.Sprintf("configMap %s key %q", resourceRef, key)
		data, err := c.configMapReader.Get(ctx, resourceRef)
		if err != nil {
			return nil, source, fmt.Errorf("get ConfigMap %s: %w", resourceRef, err)
		}
		value, ok := data[key]
		if !ok {
			return nil, source, fmt.Errorf("configMap %s has no key %q", resourceRef, key)
		}
		return []byte(value), source, nil
	case v1alpha1.TLSCARefKindSecret:
		value, err := c.resolveSecret(ctx, resourceRef, key)
		return value, fmt.Sprintf("secret %s key %q", resourceRef, key), err
	default:
		return nil, "", fmt.Errorf("unsupported caRef.kind %q", ref.Kind)
	}
}

// caBundleKey returns key, or defaultCABundleKey when key is empty.
func caBundleKey(key string) string {
	if key == "" {
		return defaultCABundleKey
	}
	return key
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
