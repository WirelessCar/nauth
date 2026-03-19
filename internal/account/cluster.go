package account

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/nkeys"
)

type clusterTargetResolver interface {
	GetClusterTarget(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef) (*domain.NatsClusterTarget, error)
}

type clusterTargetResolverImpl struct {
	natsClusterReader ports.NatsClusterReader
	secretReader      ports.SecretReader
	configMapReader   ports.ConfigMapReader
	config            *Config
}

func newClusterTargetResolverImpl(
	natsClusterReader ports.NatsClusterReader,
	secretReader ports.SecretReader,
	configMapReader ports.ConfigMapReader,
	config *Config,
) (*clusterTargetResolverImpl, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.OperatorNatsCluster != nil {
		opClusterRef := domain.NewNamespacedName(
			config.OperatorNatsCluster.ClusterRef.Namespace,
			config.OperatorNatsCluster.ClusterRef.Name,
		)
		if err := opClusterRef.Validate(); err != nil {
			return nil, fmt.Errorf("invalid operator cluster reference: %v", err)
		}
	}

	impl := &clusterTargetResolverImpl{
		natsClusterReader: natsClusterReader,
		secretReader:      secretReader,
		configMapReader:   configMapReader,
		config:            config,
	}
	if err := impl.validate(); err != nil {
		return nil, fmt.Errorf("invalid clusterTargetResolver: %w", err)
	}
	return impl, nil
}

func (r *clusterTargetResolverImpl) validate() error {
	if r.natsClusterReader == nil {
		return fmt.Errorf("natsClusterReader is required")
	}
	if r.secretReader == nil {
		return fmt.Errorf("secretReader is required")
	}
	if r.configMapReader == nil {
		return fmt.Errorf("configMapReader is required")
	}
	if r.config == nil {
		return fmt.Errorf("config is required")
	}
	return nil
}

func (r *clusterTargetResolverImpl) GetClusterTarget(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef) (*domain.NatsClusterTarget, error) {
	var result *domain.NatsClusterTarget
	var err error
	if accountClusterRef != nil {
		acClusterRef := domain.NewNamespacedName(accountClusterRef.Namespace, accountClusterRef.Name)
		if err = acClusterRef.Validate(); err != nil {
			return nil, fmt.Errorf("invalid account cluster reference: %v", err)
		}
		err = r.validateAccountClusterRef(acClusterRef)
		if err != nil {
			return nil, fmt.Errorf("invalid cluster reference: %v", err)
		}
		result, err = r.resolveTarget(ctx, acClusterRef)
	} else if opClusterRef := r.operatorClusterRef(); opClusterRef != nil {
		result, err = r.resolveTarget(ctx, *opClusterRef)
	} else {
		result, err = r.resolveTargetFromImplicitLookup(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve cluster target: %w", err)
	}
	if err = result.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cluster target: %w", err)
	}
	return result, nil
}

func (r *clusterTargetResolverImpl) resolveTarget(ctx context.Context, clusterRef domain.NamespacedName) (*domain.NatsClusterTarget, error) {
	cluster, err := r.natsClusterReader.Get(ctx, clusterRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve NATS cluster %s: %w", clusterRef, err)
	}
	natsURL, err := r.resolveNatsURL(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve NATS URL for NatsCluster %s: %w", clusterRef, err)
	}
	sysAdminCreds, err := r.resolveSysAdminCreds(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve system account user creds for NatsCluster %s: %w", clusterRef, err)
	}
	opSigningKey, err := r.resolveOperatorSigningKey(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("resolve operator signing key for NatsCluster %s: %w", clusterRef, err)
	}
	target, err := domain.NewNatsClusterTarget(natsURL, *sysAdminCreds, opSigningKey)
	if err != nil {
		return nil, fmt.Errorf("create cluster target for NatsCluster %s: %w", clusterRef, err)
	}
	return target, nil
}

