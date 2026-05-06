# account-export

This KUTTL suite verifies that `my-account-export` is adopted by `my-account` and that updates to `my-account-export` are reflected in `my-account`.

It creates:

- `my-account` in the test namespace
- `my-account-export` for `my-account`

It then updates:

- `my-account-export`

The assertions verify that:

- `my-account` reconciles successfully
- `my-account-export` resolves and stores the account ID for `my-account`
- `my-account-export` reaches `Ready=True`
- `my-account` adopts `my-account-export`
- the generated `desiredClaim` for `my-account-export` contains the expected export rule
- after the update, `my-account` renders the updated export claim
