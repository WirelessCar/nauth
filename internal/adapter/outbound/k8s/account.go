package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AccountReader interface {
	Get(ctx context.Context, accountRef domain.NamespacedName) (*v1alpha1.Account, error)
	GetAccountID(ctx context.Context, accountRef domain.NamespacedName) (nauth.AccountID, error)
}

type AccountClient struct {
	client client.Client
}

func NewAccountClient(client client.Client) *AccountClient {
	return &AccountClient{
		client: client,
	}
}

// Get the referenced Account
// Requires the account to be reconciled and ready
func (a *AccountClient) Get(ctx context.Context, accountRef domain.NamespacedName) (*v1alpha1.Account, error) {
	account, err := a.get(ctx, accountRef)
	if err != nil {
		return nil, err
	}

	if !isReady(account) {
		return nil, domain.ErrAccountNotReady
	}
	return account, nil
}

func (a *AccountClient) GetAccountID(ctx context.Context, accountRef domain.NamespacedName) (nauth.AccountID, error) {
	account, err := a.Get(ctx, accountRef)
	if err != nil {
		return "", err
	}

	accountID := account.GetLabel(v1alpha1.AccountLabelAccountID)
	if accountID == "" {
		return "", domain.ErrAccountNotReady
	}
	return nauth.AccountID(accountID), nil
}

func (a *AccountClient) get(ctx context.Context, accountRef domain.NamespacedName) (*v1alpha1.Account, error) {
	if err := accountRef.Validate(); err != nil {
		return nil, domain.ErrBadRequest.WithCause(fmt.Errorf("invalid reference %q: %w", accountRef, err))
	}
	key := client.ObjectKey{Namespace: accountRef.Namespace, Name: accountRef.Name}

	account := &v1alpha1.Account{}
	if err := a.client.Get(ctx, key, account); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, domain.ErrAccountNotFound
		}
		return nil, domain.ErrUnknownError.WithCause(fmt.Errorf("failed to get account %q: %w", accountRef, err))
	}
	return account, nil
}

func isReady(account *v1alpha1.Account) bool {
	return account.GetLabel(v1alpha1.AccountLabelAccountID) != ""
}

// Compile-time assertion that implementation satisfies the ports interface
var _ AccountReader = (*AccountClient)(nil)
var _ outbound.AccountIDReader = (*AccountClient)(nil)
