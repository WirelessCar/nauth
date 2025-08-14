# v1alpha1

This describes the intention of the `v1alpha1` version of the Nauth CRDs.
The version is intended to be used by the initial release which will be deployed in a limited scope, but where updates can still be done in a backwards compatible way.

## Account


```yaml
apiVersion: nauth.io/v1alpha1
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
    diskMaxStreamBytes: 2097
    diskStorageBytes: 1073 # or check if we can use the Mi, Gi pattern
    maxAckPending: 5
    maxBytesRequired: true
    memMaxStreamBytes: 209
    memStorageBytes: 1073
    streams: 5
  natsLimits:
    data: 1024
    payload: 500
    subs: 100
  exports:
    - subject: foo.>
      type: "stream" # or 'service'
  imports:
    - subject: foo.>
      alias: bar.>
      type: "stream" # or 'service'
      importAccount:
        name: account-a
        namespace: system-a
status:
  conditions: []
  # The claims of the jwt, including the imports and exports which are configured in the AccountExport & AccountImport manifests
  claims:
    limits: ...
  # Status to keep track of when keys should be rotated. Example inspired by cert-manager.
  signingKey:
    id: ABRZY7SDP2BBZ3UODLPFEKLGFAKY3SECIW7GQFVRAVGSUCBUMRAELHKP
    creationDate: "2025-05-06T11:14:21Z"
```

## User

```yaml
apiVersion: nauth.io/v1alpha1
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
