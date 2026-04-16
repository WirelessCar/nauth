package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AccountExportClientTestSuite struct {
	suite.Suite
	ctx       context.Context
	namespace domain.Namespace
	accountID string

	unitUnderTest *AccountExportClient
}

func TestAccountExportClient_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountExportClientTestSuite))
}

func (t *AccountExportClientTestSuite) SetupTest() {
	t.ctx = context.Background()

	t.namespace = generateNamespace()
	accountKey, _ := nkeys.CreateAccount()
	t.accountID, _ = accountKey.PublicKey()
	t.unitUnderTest = NewAccountExportClient(k8sClient)
}

func (t *AccountExportClientTestSuite) Test_FindByAccountID_ShouldSucceed() {
	// Given
	t.createExport(t.namespace, "export-1", t.accountID)
	t.createExport(t.namespace, "export-2-other-account", "AOTHER")
	t.createExport(t.namespace, "export-3", t.accountID)
	t.createExport("ns-other", "export-4-other-ns", t.accountID)
	t.createExport(t.namespace, "export-5", t.accountID)

	// When
	result, err := t.unitUnderTest.FindByAccountID(t.ctx, t.namespace, t.accountID)

	// Then
	t.Require().NoError(err)
	t.NotNil(result)
	t.assertExports(*result, "export-1", "export-3", "export-5")
}

func (t *AccountExportClientTestSuite) Test_FindByAccountID_ShouldSucceed_WhenNoResourcesExist() {
	// When
	result, err := t.unitUnderTest.FindByAccountID(t.ctx, t.namespace, t.accountID)

	// Then
	t.Require().NoError(err)
	t.NotNil(result)
	t.Emptyf(result.Items, "expected no account exports to be found")
}

func (t *AccountExportClientTestSuite) assertExports(actual v1alpha1.AccountExportList, expectNames ...string) {
	actualNames := make([]string, len(actual.Items))
	for i, export := range actual.Items {
		actualNames[i] = export.Name
	}
	t.ElementsMatch(expectNames, actualNames)
}

// Helpers

func (t *AccountExportClientTestSuite) createExport(namespace domain.Namespace, name, accountID string) v1alpha1.AccountExport {
	namespaceResource := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: string(namespace)},
	}
	if err := k8sClient.Get(t.ctx, client.ObjectKeyFromObject(namespaceResource), namespaceResource); err != nil {
		t.Require().NoError(k8sClient.Create(t.ctx, namespaceResource))
	}

	export := v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: string(namespace),
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): accountID,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "account-name",
			Rules: []v1alpha1.AccountExportRule{
				{
					Name:    "rule-name",
					Subject: "foo.*",
					Type:    v1alpha1.Stream,
				},
			},
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, &export))
	return export
}
