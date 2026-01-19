/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/WirelessCar/nauth/internal/account"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

const (
	accountName        = "test-account"
	accountNamespace   = "test-account-ns"
	opNamespace        = "nauth-operator-system"
	opSigningKeySeed   = "SOAF43LTJSU54DLV5VPWKF2ROVF2V6FZZG662Z2CCHDAFKCK5JGLQRP7SA"
	opSigningKeyPublic = "OD32J3IJ3TNSPNAG2MHKB7F77ETVXGUBYMGXQ2ONNJCKPM5RXO7XWTFD"
)

func TestAccountReconciliation_ShouldSucceed_WhenNewAccountHasDefaultSpec(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)

	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("Disconnect").Return()

	err := testCtx.k8sClient.Create(context.Background(), &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{},
	})
	require.NoError(t, err, "Failed to create account resource in k8s")

	// === WHEN ===
	_, err = testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.NoError(t, err, "Reconciliation should succeed")

	var updatedAccount natsv1alpha1.Account
	err = testCtx.k8sClient.Get(context.Background(), types.NamespacedName{
		Name:      accountName,
		Namespace: accountNamespace,
	}, &updatedAccount)
	require.NoError(t, err, "Should be able to get updated account")

	assert.NotEmpty(t, updatedAccount.Labels[k8s.LabelAccountID], "Account ID label should be set")
	assert.Equal(t, opSigningKeyPublic, updatedAccount.Labels[k8s.LabelAccountSignedBy], "Account signed by label should match operator signing key")
	assert.NotNil(t, updatedAccount.Status.Claims, "Status claims should be populated")
	assert.Contains(t, updatedAccount.Finalizers, "account.nauth.io/finalizer", "Account should have finalizer")

	testCtx.natsClientMock.AssertCalled(t, "UploadAccountJWT", mock.Anything)

	secretList := &corev1.SecretList{}
	err = testCtx.k8sClient.List(context.Background(), secretList, client.InNamespace(accountNamespace), client.MatchingLabels{
		k8s.LabelAccountID: updatedAccount.Labels[k8s.LabelAccountID],
	})
	require.NoError(t, err, "Should be able to list account secrets")
	assert.Len(t, secretList.Items, 2, "Should have created 2 secrets (root + signing key)")

	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldSucceed_WhenNewAccountHasLimits(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)

	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("Disconnect").Return()

	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{
			AccountLimits: &natsv1alpha1.AccountLimits{
				Conn:         ptr.To(int64(100)),
				LeafNodeConn: ptr.To(int64(50)),
			},
			JetStreamLimits: &natsv1alpha1.JetStreamLimits{
				Streams:  ptr.To(int64(10)),
				Consumer: ptr.To(int64(20)),
			},
		},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource in k8s")

	// === WHEN ===
	_, err = testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      accountName,
			Namespace: accountNamespace,
		},
	})

	// === THEN ===
	require.NoError(t, err, "Reconciliation should succeed")

	var updatedAccount natsv1alpha1.Account
	err = testCtx.k8sClient.Get(context.Background(), types.NamespacedName{
		Name:      accountName,
		Namespace: accountNamespace,
	}, &updatedAccount)
	require.NoError(t, err, "Should be able to get updated account")

	assert.NotEmpty(t, updatedAccount.Labels[k8s.LabelAccountID], "Account ID label should be set")
	assert.Equal(t, opSigningKeyPublic, updatedAccount.Labels[k8s.LabelAccountSignedBy], "Account signed by label should match operator signing key")
	assert.NotNil(t, updatedAccount.Status.Claims, "Status claims should be populated")
	assert.Contains(t, updatedAccount.Finalizers, "account.nauth.io/finalizer", "Account should have finalizer")

	require.NotNil(t, updatedAccount.Status.Claims.AccountLimits, "AccountLimits should be set")
	assert.Equal(t, int64(100), *updatedAccount.Status.Claims.AccountLimits.Conn, "Conn limit should match")
	assert.Equal(t, int64(50), *updatedAccount.Status.Claims.AccountLimits.LeafNodeConn, "LeafNodeConn limit should match")

	require.NotNil(t, updatedAccount.Status.Claims.JetStreamLimits, "JetStreamLimits should be set")
	assert.Equal(t, int64(10), *updatedAccount.Status.Claims.JetStreamLimits.Streams, "Streams limit should match")
	assert.Equal(t, int64(20), *updatedAccount.Status.Claims.JetStreamLimits.Consumer, "Consumer limit should match")

	testCtx.natsClientMock.AssertCalled(t, "UploadAccountJWT", mock.Anything)

	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldDoNothing_WhenAccountNotFoundDueToDeleted(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)

	// === WHEN ===
	result, err := testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.NoError(t, err, "Reconciliation should not error for non-existent account")
	assert.Equal(t, reconcile.Result{}, result, "Should return empty result for non-existent account")

	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldDoNothing_WhenGenerationAndOperatorVersionAreSame(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)

	// Set operator version for this test
	operatorVersion := "v1.0.0-test"
	_ = os.Setenv(controller.EnvOperatorVersion, operatorVersion)
	defer func() { _ = os.Unsetenv(controller.EnvOperatorVersion) }()

	// Create and reconcile account first time
	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("Disconnect").Return()

	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource")

	// Reconcile to create the account (part of Given setup)
	_, err = testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})
	require.NoError(t, err, "Initial reconciliation should succeed")

	// Get the reconciled account and verify initial state
	var reconciledAccount natsv1alpha1.Account
	err = testCtx.k8sClient.Get(context.Background(), types.NamespacedName{
		Name:      accountName,
		Namespace: accountNamespace,
	}, &reconciledAccount)
	require.NoError(t, err, "Should be able to get reconciled account")
	assert.Equal(t, int64(1), reconciledAccount.Status.ObservedGeneration, "ObservedGeneration should be 1")
	assert.Equal(t, operatorVersion, reconciledAccount.Status.OperatorVersion, "OperatorVersion should match the env var")

	// === WHEN ===
	// Reconcile again without any changes (no mock expectations set for second reconcile)
	_, err = testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.NoError(t, err, "Second reconciliation should succeed without doing anything")

	// Verify the account hasn't changed
	var unchangedAccount natsv1alpha1.Account
	err = testCtx.k8sClient.Get(context.Background(), types.NamespacedName{
		Name:      accountName,
		Namespace: accountNamespace,
	}, &unchangedAccount)
	require.NoError(t, err, "Should be able to get account")

	// Verify status is exactly the same
	assert.Equal(t, reconciledAccount.Status.ObservedGeneration, unchangedAccount.Status.ObservedGeneration, "ObservedGeneration should not change")
	assert.Equal(t, reconciledAccount.Status.OperatorVersion, unchangedAccount.Status.OperatorVersion, "OperatorVersion should not change")
	assert.Equal(t, reconciledAccount.ResourceVersion, unchangedAccount.ResourceVersion, "ResourceVersion should not change (no updates made)")

	// Verify no NATS operations were performed on second reconcile
	// The expectations set earlier should still match exactly - no new calls
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldSucceed_WhenAccountIsMarkedForDeletion(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)

	// Create account first
	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("Disconnect").Return()

	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource")

	// Reconcile to create the account (part of Given setup)
	_, err = testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})
	require.NoError(t, err, "Initial reconciliation should succeed")

	// Get the created account and prepare for deletion
	var createdAccount natsv1alpha1.Account
	err = testCtx.k8sClient.Get(context.Background(), types.NamespacedName{
		Name:      accountName,
		Namespace: accountNamespace,
	}, &createdAccount)
	require.NoError(t, err, "Should be able to get created account")

	// Setup expectations for deletion
	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("Disconnect").Return()

	// Delete the account (this sets the deletion timestamp)
	err = testCtx.k8sClient.Delete(context.Background(), &createdAccount)
	require.NoError(t, err, "Failed to delete account resource")

	// === WHEN ===
	_, err = testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.NoError(t, err, "Reconciliation should succeed during deletion")

	// Verify the account JWT was deleted from NATS
	testCtx.natsClientMock.AssertCalled(t, "DeleteAccountJWT", mock.Anything)

	// Verify the secrets were deleted
	secretList := &corev1.SecretList{}
	err = testCtx.k8sClient.List(context.Background(), secretList, client.InNamespace(accountNamespace), client.MatchingLabels{
		k8s.LabelAccountID: createdAccount.Labels[k8s.LabelAccountID],
	})
	require.NoError(t, err, "Should be able to list secrets")
	assert.Empty(t, secretList.Items, "Account secrets should have been deleted")
	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldFail_WhenOperatorSigningKeyNotFound(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)

	// Note: We deliberately do NOT create the operator namespace or signing key

	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource in k8s")

	// === WHEN ===
	result, err := testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.Error(t, err, "Reconciliation should fail without operator signing key")
	assert.Contains(t, err.Error(), "failed to create the account", "Error should mention account creation failure")
	assert.Contains(t, err.Error(), "missing operator signing key", "Error should mention missing operator signing key")
	assert.Equal(t, reconcile.Result{}, result, "Should return empty result on error")

	testCtx.natsClientMock.AssertNotCalled(t, "UploadAccountJWT")

	secretList := &corev1.SecretList{}
	err = testCtx.k8sClient.List(context.Background(), secretList, client.InNamespace(accountNamespace))
	require.NoError(t, err, "Should be able to list secrets")
	assert.Empty(t, secretList.Items, "Should not have created any secrets")

	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldFail_WhenUploadAccountJWTReturnsError(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)

	// Expect NATS operations but UploadAccountJWT will fail
	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(nil)
	testCtx.natsClientMock.On("UploadAccountJWT", mock.Anything).Return(assert.AnError)
	testCtx.natsClientMock.On("Disconnect").Return()

	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource in k8s")

	// === WHEN ===
	result, err := testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.Error(t, err, "Reconciliation should fail when UploadAccountJWT returns error")
	assert.Contains(t, err.Error(), "failed to create the account", "Error should mention account creation failure")
	assert.Contains(t, err.Error(), "failed to upload account jwt", "Error should mention upload JWT failure")
	assert.Equal(t, reconcile.Result{}, result, "Should return empty result on error")

	// Verify UploadAccountJWT was called
	testCtx.natsClientMock.AssertCalled(t, "UploadAccountJWT", mock.Anything)

	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

