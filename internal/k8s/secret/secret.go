package secret

import (
	"context"
	"fmt"
	"maps"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Client struct {
	client client.Client
}

func NewClient(client client.Client) *Client {
	return &Client{
		client: client,
	}
}

func (k *Client) Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, valueMap map[string]string) error {
	if !isManagedSecret(&meta) {
		return fmt.Errorf("label %s not supplied by secret %s/%s", k8s.LabelManaged, meta.Namespace, meta.Name)
	}
	secretRef := domain.NewNamespacedName(meta.Namespace, meta.Name)
	currentSecret, err := k.getSecret(ctx, secretRef)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get secret: %w", err)
		}
		newSecret := &v1.Secret{
			ObjectMeta: meta,
			StringData: valueMap,
		}
		if owner != nil {
			if err := controllerutil.SetControllerReference(owner, newSecret, k.client.Scheme()); err != nil {
				return fmt.Errorf("failed to link secret to owner: %w", err)
			}
		}

		if err := k.client.Create(ctx, newSecret); err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
	} else {
		if !isManagedSecret(&currentSecret.ObjectMeta) {
			return fmt.Errorf("existing secret %s/%s not managed by nauth", meta.Namespace, meta.Name)
		}
		maps.Insert(currentSecret.Labels, maps.All(meta.Labels))

		currentSecret.StringData = valueMap
		if err := addOwnerReferenceIfNotExists(currentSecret, owner); err != nil {
			return err
		}

		err = k.client.Update(ctx, currentSecret)
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
	}

	return nil
}

func addOwnerReferenceIfNotExists(secret *v1.Secret, owner metav1.Object) error {
	if owner == nil {
		return nil
	}

	rtObj, ok := owner.(runtime.Object)

	if !ok {
		return fmt.Errorf("owner does not implement runtime.Object")
	}

	ownerGVK := rtObj.GetObjectKind().GroupVersionKind()
	ownerRef := metav1.OwnerReference{
		APIVersion: ownerGVK.GroupVersion().String(),
		Kind:       ownerGVK.Kind,
		Name:       owner.GetName(),
		UID:        owner.GetUID(),
	}

	alreadyExists := false

	for _, ref := range secret.OwnerReferences {
		if ref.UID == ownerRef.UID && ref.Kind == ownerRef.Kind && ref.APIVersion == ownerRef.APIVersion && ref.Name == ownerRef.Name {
			alreadyExists = true
			break
		}
	}

	if !alreadyExists {
		secret.OwnerReferences = append(secret.OwnerReferences, ownerRef)
	}

	return nil
}

func (k *Client) Get(ctx context.Context, secretRef domain.NamespacedName) (map[string]string, error) {
	secret, err := k.getSecret(ctx, secretRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, k8s.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	secretData := make(map[string]string)
	for key, value := range secret.Data {
		secretData[key] = string(value)
	}

	return secretData, nil
}

func (k *Client) GetByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) (*v1.SecretList, error) {
	return k.getSecretsByLabels(ctx, namespace, labels)
}

func (k *Client) Delete(ctx context.Context, secretRef domain.NamespacedName) error {
	log := logf.FromContext(ctx)

	secret, err := k.getSecret(ctx, secretRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get secret while deleting: %w", err)
	}

	log.Info("Trying to delete secret", "secretRef", secretRef)
	if err := k.client.Delete(ctx, secret); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

func (k *Client) DeleteByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) error {
	log := logf.FromContext(ctx)

	secrets, err := k.getSecretsByLabels(ctx, namespace, labels)
	if err != nil {
		return fmt.Errorf("failed to find secrets by label for deletion due to: %w", err)
	}
	if len(secrets.Items) < 1 {
		return nil
	}

	for _, secret := range secrets.Items {
		log.Info("trying to delete secret", "secretName", secret.GetName())
		if err := k.client.Delete(ctx, &secret); err != nil {
			return fmt.Errorf("failed to delete secret: %w", err)
		}
	}

	return nil
}

func (k *Client) Label(ctx context.Context, secretRef domain.NamespacedName, labels map[string]string) error {
	secret, err := k.getSecret(ctx, secretRef)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	if secret.GetLabels() == nil {
		secret.Labels = make(map[string]string, len(labels))
	}

	maps.Copy(secret.Labels, labels)
	return k.client.Update(ctx, secret)
}

func (k *Client) getSecret(ctx context.Context, secretRef domain.NamespacedName) (*v1.Secret, error) {
	if err := secretRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid namespaced name %q: %w", secretRef, err)
	}

	k8sSecret := &v1.Secret{}

	key := client.ObjectKey{Namespace: secretRef.Namespace, Name: secretRef.Name}
	if err := k.client.Get(ctx, key, k8sSecret); err != nil {
		return nil, err
	}
	return k8sSecret, nil
}

func (k *Client) getSecretsByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) (*v1.SecretList, error) {
	secretList := &v1.SecretList{}
	matchingLabelsListOption := client.MatchingLabels{}
	maps.Copy(matchingLabelsListOption, labels)

	if err := k.client.List(ctx, secretList, client.InNamespace(namespace), matchingLabelsListOption); err != nil {
		return nil, err
	}
	return secretList, nil
}

func isManagedSecret(meta *metav1.ObjectMeta) bool {
	return meta.Labels != nil && meta.Labels[k8s.LabelManaged] == k8s.LabelManagedValue
}

// Compile-time assertion that implementation satisfies the ports interface
var _ outbound.SecretClient = (*Client)(nil)