// resolveTargetFromImplicitLookup performs a best-effort resolution of cluster connection details based on the presence
// of a default NATS URL and labeled secrets in the operator namespace.
// Deprecated: This method relies on legacy patterns and will sunset in a future release.
func (r *clusterTargetResolverImpl) resolveTargetFromImplicitLookup(ctx context.Context) (*domain.NatsClusterTarget, error) {
	// TODO: [#102][#144] Sunset label-based secret lookup.
	if r.config.DefaultNatsURL == "" {
		return nil, fmt.Errorf("default NATS URL is not configured for implicit cluster lookup")
	}
	if r.config.OperatorNamespace == "" {
		return nil, fmt.Errorf("operator namespace is required for implicit cluster lookup")
	}
	sysAdminCreds, err := r.resolveSysAdminCredsViaLabels(ctx, r.config.OperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("resolve system account user creds via labels: %w", err)
	}
	opSigningKey, err := r.resolveOperatorSigningKeyViaLabels(ctx, r.config.OperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("resolve operator signing key via labels: %w", err)
	}
	target, err := domain.NewNatsClusterTarget(r.config.DefaultNatsURL, *sysAdminCreds, opSigningKey)
	if err != nil {
		return nil, fmt.Errorf("create cluster target from implicit lookup: %w", err)
	}
	return target, nil
}

func (r *clusterTargetResolverImpl) resolveSysAdminCreds(ctx context.Context, cluster *v1alpha1.NatsCluster) (*domain.NatsUserCreds, error) {
	secretKeyRef := cluster.Spec.SystemAccountUserCredsSecretRef
	secretRef := domain.NewNamespacedName(cluster.GetNamespace(), secretKeyRef.Name)
	creds, err := r.resolveSecret(ctx, secretRef, secretKeyRef.Key)
	if err != nil {
		return nil, fmt.Errorf("resolve system account user creds for secret %s/%s: %w", cluster.GetNamespace(), secretRef.Name, err)
	}
	userCreds, err := domain.NewNatsUserCreds(creds)
	if err != nil {
		return nil, fmt.Errorf("invalid system account user creds in secret %s/%s: %w", cluster.GetNamespace(), secretRef.Name, err)
	}
	return userCreds, nil
}

// Deprecated: This method relies on legacy patterns and will sunset in a future release.
func (r *clusterTargetResolverImpl) resolveSysAdminCredsViaLabels(ctx context.Context, namespace domain.Namespace) (*domain.NatsUserCreds, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds,
	}
	creds, err := r.resolveSecretByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, fmt.Errorf("resolve system account user creds via labels in namespace %s: %w", namespace, err)
	}

	userCreds, err := domain.NewNatsUserCreds(creds)
	if err != nil {
		return nil, fmt.Errorf("invalid system account user creds found via labels in namespace %s: %w", namespace, err)
	}
	return userCreds, nil
}

func (r *clusterTargetResolverImpl) resolveOperatorSigningKey(ctx context.Context, cluster *v1alpha1.NatsCluster) (domain.NatsOperatorSigningKey, error) {
	secretKeyRef := cluster.Spec.OperatorSigningKeySecretRef
	secretRef := domain.NewNamespacedName(cluster.GetNamespace(), secretKeyRef.Name)
	keyData, err := r.resolveSecret(ctx, secretRef, secretKeyRef.Key)
	if err != nil {
		return nil, fmt.Errorf("resolve operator signing key for NatsCluster %s/%s: %w", cluster.GetNamespace(), cluster.GetName(), err)
	}
	opSigningKey, err := nkeys.FromSeed(keyData)
	if err != nil {
		return nil, fmt.Errorf("invalid operator signing key for NatsCluster %s/%s: %w", cluster.GetNamespace(), cluster.GetName(), err)
	}
	return opSigningKey, nil
}

