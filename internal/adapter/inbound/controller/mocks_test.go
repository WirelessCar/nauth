package controller

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/stretchr/testify/mock"
)

/* ****************************************************
* outbound.AccountReader Mock
*****************************************************/

// TODO: [#228] Remove accountReaderMock
type accountReaderMock struct {
	mock.Mock
}

func (a *accountReaderMock) Get(ctx context.Context, accountRef domain.NamespacedName) (account *v1alpha1.Account, err error) {
	args := a.Called(ctx, accountRef)
	return args.Get(0).(*v1alpha1.Account), args.Error(1)
}

func (a *accountReaderMock) mockGet(ctx context.Context, accountRef domain.NamespacedName, result *v1alpha1.Account) *mock.Call {
	call := a.On("Get", ctx, accountRef)
	call.Return(result, nil)
	return call
}

var _ outbound.AccountReader = &accountReaderMock{}
