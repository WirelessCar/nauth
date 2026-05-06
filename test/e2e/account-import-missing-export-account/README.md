# account-import-missing-export-account

This KUTTL suite verifies that `my-account-import` does not become ready while `export-account` is missing, and then converges automatically after `export-account` exists.

It creates:

- `import-account` in the test namespace
- `my-account-import` in the test namespace, pointing at `export-account`
- `export-account` later in the same namespace

The assertions verify that:

- `import-account` reconciles successfully
- `my-account-import` binds to `import-account` immediately
- `my-account-import` stays not ready while `export-account` is missing
- `my-account-import` has no resolved export account ID or `desiredClaim` while `export-account` is missing
- `export-account` reconciles successfully once created
- `my-account-import` resolves `export-account`, reaches `Ready=True`, and renders the expected `desiredClaim`
- `import-account` adopts the import and renders the expected import claim
