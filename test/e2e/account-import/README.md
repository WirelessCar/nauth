# account-import

This KUTTL suite verifies that `my-account-import` is adopted by `import-account` and renders a valid stream import from `export-account`.

It creates:

- `import-account` in the test namespace
- `export-account` in the test namespace
- `my-account-import` in the test namespace, pointing at `export-account`

The assertions verify that:

- `import-account` and `export-account` reconcile successfully
- `my-account-import` resolves and stores both account IDs
- `my-account-import` reaches `Ready=True`
- the generated `desiredClaim` for `my-account-import` contains the expected stream import rule
- `import-account` adopts the import and renders the expected import claim