func TestAccountReconciliation_ShouldFail_WhenUnableToConnectToNATS(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)

	// Expect EnsureConnected to fail
	testCtx.natsClientMock.On("EnsureConnected", mock.Anything).Return(assert.AnError)

	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource in k8s")

	// === WHEN ===
	result, err := testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.Error(t, err, "Reconciliation should fail when unable to connect to NATS")
	assert.Contains(t, err.Error(), "failed to create the account", "Error should mention account creation failure")
	assert.Contains(t, err.Error(), "failed to connect to NATS cluster", "Error should mention NATS connection failure")
	assert.Equal(t, reconcile.Result{}, result, "Should return empty result on error")

	// Verify EnsureConnected was called
	testCtx.natsClientMock.AssertCalled(t, "EnsureConnected", mock.Anything)

	// Verify UploadAccountJWT was NOT called (we failed before reaching that point)
	testCtx.natsClientMock.AssertNotCalled(t, "UploadAccountJWT")

	// Verify Disconnect was NOT called (we never successfully connected)
	testCtx.natsClientMock.AssertNotCalled(t, "Disconnect")

	// Verify no unexpected mock calls were made
	testCtx.natsClientMock.AssertExpectations(t)
}

// TestAccountReconciliation_ShouldFail_WhenImportAccountNotFound verifies that reconciliation
// fails when an account has an import that references a non-existent account.
//
// NOTE: This behavior might change when https://github.com/WirelessCar/nauth/issues/11 is implemented.
// The current implementation fails fast during claims building if an imported account cannot be found.
// Future implementations may handle this differently (e.g., requeue for later retry, or allow
// partial reconciliation with missing imports).
func TestAccountReconciliation_ShouldFail_WhenImportAccountNotFound(t *testing.T) {
	// === GIVEN ===
	testCtx := setupIntegrationTest(t)
	setupOperatorSigningKey(t, testCtx.k8sClient)
	importAccountName := "import-account"
	importAccountNamespace := "import-account-ns"

	// Create an account with an import that references a non-existent account
	accountResource := &natsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: natsv1alpha1.AccountSpec{
			Imports: natsv1alpha1.Imports{
				{
					Name:    "test-import",
					Subject: "test.subject",
					Type:    natsv1alpha1.Stream,
					AccountRef: natsv1alpha1.AccountRef{
						Name:      importAccountName,
						Namespace: importAccountNamespace,
					},
				},
			},
		},
	}
	err := testCtx.k8sClient.Create(context.Background(), accountResource)
	require.NoError(t, err, "Failed to create account resource in k8s")

	// Mock the AccountGetter to return an error when looking up the imported account
	testCtx.accountGetterMock.On("Get", mock.Anything, importAccountName, importAccountNamespace).
		Return((*natsv1alpha1.Account)(nil), assert.AnError)

	// We expect that claims building will fail, so NATS operations should NOT be called
	// However, add these expectations to make the test more explicit about what should NOT happen
	// (If these ARE called, the test will fail with unexpected mock calls)

	// === WHEN ===
	result, err := testCtx.reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: accountName, Namespace: accountNamespace},
	})

	// === THEN ===
	require.Error(t, err, "Reconciliation should fail when import account not found")
	assert.Contains(t, err.Error(), "failed to create the account", "Error should mention account creation failure")
	assert.Contains(t, err.Error(), "failed to build NATS account claims", "Error should mention claims build failure")
	assert.Contains(t, err.Error(), "failed to get account for import", "Error should mention import resolution failure")
	assert.Equal(t, reconcile.Result{}, result, "Should return empty result on error")

	// Verify AccountGetter was called
	testCtx.accountGetterMock.AssertCalled(t, "Get", mock.Anything, importAccountName, importAccountNamespace)

	// Verify NATS operations were NOT called (we failed before reaching that point)
	testCtx.natsClientMock.AssertNotCalled(t, "EnsureConnected")
	testCtx.natsClientMock.AssertNotCalled(t, "UploadAccountJWT")
	testCtx.natsClientMock.AssertNotCalled(t, "Disconnect")

	// Verify no unexpected mock calls were made
	testCtx.accountGetterMock.AssertExpectations(t)
	testCtx.natsClientMock.AssertExpectations(t)
}

