package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NatsClusterClientTestSuite struct {
	suite.Suite
	ctx        context.Context
	clusterRef domain.NamespacedName

	unitUnderTest *NatsClusterClient
}

func TestNatsClusterClient_TestSuite(t *testing.T) {
	suite.Run(t, new(NatsClusterClientTestSuite))
}

func (t *NatsClusterClientTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.clusterRef = domain.NewNamespacedName(testNamespace, sanitizeTestName(t.T().Name()))
	t.Require().NoError(t.clusterRef.Validate())
	t.unitUnderTest = NewNatsClusterClient(k8sClient)
	t.Require().NoError(cleanNatsCluster(t.ctx, t.clusterRef))
}

func (t *NatsClusterClientTestSuite) TearDownTest() {
	t.Require().NoError(cleanNatsCluster(t.ctx, t.clusterRef))
}

func (t *NatsClusterClientTestSuite) Test_Get_ShouldSucceed_WhenClusterExists() {
	// Given
	want := &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.clusterRef.Name,
			Namespace: t.clusterRef.Namespace,
		},
		Spec: v1alpha1.NatsClusterSpec{
			URL: "nats://nats:4222",
			OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
				Name: "operator-signing-key",
				Key:  "seed",
			},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
				Name: "system-account-user-creds",
				Key:  "user.creds",
			},
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, want))

	// When
	result, err := t.unitUnderTest.Get(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(want.GetName(), result.GetName())
	t.Equal(want.GetNamespace(), result.GetNamespace())
	t.Equal(want.Spec, result.Spec)
}

func (t *NatsClusterClientTestSuite) Test_Get_ShouldFail_WhenClusterDoesNotExist() {
	// Given
	missingClusterRef := domain.NewNamespacedName(testNamespace, "missing-cluster")
	t.Require().NoError(missingClusterRef.Validate())

	// When
	result, err := t.unitUnderTest.Get(t.ctx, missingClusterRef)

	// Then
	t.Error(err)
	t.Nil(result)
	t.ErrorContains(err, "failed getting NatsCluster "+testNamespace+"/missing-cluster")
	t.ErrorContains(err, "not found")
}

func cleanNatsCluster(ctx context.Context, clusterRef domain.NamespacedName) error {
	cluster := &v1alpha1.NatsCluster{}
	key := client.ObjectKey{Namespace: clusterRef.Namespace, Name: clusterRef.Name}
	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return k8sClient.Delete(ctx, cluster)
}
