package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"
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

	accountImportManagerMock *accountImportManagerMock

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

	t.accountImportManagerMock = &accountImportManagerMock{}

	t.Require().NoError(ensureNamespace(t.ctx, t.namespace))

	t.unitUnderTest = NewAccountImportReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountImportManagerMock,
	)
}

func (t *AccountImportControllerTestSuite) TearDownTest() {
	t.accountImportManagerMock.AssertExpectations(t.T())
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed() {
	// Given
	importAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, importAccountID)
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.foreignNamespace, t.exportAccountName, exportAccountID)

	resourceInput := v1alpha1.AccountImport{
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
	}
	t.Require().NoError(k8sClient.Create(t.ctx, &resourceInput))
	importAccount := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, ktypes.NamespacedName{Namespace: t.namespace, Name: t.importAccountName}, importAccount))
	importAccount.Status.Adoptions = &v1alpha1.AccountAdoptions{
		Imports: []v1alpha1.AccountAdoption{
			{
				Name:               t.importName,
				ObservedGeneration: resourceInput.Generation,
				UID:                resourceInput.UID,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionTrue,
					DesiredClaimObservedGeneration: &resourceInput.Generation,
					Reason:                         conditionReasonOK,
					Message:                        "",
				},
			},
		},
	}
	t.Require().NoError(k8sClient.Status().Update(t.ctx, importAccount))
	expectNAuthImports := nauth.Imports{
		&nauth.Import{
			AccountID: nauth.AccountID(exportAccountID),
			Subject:   nauth.Subject("foo.*"),
			Type:      nauth.ExportTypeStream,
		},
	}
	t.accountImportManagerMock.mockValidateImports(nauth.AccountID(importAccountID), expectNAuthImports).Once()

	// When
	result, err := t.runReconcileLoopForNewResource(importAccountID, exportAccountID)

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful reconciliation")

	resource := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, resource))
	t.assertCondition(resource, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeValidRules, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeAdoptedByAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeReady, metav1.ConditionTrue, conditionReasonReady)

	boolFalse := false
	expectClaim := &v1alpha1.AccountImportClaim{
		ObservedGeneration: 1,
		Rules: []v1alpha1.AccountImportRuleDerived{
			{
				Account: exportAccountID,
				AccountImportRule: v1alpha1.AccountImportRule{
					Subject:    "foo.*",
					Type:       v1alpha1.Stream,
					Share:      &boolFalse,
					AllowTrace: &boolFalse,
				},
			},
		},
	}
	t.Equalf(expectClaim, resource.Status.DesiredClaim, "expected claim")
	t.Equal(importAccountID, resource.GetLabel(v1alpha1.AccountImportLabelAccountID))
	t.Equal(exportAccountID, resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful status update")
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenExportAccountInImplicitSameNamespace() {
	// Given
	importAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, importAccountID)
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.exportAccountName, exportAccountID)

	resourceInput := v1alpha1.AccountImport{
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
	}
	t.Require().NoError(k8sClient.Create(t.ctx, &resourceInput))
	expectImports := nauth.Imports{
		{
			AccountID: nauth.AccountID(exportAccountID),
			Subject:   nauth.Subject("foo.*"),
			Type:      nauth.ExportTypeStream,
		},
	}
	t.accountImportManagerMock.mockValidateImports(nauth.AccountID(importAccountID), expectImports).Once()

	// When
	result, err := t.runReconcileLoopForNewResource(importAccountID, exportAccountID)

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful reconciliation")

	resource := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, resource))
	t.assertCondition(resource, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	t.Equal(importAccountID, resource.GetLabel(v1alpha1.AccountImportLabelAccountID))
	t.Equal(exportAccountID, resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenImportAccountIsNotReady() {
	// Given
	t.ensureAccount(t.namespace, t.importAccountName, "")
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.exportAccountName, exportAccountID)

	resourceInput := v1alpha1.AccountImport{
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
	}
	t.Require().NoError(k8sClient.Create(t.ctx, &resourceInput))

	// When
	result, err := t.runReconcileLoopForNewResource("", exportAccountID)

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful reconciliation")

	// Then
	t.Require().NoError(err)

	resource := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, resource))

	condition := t.assertCondition(resource, conditionTypeBoundToAccount, metav1.ConditionFalse, conditionReasonInvalid)
	t.Contains(condition.Message, "not bound to an Account ID yet")
	t.Equal("", resource.GetLabel(v1alpha1.AccountImportLabelAccountID))

	t.assertCondition(resource, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)
	t.Equal(exportAccountID, resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenExportAccountIsNotReady() {
	// Given
	accountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, accountID)
	t.ensureAccount(t.namespace, t.exportAccountName, "")

	resourceInput := v1alpha1.AccountImport{
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
	}
	t.Require().NoError(k8sClient.Create(t.ctx, &resourceInput))

	// When
	result, err := t.runReconcileLoopForNewResource(accountID, "")

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful reconciliation")

	resource := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, resource))

	condition := t.assertCondition(resource, conditionTypeBoundToExportAccount, metav1.ConditionFalse, conditionReasonInvalid)
	t.Contains(condition.Message, "not bound to an Account ID yet")
	t.Equal("", resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID))

	t.assertCondition(resource, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)
	t.Equal(accountID, resource.GetLabel(v1alpha1.AccountImportLabelAccountID))
}