// integrationTestContext holds all dependencies for integration testing
type integrationTestContext struct {
	t                 *testing.T
	ctx               context.Context
	k8sClient         client.Client
	reconciler        *controller.AccountReconciler
	natsClientMock    *mockNatsClient
	accountGetterMock *mockAccountGetter
}

// setupIntegrationTest sets up the complete integration test environment
// This creates a test Kubernetes cluster with CRDs and the test namespace
func setupIntegrationTest(t *testing.T) *integrationTestContext {
	// Setup Kubernetes test cluster
	err := natsv1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "charts", "nauth", "resources", "crds")},
		ErrorIfCRDPathMissing: true,
	}

	// Set binary assets directory for envtest
	if binDir := getFirstFoundEnvTestBinaryDir(); binDir != "" {
		testEnv.BinaryAssetsDirectory = binDir
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err)
	require.NotNil(t, k8sClient)

	t.Cleanup(func() {
		_ = testEnv.Stop()
	})

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: accountNamespace,
		},
	}
	err = k8sClient.Create(context.Background(), ns)
	require.NoError(t, err, "Failed to create test namespace")

	// Setup mocks (per test)
	natsClientMock := newMockNatsClient()
	accountGetterMock := newMockAccountGetter()

	// Use real secret client for integration testing
	// This allows the manager to read the actual operator signing key from Kubernetes
	// Provide the operator namespace explicitly since we're not running in a cluster
	secretClient := secret.NewClient(k8sClient, secret.WithControllerNamespace(opNamespace))

	// Create account manager with mocked dependencies
	// Note: The manager will look for the operator signing key in opNamespace
	accountManager := account.NewManager(
		accountGetterMock,
		natsClientMock,
		secretClient,
		account.WithNamespace(opNamespace), // Use cluster-level operator namespace
	)

	// Create controller with real account manager
	fakeRecorder := record.NewFakeRecorder(10)
	reconciler := controller.NewAccountReconciler(
		k8sClient,
		scheme.Scheme,
		accountManager,
		fakeRecorder,
	)

	return &integrationTestContext{
		t:                 t,
		ctx:               context.Background(),
		k8sClient:         k8sClient,
		reconciler:        reconciler,
		natsClientMock:    natsClientMock,
		accountGetterMock: accountGetterMock,
	}
}

