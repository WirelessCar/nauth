package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AccountExportControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	accountExportManagerMock *accountExportManagerMock
	accountReaderMock        *accountReaderMock

	accountExportName      string
	accountExportNamespace string
	accountExportRef       ktypes.NamespacedName
	rules                  []v1alpha1.AccountExportRule

	unitUnderTest *AccountExportReconciler
}

func TestAccountExportController_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountExportControllerTestSuite))
}

func (t *AccountExportControllerTestSuite) SetupTest() {
	t.ctx = context.Background()

	testName := t.T().Name()
	t.accountExportName = scopedTestName("test-account-export", testName)
	t.accountExportNamespace = scopedTestName("ns", testName)
	t.accountExportRef = ktypes.NamespacedName{
		Name:      t.accountExportName,
		Namespace: t.accountExportNamespace,
	}
	t.rules = []v1alpha1.AccountExportRule{
		{Type: v1alpha1.Stream, Name: "foo", Subject: "foo.>"},
		{Type: v1alpha1.Service, Name: "bar", Subject: "bar.>"},
	}

	t.Require().NoError(ensureNamespace(t.ctx, t.accountExportNamespace))

	t.accountExportManagerMock = &accountExportManagerMock{}
	t.accountReaderMock = &accountReaderMock{}

	t.unitUnderTest = NewAccountExportReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountExportManagerMock,
		t.accountReaderMock,
	)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldFail_WhenAdoptedByAccountNotImplemented() {
	// Given
	accountID := "ACCA"
	t.accountReaderMock.mockGet(t.ctx, domain.NewNamespacedName(t.accountExportNamespace, "my-account"), &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	})
	accountExportResource := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "my-account",
			Rules:       t.rules,
		},
	}
	t.accountExportManagerMock.mockCreateClaim(t.ctx, nil, &domain.AccountExportClaim{
		Rules: t.rules,
	})
	t.Require().NoError(k8sClient.Create(t.ctx, accountExportResource))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)

	accountExport := &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	t.Require().NotNil(accountExport.Status.Claim)
	t.Require().Equal(&v1alpha1.AccountExportClaim{
		Rules:              t.rules,
		ObservedGeneration: int64(1),
	}, accountExport.Status.Claim)

	conditions := accountExport.Status.Conditions
	t.Require().NotEmpty(conditions)

	t.assertCondition(conditions, accountExportConditionTypeBoundToAccount, metav1.ConditionTrue, controllerReasonOK)
	t.assertCondition(conditions, accountExportConditionTypeValidRules, metav1.ConditionTrue, controllerReasonOK)
	t.assertCondition(conditions, accountExportConditionTypeAdoptedByAccount, metav1.ConditionFalse, "NotImplemented")
	t.assertCondition(conditions, controllerTypeReady, metav1.ConditionFalse, "NotReady")
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldFail_WhenAccountIDChangeDetected() {
	// Given
	t.accountReaderMock.mockGet(t.ctx, domain.NewNamespacedName(t.accountExportNamespace, "account-a"), &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-a",
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): "ACCA",
			},
		},
	}).Once()
	t.accountReaderMock.mockGet(t.ctx, domain.NewNamespacedName(t.accountExportNamespace, "account-b"), &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-b",
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): "ACCB",
			},
		},
	}).Once()
	t.accountExportManagerMock.mockCreateClaim(t.ctx, nil, &domain.AccountExportClaim{
		Rules: t.rules,
	}).Twice()

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "account-a",
			Rules:       t.rules,
		},
	}))
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})
	t.Require().NoError(err)

	accountExport := &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	accountExport.Spec.AccountName = "account-b"
	t.Require().NoError(k8sClient.Update(t.ctx, accountExport))

	// When
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)

	accountExport = &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)

	conditions := accountExport.Status.Conditions
	t.Require().NotEmpty(conditions)
	boundToAccountCondition := t.assertCondition(conditions, accountExportConditionTypeBoundToAccount, metav1.ConditionFalse, metav1.StatusReasonConflict)
	t.Equal("Account ID conflict: previously bound to ACCA, now found ACCB", boundToAccountCondition.Message)
	t.assertCondition(conditions, accountExportConditionTypeValidRules, metav1.ConditionTrue, controllerReasonOK)
	t.assertCondition(conditions, accountExportConditionTypeAdoptedByAccount, metav1.ConditionFalse, "NotImplemented")
	t.assertCondition(conditions, controllerTypeReady, metav1.ConditionFalse, "NotReady")
}

func (t *AccountExportControllerTestSuite) assertCondition(conditions []metav1.Condition, conditionType string,
	expectStatus metav1.ConditionStatus, expectReason metav1.StatusReason) metav1.Condition {
	var condition metav1.Condition
	for _, c := range conditions {
		if c.Type == conditionType {
			condition = c
			break
		}
	}
	t.NotEmpty(condition, fmt.Sprintf("Condition not found: %s", conditionType))
	t.Equal(
		fmt.Sprintf("%s|%s|%s", conditionType, expectStatus, expectReason),
		fmt.Sprintf("%s|%s|%s", condition.Type, condition.Status, condition.Reason))
	return condition
}

/* ****************************************************
* inbound.AccountExportManager Mock
*****************************************************/
type accountExportManagerMock struct {
	mock.Mock
}

func (m *accountExportManagerMock) CreateClaim(ctx context.Context, state *v1alpha1.AccountExport) (*domain.AccountExportClaim, error) {
	args := m.Called(ctx, state)
	return args.Get(0).(*domain.AccountExportClaim), args.Error(1)
}

func (m *accountExportManagerMock) mockCreateClaim(ctx context.Context, state *v1alpha1.AccountExport, result *domain.AccountExportClaim) *mock.Call {
	var stateExpect interface{} = state
	if state == nil {
		stateExpect = mock.Anything
	}
	call := m.On("CreateClaim", ctx, stateExpect)
	call.Return(result, nil)
	return call
}

var _ inbound.AccountExportManager = &accountExportManagerMock{}
