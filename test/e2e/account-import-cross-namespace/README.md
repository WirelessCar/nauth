# account-import-cross-namespace

This KUTTL suite verifies that an `AccountImport` can bind to an export account in a different Kubernetes namespace.

It creates:

- a dedicated export namespace
- `import-account` in the test namespace
- `export-account` in the export namespace
- `my-account-import` in the test namespace, pointing at the foreign export account

The assertions verify that:

- both accounts reconcile successfully
- `my-account-import` resolves and stores both account IDs
- `my-account-import` reaches `Ready=True`
- the generated `desiredClaim` for `my-account-import` points at the foreign export account ID
- `import-account` adopts the import and renders the expected import claim

The final step deletes the extra export namespace explicitly, since it is outside the auto-generated test namespace that KUTTL cleans up automatically.