// setupOperatorSigningKey creates the operator signing key in the operator namespace
func setupOperatorSigningKey(t *testing.T, k8sClient client.Client) {
	// Ensure operator namespace exists
	var operatorNs corev1.Namespace
	err := k8sClient.Get(context.Background(), types.NamespacedName{Name: opNamespace}, &operatorNs)
	if err != nil {
		// Namespace doesn't exist, create it
		operatorNs = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: opNamespace,
			},
		}
		err = k8sClient.Create(context.Background(), &operatorNs)
		require.NoError(t, err, "Failed to create operator namespace")
	}

	// Create the operator signing key secret with the provided seed
	operatorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operator-signing-key",
			Namespace: opNamespace,
			Labels: map[string]string{
				k8s.LabelSecretType: k8s.SecretTypeOperatorSign,
			},
		},
		StringData: map[string]string{
			k8s.DefaultSecretKeyName: opSigningKeySeed,
		},
	}
	err = k8sClient.Create(context.Background(), operatorSecret)
	require.NoError(t, err, "Failed to create operator signing key")
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// Mock implementations

type mockNatsClient struct {
	mock.Mock
}

func newMockNatsClient() *mockNatsClient {
	m := &mockNatsClient{}
	// No default expectations - let each test set up only what it needs
	// This allows AssertExpectations to catch unexpected calls
	return m
}

func (m *mockNatsClient) EnsureConnected(namespace string) error {
	args := m.Called(namespace)
	return args.Error(0)
}

func (m *mockNatsClient) Disconnect() {
	m.Called()
}

func (m *mockNatsClient) LookupAccountJWT(accountID string) (string, error) {
	args := m.Called(accountID)
	return args.String(0), args.Error(1)
}

func (m *mockNatsClient) UploadAccountJWT(jwt string) error {
	args := m.Called(jwt)
	return args.Error(0)
}

func (m *mockNatsClient) DeleteAccountJWT(jwt string) error {
	args := m.Called(jwt)
	return args.Error(0)
}

type mockAccountGetter struct {
	mock.Mock
}

func newMockAccountGetter() *mockAccountGetter {
	m := &mockAccountGetter{}
	// No default expectations - let each test set up only what it needs
	// This allows AssertExpectations to catch unexpected calls
	return m
}

func (m *mockAccountGetter) Get(ctx context.Context, accountRefName string, namespace string) (*natsv1alpha1.Account, error) {
	args := m.Called(ctx, accountRefName, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*natsv1alpha1.Account), args.Error(1)
}
