package k8s

import (
	"context"
	"errors"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/errs"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountGetter struct {
	client  client.Client
	lenient bool
}

func NewAccountGetter(client client.Client, opts ...AccountGetterOption) *AccountGetter {
	ag := &AccountGetter{
		client: client,
	}

	for _, opt := range opts {
		opt(ag)
	}

	return ag
}

type AccountGetterOption func(*AccountGetter)

// Does not enforce that the account is ready.
func WithLenient() AccountGetterOption {
	return func(ag *AccountGetter) {
		ag.lenient = true
	}
}

// Gets the referenced Account
// Requires the account to be reconciled and ready unless client is `lenient`
func (a *AccountGetter) Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error) {
	log := logf.FromContext(ctx)
	key := client.ObjectKey{Namespace: namespace, Name: accountRefName}

	account = &v1alpha1.Account{}
	err = a.client.Get(ctx, key, account)
	if err != nil {
		return nil, err
	}

	if !isReady(account) && !a.lenient {
		log.Error(err, "account is not ready", "namespace", namespace, "accountName", accountRefName)
		return nil, errors.Join(errs.ErrAccountNotReady, err)
	}

	return account, err
}

func isReady(account *v1alpha1.Account) bool {
	_, ok := account.GetLabels()[domain.LabelAccountID]
	return ok
}
