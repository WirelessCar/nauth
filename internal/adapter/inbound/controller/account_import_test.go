package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

type AccountImportControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	namespace            string
	importName           string
	importNamespacedName ktypes.NamespacedName
	importAccountName    string
	exportAccountName    string
	foreignNamespace     string

	unitUnderTest *AccountImportReconciler
}

func TestAccountImportController_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountImportControllerTestSuite))
}

func (t *AccountImportControllerTestSuite) SetupTest() {
	t.ctx = context.Background()

	testName := t.T().Name()
	t.importName = scopedTestName("account-import", testName)
	t.namespace = scopedTestName("namespace", testName)
	t.importNamespacedName = ktypes.NamespacedName{Name: t.importName, Namespace: t.namespace}
	t.importAccountName = scopedTestName("import-account", testName)
	t.exportAccountName = scopedTestName("export-account", testName)
	t.foreignNamespace = scopedTestName("export-namespace", testName)

	t.Require().NoError(ensureNamespace(t.ctx, t.namespace))

	t.unitUnderTest = NewAccountImportReconciler(
		k8sClient,
		k8sClient.Scheme(),
	)
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed() {
	// Given
	importAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, importAccountID)
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.foreignNamespace, t.exportAccountName, exportAccountID)

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.importName,
			Namespace: t.namespace,
		},
		Spec: v1alpha1.AccountImportSpec{
			AccountName: t.importAccountName,
			ExportAccountRef: v1alpha1.AccountRef{
				Name:      t.exportAccountName,
				Namespace: t.foreignNamespace,
			},
			Rules: []v1alpha1.AccountImportRule{
				{
					Subject: "foo.*",
					Type:    v1alpha1.Stream,
				},
			},
		},
	}))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})

	// Then
	t.Require().NoError(err)

	result := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, result))
	t.assertCondition(result, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(result, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(result, conditionTypeValidRules, metav1.ConditionFalse, "NotImplemented")
	// TODO: [#11] Verify rules validation condition
	t.assertCondition(result, conditionTypeAdoptedByAccount, metav1.ConditionFalse, "NotImplemented")
	// TODO: [#11] Verify account adoption condition
	t.assertCondition(result, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)
	t.Equal(importAccountID, result.GetLabel(v1alpha1.AccountImportLabelAccountID))
	t.Equal(exportAccountID, result.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenExportAccountInImplicitSameNamespace() {
	// Given
	importAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, importAccountID)
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.exportAccountName, exportAccountID)

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.importName,
			Namespace: t.namespace,
		},
		Spec: v1alpha1.AccountImportSpec{
			AccountName: t.importAccountName,
			ExportAccountRef: v1alpha1.AccountRef{
				Name: t.exportAccountName,
			},
			Rules: []v1alpha1.AccountImportRule{
				{
					Subject: "foo.*",
					Type:    v1alpha1.Stream,
				},
			},
		},
	}))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})

	// Then
	t.Require().NoError(err)

	result := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, result))
	t.assertCondition(result, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(result, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	t.Equal(importAccountID, result.GetLabel(v1alpha1.AccountImportLabelAccountID))
	t.Equal(exportAccountID, result.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenImportAccountIsNotReady() {
	// Given
	t.ensureAccount(t.namespace, t.importAccountName, "")
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.exportAccountName, exportAccountID)

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.importName,
			Namespace: t.namespace,
		},
		Spec: v1alpha1.AccountImportSpec{
			AccountName: t.importAccountName,
			ExportAccountRef: v1alpha1.AccountRef{
				Name: t.exportAccountName,
			},
			Rules: []v1alpha1.AccountImportRule{
				{
					Subject: "foo.*",
					Type:    v1alpha1.Stream,
				},
			},
		},
	}))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})

	// Then
	t.Require().NoError(err)

	result := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, result))

	condition := t.assertCondition(result, conditionTypeBoundToAccount, metav1.ConditionFalse, conditionReasonInvalid)
	t.Contains(condition.Message, "not bound to an Account ID yet")
	t.Equal("", result.GetLabel(v1alpha1.AccountImportLabelAccountID))

	t.assertCondition(result, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(result, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)
	t.Equal(exportAccountID, result.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenExportAccountIsNotReady() {
	// Given
	accountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, accountID)
	t.ensureAccount(t.namespace, t.exportAccountName, "")

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.importName,
			Namespace: t.namespace,
		},
		Spec: v1alpha1.AccountImportSpec{
			AccountName: t.importAccountName,
			ExportAccountRef: v1alpha1.AccountRef{
				Name: t.exportAccountName,
			},
			Rules: []v1alpha1.AccountImportRule{
				{
					Subject: "foo.*",
					Type:    v1alpha1.Stream,
				},
			},
		},
	}))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})

	// Then
	t.Require().NoError(err)

	result := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, result))

	condition := t.assertCondition(result, conditionTypeBoundToExportAccount, metav1.ConditionFalse, conditionReasonInvalid)
	t.Contains(condition.Message, "not bound to an Account ID yet")
	t.Equal("", result.GetLabel(v1alpha1.AccountImportLabelExportAccountID))

	t.assertCondition(result, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(result, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)
	t.Equal(accountID, result.GetLabel(v1alpha1.AccountImportLabelAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenResourceNotFound() {
	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})

	// Then
	t.Require().NoError(err)
}

func (t *AccountImportControllerTestSuite) Test_getConditionedAccount_ShouldReturnFalse_WhenAccountRefInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("invalid namespace", "account")

	// When
	account, condition := t.unitUnderTest.getConditionedAccount(t.ctx, accountRef, "")

	// Then
	t.Equal(metav1.ConditionFalse, condition.Status)
	t.Equal(string(metav1.StatusReasonInvalid), condition.Reason)
	t.Contains(condition.Message, "Invalid Account reference")
	t.Nil(account)
}

