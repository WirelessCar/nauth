package k8s

import (
	"context"
	"errors"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountClient struct {
	client client.Client
}

func NewAccountClient(client client.Client) *AccountClient {
	ag := &AccountClient{
		client: client,
	}

	return ag
}

// Get the referenced Account
// Requires the account to be reconciled and ready
func (a *AccountClient) Get(ctx context.Context, accountRef domain.NamespacedName) (account *v1alpha1.Account, err error) {
	log := logf.FromContext(ctx)
	if err = accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}
	key := client.ObjectKey{Namespace: accountRef.Namespace, Name: accountRef.Name}

	account = &v1alpha1.Account{}
	err = a.client.Get(ctx, key, account)
	if err != nil {
		return nil, err
	}

	if !isReady(account) {
		log.Error(err, "account is not ready", "accountRef", accountRef)
		return nil, errors.Join(ErrAccountNotReady, err)
	}

	return account, err
}

func isReady(account *v1alpha1.Account) bool {
	return account.GetAccountID() != ""
}

// Compile-time assertion that implementation satisfies the ports interface
var _ outbound.AccountReader = (*AccountClient)(nil)
