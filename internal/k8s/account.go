package k8s

import (
	"context"
	"errors"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountClient struct {
	client  client.Client
	lenient bool
}

func NewAccountClient(client client.Client) *AccountClient {
	ag := &AccountClient{
		client: client,
	}

	return ag
}

// Gets the referenced Account
// Requires the account to be reconciled and ready unless client is `lenient`
func (a *AccountClient) Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error) {
	log := logf.FromContext(ctx)
	key := client.ObjectKey{Namespace: namespace, Name: accountRefName}

	account = &v1alpha1.Account{}
	err = a.client.Get(ctx, key, account)
	if err != nil {
		return nil, err
	}

	if !isReady(account) && !a.lenient {
		log.Error(err, "account is not ready", "namespace", namespace, "accountName", accountRefName)
		return nil, errors.Join(ErrAccountNotReady, err)
	}

	return account, err
}

func isReady(account *v1alpha1.Account) bool {
	_, ok := account.GetLabels()[LabelAccountID]
	return ok
}
