package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubernetesClient struct {
	client.Client
}

func newKubernetesClient(k8sClient client.Client) *kubernetesClient {
	return &kubernetesClient{
		Client: k8sClient,
	}
}

type metadataPatch struct {
	Metadata labelsPatch `json:"metadata"`
}

type labelsPatch struct {
	Labels map[string]string `json:"labels"`
}

func (c *kubernetesClient) PatchLabels(ctx context.Context, resource client.Object) error {
	patchData, err := json.Marshal(metadataPatch{
		Metadata: labelsPatch{
			Labels: resource.GetLabels(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to generate label patch: %w", err)
	}
	if err = c.Patch(ctx, resource, client.RawPatch(types.MergePatchType, patchData)); err != nil {
		return fmt.Errorf("failed to patch labels: %w", err)
	}
	return nil
}
