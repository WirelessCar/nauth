package account_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	controlleradapter "github.com/WirelessCar/nauth/internal/adapter/inbound/controller"
	k8sadapter "github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	natsadapter "github.com/WirelessCar/nauth/internal/adapter/outbound/nats"
	"github.com/WirelessCar/nauth/internal/core"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/test/integration/testkit/approvalstest"
	envtestkit "github.com/WirelessCar/nauth/test/integration/testkit/envtest"
	"github.com/WirelessCar/nauth/test/integration/testkit/natstest"
	"github.com/WirelessCar/nauth/test/integration/testkit/scenariotest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestAccountControllerIntegration(t *testing.T) {
	testCases := scenariotest.Discover("approvals/account_controller_integration_test.TestAccountControllerIntegration.{TestCase}.input.yaml")
	require.NotEmpty(t, testCases, "no test cases discovered")

	for _, testCase := range testCases {
		t.Run(testCase.TestName, func(t *testing.T) {
			scenario, err := scenariotest.Load(testCase.InputFile)
			require.NoError(t, err)

			server := natstest.Start(t)
			runner := newRunner(t, sharedEnv, server, scenario)
			output := runner.run(t)

			approvalstest.VerifyYAML(t, output)
		})
	}
}

func newRunner(t *testing.T, env *envtestkit.Environment, server *natstest.Server, scenario *scenariotest.Scenario) *accountRunner {
	t.Helper()
	return &accountRunner{
		env:      env,
		server:   server,
		scenario: scenario,
	}
}

type accountRunner struct {
	env      *envtestkit.Environment
	server   *natstest.Server
	scenario *scenariotest.Scenario
}

func (r *accountRunner) run(t *testing.T) string {
	t.Helper()

	stopManager := r.startManager(t)
	defer stopManager()

	r.applyBootstrapResources(t)
	r.applyScenarioResources(t)

	require.Eventually(t, func() bool {
		return r.accountReady(t)
	}, 10*time.Second, 200*time.Millisecond)

	return r.renderSnapshot(t)
}

func (r *accountRunner) startManager(t *testing.T) func() {
	t.Helper()

	operatorCluster, err := core.NewOperatorNatsCluster(v1alpha1.NatsClusterRef{
		Namespace: r.scenario.Config.OperatorCluster.Namespace,
		Name:      r.scenario.Config.OperatorCluster.Name,
	}, r.scenario.Config.OperatorCluster.Optional)
	require.NoError(t, err)

	config, err := core.NewConfig(operatorCluster, domain.Namespace(r.scenario.Config.OperatorNamespace))
	require.NoError(t, err)

	mgr, err := ctrl.NewManager(r.env.Config, ctrl.Options{
		Scheme:                 r.env.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{},
		},
	})
	require.NoError(t, err)

	k8sClient := mgr.GetClient()
	secretClient := k8sadapter.NewSecretClient(k8sClient)
	accountReader := k8sadapter.NewAccountClient(k8sClient)
	clusterReader := k8sadapter.NewNatsClusterClient(mgr.GetAPIReader())
	configMapReader := k8sadapter.NewConfigMapClient(k8sClient)
	clusterManager, err := core.NewClusterManager(
		clusterReader,
		natsadapter.NewSysClient(),
		secretClient,
		configMapReader,
		config,
	)
	require.NoError(t, err)
	accountManager, err := core.NewAccountManager(
		natsadapter.NewSysClient(),
		natsadapter.NewAccountClient(),
		accountReader,
		secretClient,
		clusterManager,
	)
	require.NoError(t, err)

	reconciler := controlleradapter.NewAccountReconciler(
		k8sClient,
		mgr.GetScheme(),
		accountManager,
		events.NewFakeRecorder(20),
	)
	require.NoError(t, reconciler.SetupWithManager(mgr))

	ctx, cancel := context.WithCancel(context.Background())
	t.Setenv("OPERATOR_VERSION", r.scenario.Config.OperatorVersion)
	go func() {
		_ = mgr.Start(ctx)
	}()

	return func() {
		cancel()
	}
}

