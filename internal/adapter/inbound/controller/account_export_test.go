package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/core"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AccountExportControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	accountExportName      string
	accountExportNamespace string
	accountExportRef       ktypes.NamespacedName
	rules                  []v1alpha1.AccountExportRule
	accountNameA           string
	accountNameB           string
	accountIDA             string
	accountIDB             string

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
	t.accountNameA = "account-a"
	t.accountIDA = accountIDAccA
	t.accountNameB = "account-b"
	t.accountIDB = accountIDAccB

	t.Require().NoError(ensureNamespace(t.ctx, t.accountExportNamespace))

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountNameA,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): t.accountIDA,
			},
		},
		Spec: v1alpha1.AccountSpec{},
	}))

	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountNameB,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): t.accountIDB,
			},
		},
		Spec: v1alpha1.AccountSpec{},
	}))

	t.unitUnderTest = NewAccountExportReconciler(
		k8sClient,
		k8sClient.Scheme(),
		core.NewAccountExportManager(),
	)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldFail_WhenExportRuleValidationFails() {
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): t.accountIDA,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: t.accountNameA,
			Rules: []v1alpha1.AccountExportRule{
				{Type: v1alpha1.Stream, Name: "invalid rule", Subject: "."},
			},
		},
	}))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)

	accountExport := &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	t.Require().Nil(accountExport.Status.DesiredClaim)

	conditions := accountExport.Status.Conditions
	t.Require().NotEmpty(conditions)

	t.assertBoundAccountID(accountExport, t.accountIDA)
	t.assertCondition(conditions, conditionTypeValidRules, metav1.ConditionFalse, conditionReasonInvalid)
	t.assertCondition(conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonReconciling)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldFail_WhenExportRuleValidationFailsKeepingLastValid() {
	// Given
	accountExportResource := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): t.accountIDA,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: t.accountNameA,
			Rules:       t.rules,
		},
	}

	t.Require().NoError(k8sClient.Create(t.ctx, accountExportResource))
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})
	t.Require().NoError(err)

	// When
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExportResource)
	t.Require().NoError(err)
	invalidRules := []v1alpha1.AccountExportRule{
		{Type: v1alpha1.Stream, Name: "invalid rule", Subject: "."},
	}
	accountExportResource.Spec.Rules = invalidRules
	t.Require().NoError(k8sClient.Update(t.ctx, accountExportResource))

	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)

	accountExport := &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	t.Require().NotNil(accountExport.Status.DesiredClaim)
	t.Require().Equal(int64(1), accountExport.Status.DesiredClaim.ObservedGeneration)
	t.Require().Equal(t.rules, accountExport.Status.DesiredClaim.Rules)

	conditions := accountExport.Status.Conditions
	t.Require().NotEmpty(conditions)

	t.assertBoundAccountID(accountExport, t.accountIDA)
	t.assertCondition(conditions, conditionTypeValidRules, metav1.ConditionFalse, conditionReasonInvalid)
	t.assertCondition(conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonReconciling)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldFail_WhenAccountIDChangeDetected() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): t.accountIDA,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: t.accountNameA,
			Rules:       t.rules,
		},
	}))
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})
	t.Require().NoError(err)

	accountExport := &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	accountExport.Spec.AccountName = t.accountNameB
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
	boundToAccountCondition := t.assertCondition(conditions, conditionTypeBoundToAccount, metav1.ConditionFalse, conditionReasonConflict)
	t.Equal("account export is already bound to account: ACCA", boundToAccountCondition.Message)
	t.Equalf(t.accountIDB, accountExport.Status.AccountID, "Expected Status.AccountID")
	t.Equalf(t.accountIDA, accountExport.GetLabel(v1alpha1.AccountExportLabelAccountID), "Expected label %q", v1alpha1.AccountExportLabelAccountID)

	t.assertCondition(conditions, conditionTypeValidRules, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(conditions, conditionTypeAdoptedByAccount, metav1.ConditionFalse, conditionReasonAdopting)
	t.assertCondition(conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonReconciling)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldBindAccount() {
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: t.accountNameA,
			Rules: []v1alpha1.AccountExportRule{
				{Type: v1alpha1.Stream, Name: "dummy rule", Subject: "mysubject"},
			},
		},
	}))

	// When
	res, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)
	t.NotEmpty(res)
	t.Equal(time.Millisecond*250, res.RequeueAfter, "Should be requeued")

	accountExport := &v1alpha1.AccountExport{}
	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	t.Equalf(t.accountIDA, accountExport.GetLabel(v1alpha1.AccountExportLabelAccountID), "Expected label %q", v1alpha1.AccountExportLabelAccountID)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldBeAdoptedByAccount() {
	accountExport := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): t.accountIDA,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: t.accountNameA,
			Rules: []v1alpha1.AccountExportRule{
				{Type: v1alpha1.Stream, Name: "dummy rule", Subject: "mysubject"},
			},
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, accountExport))

	accountA := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(ctx, ktypes.NamespacedName{Namespace: t.accountExportNamespace, Name: t.accountNameA}, accountA))
	accountA.Status.Adoptions = &v1alpha1.AccountAdoptions{
		Exports: []v1alpha1.AccountAdoption{
			{
				Name:               "export",
				ObservedGeneration: int64(1),
				UID:                accountExport.UID,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionTrue,
					DesiredClaimObservedGeneration: &accountExport.Generation,
					Reason:                         conditionReasonOK,
					Message:                        "",
				},
			},
		},
	}
	t.Require().NoError(k8sClient.Status().Update(ctx, accountA))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	t.Equalf(t.accountIDA, accountExport.GetLabel(v1alpha1.AccountExportLabelAccountID), "Expected label %q", v1alpha1.AccountExportLabelAccountID)

	conditions := accountExport.Status.Conditions
	t.Require().NotEmpty(conditions)

	t.assertCondition(conditions, conditionTypeAdoptedByAccount, metav1.ConditionTrue, conditionReasonOK)
	t.assertCondition(conditions, conditionTypeReady, metav1.ConditionTrue, conditionReasonOK)
}

