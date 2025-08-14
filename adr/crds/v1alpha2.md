# v1alpha2

**Status: DRAFT**

This describes the intention of the `v1alpha2` version of the Nauth CRDs.
It includes updates such as:

- AccountExports & AccountImports separated into CRDs
- Restricting exports to specific accounts
- Automatic signing key rotation for accounts

## Account

The account claims is split into multiple CRD:s such as `AccountExport` & `AccountImport`.
The reasoning behind this is that it quickly becomes cumbersome to define all exports in the same CRD when an account is fanning out messages across a lot of systems.
By leveraging labels & label selectors, fetching all exports and imports is essentially as easy as getting a list within the `Account` CR. [^1]

```yaml
apiVersion: nauth.io/v1alpha2
kind: Account
metadata:
  labels:
    # Labeled when reconciled and account set. Used when resource is created again without wanting to change the account id.
    account.nauth.io/id: AC4N5Q7SATQRF7BM3ODZ4T673CBWB2FMT344FHHAYK46ZKXNENWNWUM3
    # In order to identify accounts signed with certain signing key in the event of rotation
    account.nauth.io/signed-by: OBWDX2NWMMH7MEUQYORBXZ2TC6X3VSIY5JWXPFEGI2DOD6SKFPXL6UFA
  name: default-account
  namespace: system-a-dev
spec:
  accountLimits:
    conn: 100
    exports: 10
    imports: 10
    wildcards: true
  jetStreamLimits:
    consumer: 5
    # Use the default resource config as strings same as Kubernetes resources
    diskMaxStream: '100MiB' 
    diskStorage: '100MiB'
    maxAckPending: 5
    maxBytesRequired: true
    memMaxStream: '100MiB'
    memStorage: '100MiB'
    streams: 5
  natsLimits:
    data: 1024
    payload: 500
    subs: 100
  # START For backwards compatibility
  exports:
    - subject: foo.>
      type: "stream" # or 'service'
      # The old variant will be the same as "allow all"
  imports:
    - subject: foo.>
      alias: bar.>
      type: "stream" # or 'service'
      importAccount:
        name: account-a
        namespace: system-a
  # END
  signingKeyExpirationDays: 30 # Allow for setting the expiry if needed, otherwise use the settings of nauth
status:
  conditions: []
  # The claims of the JWT, including the imports and exports which are configured in the AccountExport & AccountImport manifests
  claims:
    limits: ...
  # Status to keep track of when keys should be rotated. Example inspired by cert-manager.
  signingKeys:
    primary:
      id: ABRZY7SDP2BBZ3UODLPFEKLGFAKY3SECIW7GQFVRAVGSUCBUMRAELHKP
      creationDate: "2025-05-06T11:14:21Z"
      expirationDate: "2025-08-06T11:14:21Z"
      renewAfter: "2025-07-06T11:14:21Z"
    secondary: 
      id: AA2MPUT7KDXVDONLTTRIG6RT6NUKBVS6UIB74T2WMURLL743CZBWRGSV
      creationDate: "2025-04-06T11:14:21Z"
      expirationDate: "2025-07-06T11:14:21Z"
      renewAfter: "2025-06-06T11:14:21Z"
```

### AccountExport

Restricting the exports of an account is handled by nauth and does not leverage the NATS private export/import functionality.
This might be added in future releases if it would add security to the solution. Only nauth is able to push new JWTs to NATS, making it the only gate for signing imports.

```yaml
apiVersion: nauth.io/v1alpha2
kind: AccountExport
metadata:
  labels:
    # Set when the account has been linked - allows for fast queries when account is reconciled
    account-export.nauth.io/account-id: AC4N5Q7SATQRF7BM3ODZ4T673CBWB2FMT344FHHAYK46ZKXNENWNWUM3 
  name: export-to-system-b
  namespace: system-a
spec:
  accountRef: account-a
  subject: foo.>
  type: 1
  allowedAccounts:
    - name: account-b
      namespace: system-b
status:
  conditions: []
```

### AccountImport
```yaml
apiVersion: nauth.io/v1alpha2
kind: AccountImport
metadata:
  labels:
    # Set when the account has been linked - allows for fast queries when account is reconciled
    account-import.nauth.io/account-id: AC4N5Q7SATQRF7BM3ODZ4T673CBWB2FMT344FHHAYK46ZKXNENWNWUM3 
  name: import-from-system-a
  namespace: system-b
spec:
  accountRef: account-b
  subject: foo.>
  alias: bar.>
  type: 1
  importAccount:
    name: account-a
    namespace: system-a
status:
  conditions: []
```


## User

```yaml
apiVersion: nauth.io/v1alpha2
kind: User
metadata:
  labels:
    user.nauth.io/id: UCKKOB6SGBSRLS2BHTY7KS7GDKTM7XC7LND2UBJ65BQ553NJZWA4CEWS
    # Labels added during creation. Want to be able to easily query for both account & signing key during account reconciliation.
    user.nauth.io/account-id: ABRZY7SDP2BBZ3UODLPFEKLGFAKY3SECIW7GQFVRAVGSUCBUMRAELHKP
    user.nauth.io/signed-by: AC4N5Q7SATQRF7BM3ODZ4T673CBWB2FMT344FHHAYK46ZKXNENWNWUM3
  name: nats-test-user
  namespace: system-a-dev
spec:
  accountName: default-account
  natsLimits: {}
  permissions:
    pub: {}
    sub: {}
  userLimits: {}
status:
  claims: {} 
  conditions: []
```


[^1]: Example go snippet for fetching other resources using label selector ☝️

### Example snippet using label selector
```go
labelSelector := "foo=bar"

opts := metav1.ListOptions{
		LabelSelector: labelSelector,
	})

result := &accountexport.AccountExportList{}
	err := c.restClient.
		Get().
		Namespace("<same namespace as account>").
		Resource("accountexports").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(result)

for i, item := range result.Items {
...
}

