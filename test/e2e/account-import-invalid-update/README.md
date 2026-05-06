# account-import-invalid-update

This KUTTL suite verifies that an invalid update to `my-account-import` does not replace the last valid import claim already adopted by `import-account`.

It creates:

- `import-account` in the test namespace
- `export-account` in the test namespace
- `my-account-import` in the test namespace, pointing at `export-account`

It then updates:

- `my-account-import` with invalid import rules

The assertions verify that:

- the initial `my-account-import` reconciles successfully
- `import-account` adopts the initial import and renders the initial import claim
- after the invalid update, the new generation does not become valid or ready
- the last valid `desiredClaim` remains present on `my-account-import`
- `import-account` keeps the previously adopted import claim instead of switching to the invalid update