func (r *accountRunner) applyBootstrapResources(t *testing.T) {
	t.Helper()

	bootstrapObjects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: r.scenario.Config.OperatorNamespace}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: r.scenario.Config.OperatorCluster.Namespace}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "operator-signing-key",
				Namespace: r.scenario.Config.OperatorCluster.Namespace,
			},
			Data: map[string][]byte{"default": r.server.OperatorSigningSeed},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "system-account-creds",
				Namespace: r.scenario.Config.OperatorCluster.Namespace,
			},
			Data: map[string][]byte{"default": r.server.SystemCreds},
		},
		&v1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.scenario.Config.OperatorCluster.Name,
				Namespace: r.scenario.Config.OperatorCluster.Namespace,
			},
			Spec: v1alpha1.NatsClusterSpec{
				URL: r.server.URL,
				OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
					Name: "operator-signing-key",
				},
				SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
					Name: "system-account-creds",
				},
			},
		},
	}

	for _, object := range bootstrapObjects {
		err := r.env.Client.Create(context.Background(), object)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			require.NoError(t, err)
		}
	}
}

func (r *accountRunner) applyScenarioResources(t *testing.T) {
	t.Helper()

	objects, err := scenariotest.DecodeObjects(r.scenario.Objects)
	require.NoError(t, err)

	for _, object := range objects {
		err := r.env.Client.Create(context.Background(), object)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			require.NoError(t, err)
		}
	}
}

func (r *accountRunner) accountReady(t *testing.T) bool {
	t.Helper()

	for _, ref := range r.scenario.Collect {
		if ref.APIVersion == "nauth.io/v1alpha1" && ref.Kind == "Account" {
			account := &v1alpha1.Account{}
			err := r.env.Client.Get(context.Background(), types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, account)
			if err != nil {
				return false
			}
			for _, condition := range account.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
					return true
				}
			}
		}
	}
	return false
}

func (r *accountRunner) renderSnapshot(t *testing.T) string {
	t.Helper()

	collected := make([]map[string]interface{}, 0, len(r.scenario.Collect))
	for _, ref := range r.scenario.Collect {
		object := &unstructured.Unstructured{}
		object.SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
		err := r.env.Client.Get(context.Background(), types.NamespacedName{
			Namespace: ref.Namespace,
			Name:      ref.Name,
		}, object)
		require.NoError(t, err)
		normalizeObject(object.Object)
		collected = append(collected, object.Object)
	}

	sort.Slice(collected, func(i, j int) bool {
		left := collected[i]["kind"].(string) + "/" + collected[i]["metadata"].(map[string]interface{})["namespace"].(string) + "/" + collected[i]["metadata"].(map[string]interface{})["name"].(string)
		right := collected[j]["kind"].(string) + "/" + collected[j]["metadata"].(map[string]interface{})["namespace"].(string) + "/" + collected[j]["metadata"].(map[string]interface{})["name"].(string)
		return left < right
	})

	data, err := yaml.Marshal(map[string]interface{}{"resources": collected})
	require.NoError(t, err)
	return string(data)
}

func normalizeObject(object map[string]interface{}) {
	metadata, ok := object["metadata"].(map[string]interface{})
	if ok {
		delete(metadata, "creationTimestamp")
		delete(metadata, "generation")
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "uid")

		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			if _, exists := labels[string(v1alpha1.AccountLabelAccountID)]; exists {
				labels[string(v1alpha1.AccountLabelAccountID)] = "<ACCOUNT_ID>"
			}
			if _, exists := labels[string(v1alpha1.AccountLabelSignedBy)]; exists {
				labels[string(v1alpha1.AccountLabelSignedBy)] = "<OPERATOR_SIGNING_KEY>"
			}
		}
	}

	status, ok := object["status"].(map[string]interface{})
	if !ok {
		return
	}

	if _, exists := status["claimsHash"]; exists {
		status["claimsHash"] = "<CLAIMS_HASH>"
	}
	if _, exists := status["reconcileTimestamp"]; exists {
		status["reconcileTimestamp"] = "<TIMESTAMP>"
	}
	if conditions, ok := status["conditions"].([]interface{}); ok {
		for _, rawCondition := range conditions {
			condition, ok := rawCondition.(map[string]interface{})
			if !ok {
				continue
			}
			if _, exists := condition["lastTransitionTime"]; exists {
				condition["lastTransitionTime"] = "<TIMESTAMP>"
			}
		}
	}
	if claims, ok := status["claims"].(map[string]interface{}); ok {
		if signingKeys, ok := claims["signingKeys"].([]interface{}); ok {
			for _, rawSigningKey := range signingKeys {
				signingKey, ok := rawSigningKey.(map[string]interface{})
				if !ok {
					continue
				}
				if _, exists := signingKey["key"]; exists {
					signingKey["key"] = "<ACCOUNT_SIGNING_KEY>"
				}
			}
		}
	}
}
