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

type FactoryConfig struct {
	nauthNamespace string
	defaultNatsURL string
	opNatsCluster  *OperatorNatsCluster
}

func NewFactoryConfig(nauthNamespace, defaultNatsURL string, opNatsCluster *OperatorNatsCluster) *FactoryConfig {
	return &FactoryConfig{
		nauthNamespace: nauthNamespace,
		defaultNatsURL: defaultNatsURL,
		opNatsCluster:  opNatsCluster,
	}
}

func (c *FactoryConfig) GetOperatorNatsCluster() *OperatorNatsCluster {
	return c.opNatsCluster
}

func (c *FactoryConfig) validate() error {
	if c.nauthNamespace == "" {
		return fmt.Errorf("nauth namespace is required")
	}

	if c.opNatsCluster != nil {
		err := c.opNatsCluster.validate()
		if err != nil {
			return fmt.Errorf("invalid operator NATS cluster reference: %v", err)
		}
	}

	return nil
}

type OperatorNatsCluster struct {
	natsClusterRef v1alpha1.NatsClusterRef
	optional       bool
}

func (c *OperatorNatsCluster) String() string {
	optionalStr := "optional"
	if !c.optional {
		optionalStr = "non-optional"
	}
	return fmt.Sprintf("%s/%s (%s)", c.natsClusterRef.Namespace, c.natsClusterRef.Name, optionalStr)
}

func NewOperatorNatsCluster(natsClusterRef string, optional bool) (*OperatorNatsCluster, error) {
	clusterRef, err := parseNatsClusterRef(natsClusterRef)
	if err != nil {
		return nil, fmt.Errorf("invalid operator NATS cluster reference %q: %v", natsClusterRef, err)
	}

	return &OperatorNatsCluster{
		natsClusterRef: *clusterRef,
		optional:       optional,
	}, nil
}

func (c *OperatorNatsCluster) validate() error {
	if c.natsClusterRef.Name == "" || c.natsClusterRef.Namespace == "" {
		return fmt.Errorf("operator NATS cluster reference must have both namespace and name")
	}
	return nil
}

type ClusterGetter interface {
	GetNatsCluster(ctx context.Context, namespace, name string) (*v1alpha1.NatsCluster, error)
}

type ManagerFactory struct {
	config          *FactoryConfig
	clusters        ClusterGetter
	accounts        AccountGetter
	secretClient    SecretClient
	configmapClient ConfigMapClient
}

func NewManagerFactory(
	config *FactoryConfig,
	clusters ClusterGetter,
	accounts AccountGetter,
	secretClient SecretClient,
	configmapClient ConfigMapClient,
) (*ManagerFactory, error) {
	f := &ManagerFactory{
		config:          config,
		clusters:        clusters,
		accounts:        accounts,
		secretClient:    secretClient,
		configmapClient: configmapClient,
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *ManagerFactory) validate() error {
	if f.config == nil {
		return fmt.Errorf("config is required")
	}

	if err := f.config.validate(); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	return nil
}

func (f *ManagerFactory) ForAccount(ctx context.Context, acct *v1alpha1.Account) (controller.AccountManager, error) {
	clusterRef, err := getEffectiveNatsClusterRef(acct, f.config.opNatsCluster)
	if err != nil {
		return nil, fmt.Errorf("determine effective NATS cluster reference for account: %w", err)
	}

	var config *ManagerConfig
	var natsClient *natsc.Client
	if clusterRef == nil {
		if f.config.defaultNatsURL == "" {
			return nil, fmt.Errorf("no NATS cluster reference derived for account and no default NATS URL is configured")
		}

		natsClient = natsc.NewClient(f.config.defaultNatsURL, f.secretClient)
		config, err = NewManagerConfig(f.config.nauthNamespace)
		if err != nil {
			return nil, fmt.Errorf("create manager config without NATS cluster reference: %w", err)
		}
	} else {
		cluster, err := f.clusters.GetNatsCluster(ctx, clusterRef.Namespace, clusterRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get NatsCluster %s/%s: %w", clusterRef.Namespace, clusterRef.Name, err)
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

		natsClient = natsc.NewClientWithSecretRef(natsURL, f.secretClient, systemCredsSecretRef)
		config, err = NewManagerConfig(f.config.nauthNamespace, WithNatsCluster(cluster))
		if err != nil {
			return nil, fmt.Errorf("create manager config with NATS cluster reference: %w", err)
		}
	}

	return NewManager(config, f.accounts, natsClient, f.secretClient), nil
}

func getEffectiveNatsClusterRef(account *v1alpha1.Account, opNatsClusterRef *OperatorNatsCluster) (*v1alpha1.NatsClusterRef, error) {
	acRef := account.Spec.NatsClusterRef
	if acRef != nil && acRef.Namespace == "" {
		acRef = acRef.DeepCopy()
		acRef.Namespace = account.GetNamespace()
	}

	if opNatsClusterRef == nil {
		// #1: Account cluster (or none)
		return acRef, nil
	}

	opRef := opNatsClusterRef.natsClusterRef
	if acRef == nil {
		// #2: Operator cluster
		return &opRef, nil
	}

	if !opNatsClusterRef.optional && (acRef.Namespace != opRef.Namespace || acRef.Name != opRef.Name) {
		return nil, fmt.Errorf("the account NATS cluster reference %s/%s does not match the non-optional operator cluster %s/%s", acRef.Namespace, acRef.Name, opRef.Namespace, opRef.Name)
	}

	return acRef, nil
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

func parseNatsClusterRef(refStr string) (*v1alpha1.NatsClusterRef, error) {
	parts := strings.Split(refStr, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cluster reference pattern %q, expected namespace/name", refStr)
	}

	namespace := parts[0]
	name := parts[1]
	if name == "" || namespace == "" {
		return nil, fmt.Errorf("invalid cluster reference pattern %q, expected namespace/name", refStr)
	}
	if errs := k8sval.NameIsDNSSubdomain(name, false); len(errs) > 0 {
		return nil, fmt.Errorf("invalid cluster reference name %q in %q: %s", name, refStr, strings.Join(errs, ", "))
	}
	if errs := k8sval.ValidateNamespaceName(namespace, false); len(errs) > 0 {
		return nil, fmt.Errorf("invalid cluster reference namespace %q in %q: %s", namespace, refStr, strings.Join(errs, ", "))
	}

	return &v1alpha1.NatsClusterRef{Name: name, Namespace: namespace}, nil
}

// Compile-time assertion that ManagerFactory implements the controller.AccountManagerFactory interface
var _ controller.AccountManagerFactory = (*ManagerFactory)(nil)