func (t *AccountImportControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenRulesValidationFail() {
	// Given
	importAccountID := t.anyAccountID()
	t.ensureAccount(t.namespace, t.importAccountName, importAccountID)
	exportAccountID := t.anyAccountID()
	t.ensureAccount(t.foreignNamespace, t.exportAccountName, exportAccountID)

	resourceInput := v1alpha1.AccountImport{
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
	}
	t.Require().NoError(k8sClient.Create(t.ctx, &resourceInput))
	expectImports := nauth.Imports{
		&nauth.Import{
			AccountID: nauth.AccountID(exportAccountID),
			Subject:   nauth.Subject("foo.*"),
			Type:      nauth.ExportTypeStream,
		},
	}
	t.accountImportManagerMock.mockValidateImportsError(nauth.AccountID(importAccountID), expectImports, fmt.Errorf("invalid test rules")).Once()

	// When
	result, err := t.runReconcileLoopForNewResource(importAccountID, exportAccountID)

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful reconciliation")

	resource := &v1alpha1.AccountImport{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, resource))
	t.assertCondition(resource, conditionTypeBoundToAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(resource, conditionTypeBoundToExportAccount, metav1.ConditionTrue, conditionReasonOK)
	rulesCondition := t.assertCondition(resource, conditionTypeValidRules, metav1.ConditionFalse, conditionReasonNOK)
	t.Contains(rulesCondition.Message, "invalid test rules")
	t.assertCondition(resource, conditionTypeAdoptedByAccount, metav1.ConditionFalse, conditionReasonAdopting)
	t.assertCondition(resource, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)

	t.Nil(resource.Status.DesiredClaim, "expected no claim")
	t.Equal(importAccountID, resource.GetLabel(v1alpha1.AccountImportLabelAccountID))
	t.Equal(exportAccountID, resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID))
	t.Require().Empty(result.RequeueAfter, "no reconcile requeue expected after successful status update")
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

func (t *AccountImportControllerTestSuite) runReconcileLoopForNewResource(expectAccountID string, expectExportAccountID string) (ctrl.Result, error) {
	// 1) expect labels to be upserted
	resource := &v1alpha1.AccountImport{}
	result, err := t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().NotEmpty(result.RequeueAfter, "reconcile requeue expected after labels upsert")
	t.Require().NoError(k8sClient.Get(t.ctx, t.importNamespacedName, resource))
	t.Require().Equalf(expectAccountID, resource.GetLabel(v1alpha1.AccountImportLabelAccountID), "Account ID label mismatch")
	t.Require().Equalf(expectExportAccountID, resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID), "Export Account ID label mismatch")
	t.Require().Nil(resource.Status.Conditions, "no Status Conditions expected after first reconcile run")

	// 2) expect conditions to be updated
	return t.unitUnderTest.Reconcile(t.ctx, ctrl.Request{NamespacedName: t.importNamespacedName})
}

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

type accountImportManagerMock struct {
	mock.Mock
}

func (m *accountImportManagerMock) ValidateImports(importAccountID nauth.AccountID, imports nauth.Imports) error {
	args := m.Called(importAccountID, imports)
	return args.Error(0)
}

func (m *accountImportManagerMock) mockValidateImports(importAccountID nauth.AccountID, imports nauth.Imports) *mock.Call {
	return m.On("ValidateImports", importAccountID, imports).Return(nil)
}

func (m *accountImportManagerMock) mockValidateImportsError(importAccountID nauth.AccountID, imports nauth.Imports, err error) *mock.Call {
	return m.On("ValidateImports", importAccountID, imports).Return(err)
}

var _ inbound.AccountImportManager = (*accountImportManagerMock)(nil)
