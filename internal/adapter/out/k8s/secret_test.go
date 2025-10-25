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

package k8s

import (
	"context"

	"github.com/WirelessCar-WDP/nauth/internal/core/domain/errs"
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
		}
		ctx := context.Background()
		var secretStorer *SecretStorer

		BeforeEach(func() {
			By("creating the custom resource for the Kind Account")
			secretStorer = &SecretStorer{
				client:              k8sClient,
				controllerNamespace: secretMeta.Namespace,
			}
		})

		AfterEach(func() {
			By("Cleanup the secret")
			err := cleanSecret(secretMeta.Namespace, secretMeta.Name)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should successfully create and update an existing secret", func() {
			By("Creating a new secret from scratch")
			secret := map[string]string{"key": "value"}
			err := secretStorer.ApplySecret(ctx, nil, secretMeta, secret)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the secret")
			fetchedSecret, err := secretStorer.GetSecret(ctx, namespace, resourceName)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchedSecret).ToNot(BeNil())
			Expect(fetchedSecret).To(Equal(secret))

			By("Updating the secret with a new value")
			newSecret := map[string]string{"key": "new value"}
			err = secretStorer.ApplySecret(ctx, nil, secretMeta, newSecret)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the updated secret")
			newFetchedSecret, err := secretStorer.GetSecret(ctx, namespace, resourceName)
			Expect(err).ToNot(HaveOccurred())
			Expect(newFetchedSecret).ToNot(BeNil())
			Expect(newFetchedSecret).To(Equal(newSecret))
		})
		It("should return success when deleting a non existing secret", func() {
			By("Trying to delete a non-existing secret")
			err := secretStorer.DeleteSecret(ctx, namespace, "non-existing-secret")
			Expect(err).ToNot(HaveOccurred())
		})
		It("should return an error when the secret does not exist", func() {
			By("Trying to retrieve a non-existing secret")
			_, err := secretStorer.GetSecret(ctx, namespace, "non-existing-secret")
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(errs.ErrNotFound))
		})
		It("should return success when deleting existing secret", func() {
			By("Creating a new secret from scratch")
			secret := map[string]string{"key": "value"}
			err := secretStorer.ApplySecret(ctx, nil, secretMeta, secret)
			Expect(err).ToNot(HaveOccurred())

			By("Deleting the secret")
			err = secretStorer.DeleteSecret(ctx, namespace, resourceName)
			Expect(err).ToNot(HaveOccurred())

			By("Retrieving the deleted secret")
			_, err = secretStorer.GetSecret(ctx, namespace, resourceName)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(errs.ErrNotFound))
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
