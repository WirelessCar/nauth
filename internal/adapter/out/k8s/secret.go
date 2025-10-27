package k8s

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"

	"github.com/WirelessCar-WDP/nauth/internal/core/domain/errs"
	"github.com/WirelessCar-WDP/nauth/internal/core/ports"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SecretStorer struct {
	client              client.Client
	controllerNamespace string
}

func NewK8sSecretStorer(client client.Client) *SecretStorer {
	k8sSecretStorer := &SecretStorer{}

	namespacePath, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		log.Fatalf("Failed to read namespace: %v", err)
	}

	k8sSecretStorer.controllerNamespace = string(namespacePath)
	k8sSecretStorer.client = client

	return k8sSecretStorer
}

func (k SecretStorer) ApplySecret(ctx context.Context, owner *ports.SecretOwner, meta metav1.ObjectMeta, valueMap map[string]string) error {
	log := logf.FromContext(ctx)

	newSecret := &v1.Secret{
		ObjectMeta: meta,
		StringData: valueMap,
	}

	log.Info("Trying to create secret", "namespace", meta.GetNamespace(), "secretName", meta.GetName())

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner.Owner, newSecret, k.client.Scheme()); err != nil {
			return fmt.Errorf("failed to link secret to owner: %w", err)
		}
	}

	currentSecret, err := k.getSecret(ctx, meta.GetNamespace(), meta.GetName())
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get secret: %w", err)
		}
		if err := k.client.Create(ctx, newSecret); err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
	} else {
		newSecret = currentSecret
		newSecret.StringData = valueMap

		if err := addOwnerReferenceIfNotExists(newSecret, owner); err != nil {
			return err
		}

		err = k.client.Update(ctx, newSecret)
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
	}

	return nil
}

func addOwnerReferenceIfNotExists(secret *v1.Secret, secretOwner *ports.SecretOwner) error {
	if secretOwner == nil {
		return nil
	}

	owner := secretOwner.Owner
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

func (k SecretStorer) GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error) {
	secret, err := k.getSecret(ctx, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errs.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	secretData := make(map[string]string)
	for k, v := range secret.Data {
		secretData[k] = string(v)
	}

	return secretData, nil
}

func (k SecretStorer) GetSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error) {
	return k.getSecretsByLabels(ctx, namespace, labels)
}

func (k SecretStorer) DeleteSecret(ctx context.Context, namespace string, name string) error {
	log := logf.FromContext(ctx)

	secret, err := k.getSecret(ctx, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get secret while deleting: %w", err)
	}

	log.Info("Trying to delete secret", "secretName", name)
	if err := k.client.Delete(ctx, secret); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

func (k SecretStorer) DeleteSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) error {
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

func (k SecretStorer) LabelSecret(ctx context.Context, namespace string, name string, labels map[string]string) error {
	secret, err := k.getSecret(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	if secret.GetLabels() == nil {
		secret.Labels = make(map[string]string, len(labels))
	}

	maps.Copy(secret.Labels, labels)
	return k.client.Update(ctx, secret)
}

func (k SecretStorer) getSecret(ctx context.Context, namespace string, name string) (*v1.Secret, error) {
	k8sSecret := &v1.Secret{}

	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := k.client.Get(ctx, key, k8sSecret); err != nil {
		return nil, err
	}
	return k8sSecret, nil
}

func (k SecretStorer) getSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error) {
	secretList := &v1.SecretList{}
	matchingLabelsListOption := client.MatchingLabels{}
	maps.Copy(matchingLabelsListOption, labels)

	if err := k.client.List(ctx, secretList, client.InNamespace(namespace), matchingLabelsListOption); err != nil {
		return nil, err
	}
	return secretList, nil
}
