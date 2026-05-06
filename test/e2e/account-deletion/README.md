# account-deletion

This KUTTL suite verifies that deleting `example-account` removes the account and its generated secrets.

It creates:

- `example-account` in the test namespace
- generated account root and signing secrets for `example-account`

It then deletes:

- `example-account`

The assertions verify that:

- `example-account` reconciles successfully before deletion
- the generated account root and signing secrets exist before deletion
- `example-account` no longer exists after deletion
- the generated account root and signing secrets no longer exist after deletion
