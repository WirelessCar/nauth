# account-export-conflict

This KUTTL suite verifies that conflicting exports for `my-account` do not both become active at the same time.

It creates:

- `my-account` in the test namespace
- `my-account-export-a` for `my-account`
- `my-account-export-b` for `my-account`

The export rules intentionally overlap:

- `my-account-export-a` exports `conflict.>`
- `my-account-export-b` exports `conflict.*`

The assertions verify that:

- `my-account` reconciles successfully
- both exports resolve and store the account ID for `my-account`
- both exports are bound and valid
- `my-account` records adoptions for both exports
- exactly one export is adopted successfully
- the other export reports a conflict and is not active
- `my-account` renders only one export claim
