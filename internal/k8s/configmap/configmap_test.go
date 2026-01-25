package configmap

import (
	"context"

	"github.com/WirelessCar/nauth/internal/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ConfigMap client", func() {
	Context("When reading ConfigMaps", func() {
		const (
			configmapName = "test-configmap"
			namespace     = "default"
		)

		ctx := context.Background()
		var cmClient *Client

		BeforeEach(func() {
			cmClient = NewClient(k8sClient)
		})

		AfterEach(func() {
			By("cleaning up the ConfigMap")
			_ = cleanConfigMap(namespace, configmapName)
		})

		It("returns ErrNotFound when the ConfigMap does not exist", func() {
			By("getting a non-existing ConfigMap")
			_, err := cmClient.Get(ctx, namespace, "non-existing-configmap")
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(k8s.ErrNotFound))
		})

		It("retrieves ConfigMap Data keys", func() {
			By("creating a ConfigMap with Data")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configmapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"url":   "nats://nats.example.com:4222",
					"other": "value",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("getting the ConfigMap")
			data, err := cmClient.Get(ctx, namespace, configmapName)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(HaveKeyWithValue("url", "nats://nats.example.com:4222"))
			Expect(data).To(HaveKeyWithValue("other", "value"))
			Expect(data).To(HaveLen(2))
		})

		It("retrieves ConfigMap BinaryData keys", func() {
			By("creating a ConfigMap with BinaryData")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configmapName,
					Namespace: namespace,
				},
				BinaryData: map[string][]byte{
					"url": []byte("nats://nats.example.com:4222"),
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("getting the ConfigMap")
			data, err := cmClient.Get(ctx, namespace, configmapName)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(HaveKeyWithValue("url", "nats://nats.example.com:4222"))
			Expect(data).To(HaveLen(1))
		})

		It("retrieves both Data and BinaryData keys", func() {
			By("creating a ConfigMap with Data and BinaryData")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configmapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"data-key": "data-value",
				},
				BinaryData: map[string][]byte{
					"binary-key": []byte("binary-value"),
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("getting the ConfigMap")
			data, err := cmClient.Get(ctx, namespace, configmapName)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(HaveKeyWithValue("data-key", "data-value"))
			Expect(data).To(HaveKeyWithValue("binary-key", "binary-value"))
			Expect(data).To(HaveLen(2))
		})
	})
})

func cleanConfigMap(namespace string, name string) error {
	cm := &v1.ConfigMap{}
	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := k8sClient.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return k8sClient.Delete(ctx, cm)
}