// Deprecated: This method relies on legacy patterns and will sunset in a future release.
func (r *clusterTargetResolverImpl) resolveOperatorSigningKeyViaLabels(ctx context.Context, namespace domain.Namespace) (domain.NatsOperatorSigningKey, error) {
	labels := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
	seed, err := r.resolveSecretByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, fmt.Errorf("resolve operator signing key via labels: %w", err)
	}
	keyPair, err := nkeys.FromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("invalid operator signing key in secret found via labels in namespace %s: %w", namespace, err)
	}
	return keyPair, err
}

func (r *clusterTargetResolverImpl) resolveSecretByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) ([]byte, error) {
	secrets, err := r.secretReader.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, fmt.Errorf("get secrets by labels %v in namespace %q: %w", labels, namespace, err)
	}
	if len(secrets.Items) == 0 {
		return nil, fmt.Errorf("no secrets found with labels %v in namespace %q", labels, namespace)
	}
	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("multiple secrets found with labels %v in namespace %q, expected exactly one", labels, namespace)
	}
	value, ok := secrets.Items[0].Data[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s found with labels %v does not contain key %q", namespace, secrets.Items[0].Name, labels, k8s.DefaultSecretKeyName)
	}
	return value, nil
}

func (r *clusterTargetResolverImpl) resolveSecret(ctx context.Context, namespacedName domain.NamespacedName, key string) ([]byte, error) {
	secretData, err := r.secretReader.Get(ctx, namespacedName)
	if err != nil {
		return nil, fmt.Errorf("resolve secret %s: %w", namespacedName, err)
	}

	if key == "" {
		key = k8s.DefaultSecretKeyName
	}

	value, ok := secretData[key]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %q", namespacedName, key)
	}

	return []byte(value), nil
}

func (r *clusterTargetResolverImpl) resolveNatsURL(ctx context.Context, cluster *v1alpha1.NatsCluster) (string, error) {
	if cluster.Spec.URL != "" {
		return cluster.Spec.URL, nil
	}

	if cluster.Spec.URLFrom != nil {
		urlFromRef := cluster.Spec.URLFrom
		resourceRef := domain.NewNamespacedName(urlFromRef.Namespace, urlFromRef.Name)
		if resourceRef.Namespace == "" {
			resourceRef.Namespace = cluster.GetNamespace()
		}

		switch urlFromRef.Kind {
		case v1alpha1.URLFromKindConfigMap:
			data, err := r.configMapReader.Get(ctx, resourceRef)
			if err != nil {
				return "", fmt.Errorf("get ConfigMap %s: %w", resourceRef, err)
			}
			if natsURL, ok := data[urlFromRef.Key]; ok {
				return natsURL, nil
			}
			return "", fmt.Errorf("configMap %s has no key %q", resourceRef, urlFromRef.Key)
		case v1alpha1.URLFromKindSecret:
			data, err := r.secretReader.Get(ctx, resourceRef)
			if err != nil {
				return "", fmt.Errorf("get Secret %s: %w", resourceRef, err)
			}
			if natsURL, ok := data[urlFromRef.Key]; ok {
				return natsURL, nil
			}
			return "", fmt.Errorf("secret %s has no key %q", resourceRef, urlFromRef.Key)
		default:
			return "", fmt.Errorf("unsupported urlFrom.kind %q", urlFromRef.Kind)
		}
	}

	return "", fmt.Errorf("neither url nor urlFrom is set")
}

func (r *clusterTargetResolverImpl) validateAccountClusterRef(accountClusterRef domain.NamespacedName) error {
	opClusterRef := r.operatorClusterRef()
	if opClusterRef != nil && !r.config.OperatorNatsCluster.Optional && !opClusterRef.Equals(accountClusterRef) {
		return fmt.Errorf("account cluster reference %s does not match required operator cluster %s", accountClusterRef, opClusterRef)
	}

	return nil
}

func (r *clusterTargetResolverImpl) operatorClusterRef() *domain.NamespacedName {
	if r.config == nil || r.config.OperatorNatsCluster == nil {
		return nil
	}
	result := domain.NewNamespacedName(r.config.OperatorNatsCluster.ClusterRef.Namespace, r.config.OperatorNatsCluster.ClusterRef.Name)
	return &result
}
