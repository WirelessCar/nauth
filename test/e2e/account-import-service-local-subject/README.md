# account-import-service-local-subject

This KUTTL suite verifies that `my-account-import` supports a service import rule with `localSubject`.

It creates:

- `import-account` in the test namespace
- `export-account` in the test namespace
- `my-account-import` in the test namespace, pointing at `export-account`

The import rule uses:

- `subject: svc.request`
- `localSubject: local.svc.request`
- `type: service`

The assertions verify that:

- `import-account` and `export-account` reconcile successfully
- `my-account-import` resolves and stores both account IDs
- `my-account-import` reaches `Ready=True`
- the generated `desiredClaim` for `my-account-import` preserves `subject`, `localSubject`, `type`, and the resolved export account ID
- `import-account` adopts the import and renders the expected service import claim
