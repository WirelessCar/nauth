# account-update

This KUTTL suite verifies that updating `example-account` changes its claims without changing its account ID.

It creates:

- `example-account` in the test namespace

It then updates:

- the display name and NATS limits on `example-account`

The assertions verify that:

- `example-account` reconciles successfully before and after the update
- the original account ID is recorded before the update
- the updated claims are applied after the update
- the account ID on `example-account` stays the same across the update
