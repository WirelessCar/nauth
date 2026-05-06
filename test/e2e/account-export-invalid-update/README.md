# account-export-invalid-update

This KUTTL suite verifies that an invalid update to `my-account-export` does not replace the last valid export claim already adopted by `my-account`.

It creates:

- `my-account` in the test namespace
- `my-account-export` for `my-account`

It then updates:

- `my-account-export` with an invalid rule

The assertions verify that:

- `my-account` and the initial `my-account-export` reconcile successfully
- `my-account` adopts the initial export and renders the initial export claim
- after the invalid update, `my-account-export` no longer has valid rules for the new generation
- the last valid `desiredClaim` remains present on `my-account-export`
- `my-account` keeps the previously adopted export claim instead of switching to the invalid update
