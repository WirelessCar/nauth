package secret

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/internal/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Secrets storer", func() {
	Context("When handling secrets", func() {
		const resourceName = "test-resource"
		const namespace = "default"
		secretMeta := metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
			Labels: map[string]string{
				k8s.LabelManaged: k8s.LabelManagedValue,
			},
		}
		ctx := context.Background()
		var secretStorer *Client

		BeforeEach(func() {
			By("creating the custom resource for the Kind Account")
			secretStorer = NewClient(k8sClient, WithControllerNamespace(secretMeta.Namespace))
		})

		AfterEach(func() {
			By("Cleanup the secret")
			err := cleanSecret(secretMeta.Namespace, secretMeta.Name)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should successfully create and update an existing secret", func() {
			By("Creating a new secret from scratch")
			secret := map[string]string{"key": "value"}
			err := secretStorer.Apply(ctx, nil, secretMeta, secret)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the secret")
			fetchedSecret, err := secretStorer.Get(ctx, namespace, resourceName)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchedSecret).ToNot(BeNil())
			Expect(fetchedSecret).To(Equal(secret))

			By("Updating the secret with a new value")
			newSecret := map[string]string{"key": "new value"}
			err = secretStorer.Apply(ctx, nil, secretMeta, newSecret)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the updated secret")
			newFetchedSecret, err := secretStorer.Get(ctx, namespace, resourceName)
			Expect(err).ToNot(HaveOccurred())
			Expect(newFetchedSecret).ToNot(BeNil())
			Expect(newFetchedSecret).To(Equal(newSecret))
		})
		DescribeTable("should fail to update an existing secret not managed by nauth",
			func(existingSecretLabels map[string]string) {
				By("Creating the existing secret from scratch without managed label")
				existingSecret := map[string]string{"key": "value"}
				err := k8sClient.Create(ctx, &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
						Labels:    existingSecretLabels,
					},
					StringData: existingSecret,
				})
				Expect(err).ToNot(HaveOccurred())

				By("Trying to update the existing secret with a new value")
				newSecret := map[string]string{"key": "new value"}
				err = secretStorer.Apply(ctx, nil, secretMeta, newSecret)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fmt.Errorf("existing secret %s/%s not managed by nauth", namespace, resourceName)))

				By("Retrieving the secret again to verify not mutated")
				newFetchedSecret, err := secretStorer.Get(ctx, namespace, resourceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(newFetchedSecret).ToNot(BeNil())
				Expect(newFetchedSecret).To(Equal(existingSecret))
			},
			Entry("due to absent labels map",
				nil),
			Entry("due to empty labels map",
				map[string]string{}),
			Entry("due to irrelevant labels",
				map[string]string{"foo": "bar"}),
			Entry("due to existing managed label with unexpected value",
				map[string]string{k8s.LabelManaged: "false"}))

		It("should return success when deleting a non existing secret", func() {
			By("Trying to delete a non-existing secret")
			err := secretStorer.Delete(ctx, namespace, "non-existing-secret")
			Expect(err).ToNot(HaveOccurred())
		})
		It("should return an error when the secret does not exist", func() {
			By("Trying to retrieve a non-existing secret")
			_, err := secretStorer.Get(ctx, namespace, "non-existing-secret")
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(k8s.ErrNotFound))
		})
		It("should return success when deleting existing secret", func() {
			By("Creating a new secret from scratch")
			secret := map[string]string{"key": "value"}
			err := secretStorer.Apply(ctx, nil, secretMeta, secret)
			Expect(err).ToNot(HaveOccurred())

			By("Deleting the secret")
			err = secretStorer.Delete(ctx, namespace, resourceName)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the deleted secret")
			_, err = secretStorer.Get(ctx, namespace, resourceName)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(k8s.ErrNotFound))
		})
	})
})

func cleanSecret(namespace string, name string) error {
	k8sSecret := &v1.Secret{}

	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := k8sClient.Get(ctx, key, k8sSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	err := k8sClient.Delete(ctx, k8sSecret)
	if err != nil {
		return err
	}
	return nil
}
