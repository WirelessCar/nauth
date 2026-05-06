# account-export-missing-account

This KUTTL suite verifies that `my-account-export` stays not ready while `my-account` is missing, and then converges after `my-account` is created.

It creates:

- `my-account-export` in the test namespace
- `my-account` later in the same namespace

The assertions verify that:

- `my-account-export` has valid rules even before `my-account` exists
- `my-account-export` stays not ready while `my-account` is missing
- `my-account-export` has no bound account ID while `my-account` is missing
- once `my-account` is created, `my-account-export` resolves the account ID and reaches `Ready=True`
- `my-account` adopts the export and renders the expected export claim
