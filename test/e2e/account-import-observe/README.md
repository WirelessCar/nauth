# account-import-observe

This KUTTL suite verifies that an observed account can be imported from an externally provided account JWT and reconciled through `nauth.io/management-policy: observe`.

It creates:

- account root and signing secrets for the fixed account ID `ADDS6I3G7LBIBNDMZ5Q32VUJN2XNSW2QK4HY2SY5ZA7I2JLBFYF4KJDO`
- an uploaded account JWT for the same account ID
- `example-account` in observe mode, bound to that account ID

The assertions verify that:

- `example-account` reconciles successfully in observe mode
- `example-account` keeps the expected account ID and signed-by labels
- `example-account` imports the claims from the uploaded JWT
- the observed claims include the expected limits and export rule from the external account definition
