package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AccountExportClient struct {
	reader client.Reader
}

func NewAccountExportClient(reader client.Reader) *AccountExportClient {
	return &AccountExportClient{
		reader: reader,
	}
}

func (a AccountExportClient) FindByAccountID(ctx context.Context, namespace domain.Namespace, accountID string) (*v1alpha1.AccountExportList, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account ID required")
	}
	exports := &v1alpha1.AccountExportList{}
	err := a.reader.List(ctx, exports, client.InNamespace(namespace), client.MatchingLabels{
		string(v1alpha1.AccountExportLabelAccountID): accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list account exports: %w", err)
	}
	return exports, nil
}

var _ outbound.AccountExportReader = (*AccountExportClient)(nil)