func (t *AccountImportControllerTestSuite) Test_getConditionedAccount_ShouldReturnFalse_WhenAccountNotFound() {
	// Given
	accountRef := domain.NewNamespacedName(t.namespace, "my-account")

	// When
	account, condition := t.unitUnderTest.getConditionedAccount(t.ctx, accountRef, "")

	// Then
	t.Equal(metav1.ConditionFalse, condition.Status)
	t.Equal(string(metav1.StatusReasonNotFound), condition.Reason)
	t.Contains(condition.Message, "/my-account not found")
	t.Nil(account)
}

func (t *AccountImportControllerTestSuite) Test_getConditionedAccount_ShouldReturnFalse_WhenAccountIDNotSet() {
	// Given
	accountRef := domain.NewNamespacedName(t.namespace, "my-account")
	t.ensureAccount(t.namespace, "my-account", "")

	// When
	account, condition := t.unitUnderTest.getConditionedAccount(t.ctx, accountRef, "")

	// Then
	t.Equal(metav1.ConditionFalse, condition.Status)
	t.Equal(string(metav1.StatusReasonInvalid), condition.Reason)
	t.Contains(condition.Message, "/my-account not bound to an Account ID yet")
	t.Nil(account)
}

func (t *AccountImportControllerTestSuite) Test_getConditionedAccount_ShouldReturnFalse_WhenAccountIDMismatchBound() {
	// Given
	accountRef := domain.NewNamespacedName(t.namespace, "my-account")
	t.ensureAccount(t.namespace, "my-account", t.anyAccountID())

	// When
	account, condition := t.unitUnderTest.getConditionedAccount(t.ctx, accountRef, t.anyAccountID())

	// Then
	t.Equal(metav1.ConditionFalse, condition.Status)
	t.Equal(string(metav1.StatusReasonConflict), condition.Reason)
	t.Contains(condition.Message, "Account ID conflict")
	t.Nil(account)
}

// Helpers

func (t *AccountImportControllerTestSuite) assertCondition(result *v1alpha1.AccountImport, conditionType string,
	expectStatus metav1.ConditionStatus, expectReason string) metav1.Condition {
	var condition metav1.Condition
	for _, c := range *result.GetConditions() {
		if c.Type == conditionType {
			condition = c
			break
		}
	}
	t.NotEmpty(condition, fmt.Sprintf("Condition not found: %s", conditionType))
	t.Equalf(
		fmt.Sprintf("%s|%s|%s", conditionType, expectStatus, expectReason),
		fmt.Sprintf("%s|%s|%s", condition.Type, condition.Status, condition.Reason),
		condition.Message)
	return condition
}

func (t *AccountImportControllerTestSuite) ensureAccount(namespace, name, accountID string) {
	t.Require().NoError(ensureNamespace(t.ctx, namespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	}))
}

func (t *AccountImportControllerTestSuite) anyAccountID() (accountID string) {
	accountKey, _ := nkeys.CreateAccount()
	accountID, _ = accountKey.PublicKey()
	return
}