func (t *AccountExportControllerTestSuite) Test_Reconcile_ShouldNotBeAdoptedByAccount_WhenFailures() {
	accountExport := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountExportName,
			Namespace: t.accountExportNamespace,
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): t.accountIDA,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: t.accountNameA,
			Rules: []v1alpha1.AccountExportRule{
				{Type: v1alpha1.Stream, Name: "dummy rule", Subject: "."},
			},
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, accountExport))

	accountA := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(ctx, ktypes.NamespacedName{Namespace: t.accountExportNamespace, Name: t.accountNameA}, accountA))
	accountA.Status.Adoptions = &v1alpha1.AccountAdoptions{
		Exports: []v1alpha1.AccountAdoption{
			{
				Name:               "export",
				ObservedGeneration: int64(1),
				UID:                accountExport.UID,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionFalse,
					DesiredClaimObservedGeneration: &accountExport.Generation,
					Reason:                         conditionReasonInvalid,
					Message:                        "Invalid export",
				},
			},
		},
	}
	t.Require().NoError(k8sClient.Status().Update(ctx, accountA))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountExportRef})

	// Then
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountExportRef, accountExport)
	t.Require().NoError(err)
	t.Equalf(t.accountIDA, accountExport.GetLabel(v1alpha1.AccountExportLabelAccountID), "Expected label %q", v1alpha1.AccountExportLabelAccountID)

	conditions := accountExport.Status.Conditions
	t.Require().NotEmpty(conditions)

	t.assertCondition(conditions, conditionTypeAdoptedByAccount, metav1.ConditionFalse, conditionReasonFailed)
	t.assertCondition(conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonReconciling)
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

func (t *AccountExportControllerTestSuite) assertBoundAccountID(export *v1alpha1.AccountExport, expectAccountID string) {
	t.Equalf(expectAccountID, export.GetLabel(v1alpha1.AccountExportLabelAccountID), "Expected label %q", v1alpha1.AccountExportLabelAccountID)
	t.Equalf(expectAccountID, export.Status.AccountID, "Expected Status.AccountID")
}

