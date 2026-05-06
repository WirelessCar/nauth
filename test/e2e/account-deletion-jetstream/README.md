# account-deletion-jetstream

This KUTTL suite verifies that deleting `example-account` is blocked while the account still owns a JetStream stream, and then completes after the stream is removed.

It creates:

- `example-account` in the test namespace
- a JetStream stream named `ORDERS` using temporary credentials for `example-account`

It then:

- requests deletion of `example-account`
- removes the `ORDERS` stream

The assertions verify that:

- `example-account` reconciles successfully before deletion
- the delete request puts `example-account` into a terminating state with its finalizer still present
- `example-account` reports a failed ready condition that mentions the blocking JetStream stream
- once `ORDERS` is deleted, `example-account` no longer exists
- the generated account root and signing secrets are cleaned up after deletion completes
