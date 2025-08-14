package k8s

import (
	"context"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Account getter", func() {
	Context("When getting account CRs", func() {
		const (
			accountName  = "test-account"
			namespace    = "default"
			resourceName = "test-resource"
		)

		var (
			ctx = context.Background()
		)

		BeforeEach(func() {

			By("creating account to fetch")
			err := createAccount(namespace, accountName)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			By("cleaning up the accounts")
			err := cleanAccount(namespace, accountName)
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not allow non-ready accounts by default", func() {
			By("setting up a default account getter")
			accountGetter := NewAccountGetter(k8sClient)
			By("getting the account")
			fetchedAccount, err := accountGetter.Get(ctx, accountName, namespace)

			By("verifying the fetched account")
			Expect(err).To(HaveOccurred())
			Expect(fetchedAccount).To(BeNil())
		})

		It("fetches the relevant ready account", func() {
			By("setting up a default account getter")
			accountGetter := NewAccountGetter(k8sClient)

			By("reconciling target account successfully")
			err := accountIsReady(namespace, accountName)
			Expect(err).ToNot(HaveOccurred())

			By("getting the account")
			fetchedAccount, err := accountGetter.Get(ctx, accountName, namespace)

			By("verifying the fetched account")
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchedAccount).ToNot(BeNil())
			Expect(fetchedAccount.Name).To(Equal(accountName))
		})

		It("should fetch the relevant account even if not ready if lenient", func() {
			By("setting up a lenient account getter")
			accountGetter := NewAccountGetter(k8sClient, WithLenient())
			By("getting the account")
			fetchedAccount, err := accountGetter.Get(ctx, accountName, namespace)

			By("verifying the fetched account")
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchedAccount).ToNot(BeNil())
			Expect(fetchedAccount.Name).To(Equal(accountName))
		})
	})
})

func createAccount(namespace string, name string) error {
	account := &v1alpha1.Account{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	return k8sClient.Create(ctx, account)
}

func accountIsReady(namespace string, name string) error {
	key := client.ObjectKey{Namespace: namespace, Name: name}
	account := &v1alpha1.Account{}

	err := k8sClient.Get(ctx, key, account)
	if err != nil {
		return err
	}

	accountLabels := map[string]string{
		domain.LabelAccountId: "account-id",
	}
	account.SetLabels(accountLabels)

	return k8sClient.Update(ctx, account)
}

func cleanAccount(namespace string, name string) error {
	account := &v1alpha1.Account{}

	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := k8sClient.Get(ctx, key, account); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	err := k8sClient.Delete(ctx, account)
	if err != nil {
		return err
	}
	return nil
}