func Test_findAdoptionByUID(t *testing.T) {
	adoptions := []v1alpha1.AccountAdoption{
		{UID: "export-1", Name: "first"},
		{UID: "export-2", Name: "second"},
	}

	tests := []struct {
		name    string
		account *v1alpha1.Account
		uid     ktypes.UID
		want    *v1alpha1.AccountAdoption
	}{
		{
			name:    "nil_adoptions",
			account: &v1alpha1.Account{},
			uid:     "export-1",
			want:    nil,
		},
		{
			name: "matching_uid",
			account: &v1alpha1.Account{
				Status: v1alpha1.AccountStatus{
					Adoptions: &v1alpha1.AccountAdoptions{Exports: adoptions},
				},
			},
			uid:  "export-2",
			want: &adoptions[1],
		},
		{
			name: "missing_uid",
			account: &v1alpha1.Account{
				Status: v1alpha1.AccountStatus{
					Adoptions: &v1alpha1.AccountAdoptions{Exports: adoptions},
				},
			},
			uid:  "export-3",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAdoptionByUID(tt.account, tt.uid)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_mapToExportRule(t *testing.T) {
	responseThreshold := 25 * time.Millisecond
	accountTokenPosition := uint(3)
	advertise := true
	allowTrace := true

	t.Run("maps_all_fields", func(t *testing.T) {
		rule := v1alpha1.AccountExportRule{
			Name:              "my-export",
			Subject:           "subject.>",
			Type:              v1alpha1.Service,
			ResponseType:      v1alpha1.ResponseType("Singleton"),
			ResponseThreshold: &responseThreshold,
			Latency: &v1alpha1.ServiceLatency{
				Sampling: 50,
				Results:  "latency.results",
			},
			AccountTokenPosition: &accountTokenPosition,
			Advertise:            &advertise,
			AllowTrace:           &allowTrace,
		}

		want := nauth.ExportRule{
			Name:              "my-export",
			Subject:           nauth.Subject("subject.>"),
			Type:              nauth.ExportTypeService,
			ResponseType:      nauth.ResponseType("Singleton"),
			ResponseThreshold: responseThreshold,
			Latency: &nauth.ServiceLatency{
				Sampling: 50,
				Results:  nauth.Subject("latency.results"),
			},
			AccountTokenPosition: 3,
			Advertise:            true,
			AllowTrace:           true,
		}

		assert.Equal(t, want, mapToExportRule(rule))
	})

	t.Run("keeps_zero_values_when_optional_fields_missing", func(t *testing.T) {
		rule := v1alpha1.AccountExportRule{
			Subject: "subject",
			Type:    v1alpha1.Stream,
		}

		want := nauth.ExportRule{
			Subject: nauth.Subject("subject"),
			Type:    nauth.ExportTypeStream,
		}

		assert.Equal(t, want, mapToExportRule(rule))
	})
}

func Test_isAccountExportReady(t *testing.T) {
	trueCondition := func(conditionType string) metav1.Condition {
		return newCondition(conditionType, metav1.ConditionTrue, conditionReasonOK, "")
	}

	createExport := func(generation int64, desiredClaim *v1alpha1.AccountExportClaim, conditions ...metav1.Condition) *v1alpha1.AccountExport {
		return &v1alpha1.AccountExport{
			ObjectMeta: metav1.ObjectMeta{Generation: generation},
			Status: v1alpha1.AccountExportStatus{
				DesiredClaim: desiredClaim,
				Conditions:   conditions,
			},
		}
	}

	tests := []struct {
		name   string
		export *v1alpha1.AccountExport
		want   bool
	}{
		{
			name: "all_conditions_true_and_claim_matches_generation",
			export: createExport(
				2,
				&v1alpha1.AccountExportClaim{ObservedGeneration: 2},
				trueCondition(conditionTypeBoundToAccount),
				trueCondition(conditionTypeValidRules),
				trueCondition(conditionTypeAdoptedByAccount),
			),
			want: true,
		},
		{
			name: "missing_desired_claim",
			export: createExport(
				2,
				nil,
				trueCondition(conditionTypeBoundToAccount),
				trueCondition(conditionTypeValidRules),
				trueCondition(conditionTypeAdoptedByAccount),
			),
			want: false,
		},
		{
			name: "stale_claim_generation",
			export: createExport(
				2,
				&v1alpha1.AccountExportClaim{ObservedGeneration: 1},
				trueCondition(conditionTypeBoundToAccount),
				trueCondition(conditionTypeValidRules),
				trueCondition(conditionTypeAdoptedByAccount),
			),
			want: false,
		},
		{
			name: "missing_ready_condition_dependency",
			export: createExport(
				2,
				&v1alpha1.AccountExportClaim{ObservedGeneration: 2},
				trueCondition(conditionTypeBoundToAccount),
				newCondition(conditionTypeValidRules, metav1.ConditionFalse, conditionReasonInvalid, "invalid rules"),
				trueCondition(conditionTypeAdoptedByAccount),
			),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAccountExportReady(tt.export))
		})
	}
}
