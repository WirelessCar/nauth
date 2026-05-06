# account-import-missing-account

This KUTTL suite verifies that `my-account-import` stays not ready while `import-account` is missing, and then converges after `import-account` is created.

It creates:

- `export-account` in the test namespace
- `my-account-import` in the test namespace, pointing at `export-account`
- `import-account` later in the same namespace

The assertions verify that:

- `my-account-import` resolves `export-account` immediately
- `my-account-import` stays not ready while `import-account` is missing
- `my-account-import` has no `desiredClaim` while `import-account` is missing
- once `import-account` is created, `my-account-import` resolves the import account ID and reaches `Ready=True`
- `import-account` adopts the import and renders the expected import claim
