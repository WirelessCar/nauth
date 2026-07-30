---
title: API Reference
description: API reference for nauth CRDs
---


## Packages
- [nauth.io/v1alpha1](#nauthiov1alpha1)


## nauth.io/v1alpha1

Package v1alpha1 contains API schema definitions for the nauth.io v1alpha1 API group.

### Resource Types
- [Account](#account)
- [AccountExport](#accountexport)
- [AccountExportList](#accountexportlist)
- [AccountImport](#accountimport)
- [AccountImportList](#accountimportlist)
- [AccountList](#accountlist)
- [AccountSigningKey](#accountsigningkey)
- [AccountSigningKeyList](#accountsigningkeylist)
- [NatsCluster](#natscluster)
- [NatsClusterList](#natsclusterlist)
- [User](#user)
- [UserList](#userlist)



#### Account



Account is the composite resource for the accounts API.



_Appears in:_
- [AccountList](#accountlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `Account` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AccountSpec](#accountspec)_ |  |  |  |
| `status` _[AccountStatus](#accountstatus)_ |  |  |  |


#### AccountAdoption







_Appears in:_
- [AccountAdoptions](#accountadoptions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name the child resource name |  | MinLength: 1 <br />Required <br /> |
| `uid` _[UID](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#uid-types-pkg)_ | UID of the child resource UID |  | Required <br /> |
| `observedGeneration` _integer_ | ObservedGeneration refers to the observed generation of the child resource. |  | Minimum: 0 <br />Required <br /> |
| `status` _[AccountAdoptionStatus](#accountadoptionstatus)_ | Status of the adoption |  | Required <br /> |


#### AccountAdoptionStatus







_Appears in:_
- [AccountAdoption](#accountadoption)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `status` _[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#conditionstatus-v1-meta)_ | Status of the adoption, one of True, False, Unknown. |  | Enum: [True False Unknown] <br />Required <br /> |
| `desiredClaimObservedGeneration` _integer_ | DesiredClaimObservedGeneration refers to the observed generation of the child resource desired claim. |  | Minimum: 0 <br /> |
| `reason` _string_ | Reason contains a programmatic identifier indicating the reason for the adoption's last transition.<br />The value should be a CamelCase string.<br />This field may not be empty. |  | MaxLength: 1024 <br />MinLength: 1 <br />Pattern: `^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$` <br />Required <br /> |
| `message` _string_ | Message is a human-readable message indicating details about the adoption. |  | MaxLength: 32768 <br /> |


#### AccountAdoptions



AccountAdoptions defines the status of child resources that have been adopted or are candidates for adoption by this account.



_Appears in:_
- [AccountStatus](#accountstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `exports` _[AccountAdoption](#accountadoption) array_ | Exports defines adoptions of type `AccountExport` that are bound to the account. |  |  |
| `imports` _[AccountAdoption](#accountadoption) array_ | Imports defines adoptions of type `AccountImport` that are bound to the account. |  |  |


#### AccountClaims







_Appears in:_
- [AccountStatus](#accountstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountLimits` _[AccountLimits](#accountlimits)_ |  |  |  |
| `displayName` _string_ |  |  |  |
| `signingKeys` _[SigningKeys](#signingkeys)_ |  |  |  |
| `exports` _[Exports](#exports)_ |  |  |  |
| `imports` _[Imports](#imports)_ |  |  |  |
| `jetStreamEnabled` _boolean_ |  |  |  |
| `jetStreamLimits` _[JetStreamLimits](#jetstreamlimits)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |


#### AccountExport



AccountExport is a component resource for exports in the accounts API.



_Appears in:_
- [AccountExportList](#accountexportlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountExport` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AccountExportSpec](#accountexportspec)_ |  |  |  |
| `status` _[AccountExportStatus](#accountexportstatus)_ |  |  |  |


#### AccountExportClaim







_Appears in:_
- [AccountExportStatus](#accountexportstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `rules` _[AccountExportRule](#accountexportrule) array_ | Rules contains export rules that have been validated and are ready to be used by Account |  | MinItems: 1 <br />Required <br /> |
| `observedGeneration` _integer_ |  |  | Required <br /> |




#### AccountExportList



AccountExportList contains a list of AccountExport.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountExportList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AccountExport](#accountexport) array_ |  |  |  |


#### AccountExportRule







_Appears in:_
- [AccountExportClaim](#accountexportclaim)
- [AccountExportSpec](#accountexportspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ |  |  | Required <br /> |
| `type` _[ExportType](#exporttype)_ |  |  | Enum: [stream service] <br />Required <br /> |
| `responseType` _[ResponseType](#responsetype)_ |  |  | Enum: [Singleton Stream Chunked] <br /> |
| `responseThreshold` _[Duration](#duration)_ |  |  |  |
| `serviceLatency` _[ServiceLatency](#servicelatency)_ |  |  |  |
| `accountTokenPosition` _integer_ |  |  |  |
| `advertise` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |


#### AccountExportSpec



AccountExportSpec defines the desired state of AccountExport.



_Appears in:_
- [AccountExport](#accountexport)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountName` _string_ | AccountName refers to the Account in the same namespace to which this export applies. |  | Required <br /> |
| `rules` _[AccountExportRule](#accountexportrule) array_ | Rules defines the export rules for this account export. Must have at least one rule. |  | MinItems: 1 <br />Required <br /> |


#### AccountExportStatus



AccountExportStatus defines the observed state of AccountExport.



_Appears in:_
- [AccountExport](#accountexport)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountID` _string_ | AccountID is the ID of the account that this export is bound to. |  |  |
| `desiredClaim` _[AccountExportClaim](#accountexportclaim)_ | Normalized claim for account to use |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ |  |  |  |
| `operatorVersion` _string_ |  |  |  |


#### AccountImport



AccountImport is a component resource for imports in the accounts API.



_Appears in:_
- [AccountImportList](#accountimportlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountImport` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AccountImportSpec](#accountimportspec)_ |  |  |  |
| `status` _[AccountImportStatus](#accountimportstatus)_ |  |  |  |


#### AccountImportClaim







_Appears in:_
- [AccountImportStatus](#accountimportstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `rules` _[AccountImportRuleDerived](#accountimportrulederived) array_ | Rules contains import rules that have been validated and are ready to be used by Account. |  | MinItems: 1 <br />Required <br /> |
| `observedGeneration` _integer_ |  |  | Required <br /> |




#### AccountImportList



AccountImportList contains a list of AccountImport.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountImportList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AccountImport](#accountimport) array_ |  |  |  |


#### AccountImportRule







_Appears in:_
- [AccountImportRuleDerived](#accountimportrulederived)
- [AccountImportSpec](#accountimportspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ | Subject is the exported subject to import.<br />It must be identical to or a subset of the exported subject. |  | Required <br /> |
| `localSubject` _[RenamingSubject](#renamingsubject)_ | LocalSubject remaps the imported subject locally in the importing account. |  |  |
| `type` _[ExportType](#exporttype)_ | Type defines whether the import is a stream or service import. |  | Enum: [stream service] <br />Required <br /> |
| `share` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |


#### AccountImportRuleDerived







_Appears in:_
- [AccountImportClaim](#accountimportclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ | Subject is the exported subject to import.<br />It must be identical to or a subset of the exported subject. |  | Required <br /> |
| `localSubject` _[RenamingSubject](#renamingsubject)_ | LocalSubject remaps the imported subject locally in the importing account. |  |  |
| `type` _[ExportType](#exporttype)_ | Type defines whether the import is a stream or service import. |  | Enum: [stream service] <br />Required <br /> |
| `share` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |
| `account` _string_ | Account is the resolved export account ID used for this import rule. |  | Required <br /> |


#### AccountImportSpec



AccountImportSpec defines the desired state of AccountImport.



_Appears in:_
- [AccountImport](#accountimport)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountName` _string_ | AccountName refers to the Account in the same namespace to which this import applies. |  | Required <br /> |
| `exportAccountRef` _[AccountRef](#accountref)_ | ExportAccountRef refers to the Account from which the exports are imported.<br />This reference may point to an Account in another namespace. |  | Required <br /> |
| `rules` _[AccountImportRule](#accountimportrule) array_ | Rules defines the import rules for this AccountImport. |  | MinItems: 1 <br />Required <br /> |


#### AccountImportStatus



AccountImportStatus defines the observed state of AccountImport.



_Appears in:_
- [AccountImport](#accountimport)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountID` _string_ | AccountID is the resolved ID of the Account referenced by spec.accountName. |  |  |
| `exportAccountID` _string_ | ExportAccountID is the resolved ID of the Account referenced by spec.exportAccountRef. |  |  |
| `desiredClaim` _[AccountImportClaim](#accountimportclaim)_ | DesiredClaim is the normalized claim for Account to use. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ |  |  |  |
| `operatorVersion` _string_ |  |  |  |




#### AccountLimits







_Appears in:_
- [AccountClaims](#accountclaims)
- [AccountSpec](#accountspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imports` _integer_ |  | -1 |  |
| `exports` _integer_ |  | -1 |  |
| `wildcards` _boolean_ |  | true |  |
| `conn` _integer_ |  | -1 |  |
| `leaf` _integer_ |  | -1 |  |


#### AccountList



AccountList contains a list of Account.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[Account](#account) array_ |  |  |  |


#### AccountRef







_Appears in:_
- [AccountImportSpec](#accountimportspec)
- [Import](#import)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `namespace` _string_ |  |  |  |


#### AccountSigningKey



AccountSigningKey manages one NATS account signing-key seed in a Kubernetes Secret.
By default NAuth manages the signing key seed: it generates a new key and stores it in
a Secret named spec.secretName (defaulting to <resourceName>-ac-sign). The Secret is
owned by this resource and garbage-collected when the resource is deleted.

In observe mode (label nauth.io/management-policy=observe), NAuth only reads an existing
Secret with the resolved name and derives the public key. Observed Secrets are not
modified, owned, or deleted by the operator.

An Account trusts the public key by listing this resource in Account.spec.signingKeyRefs.



_Appears in:_
- [AccountSigningKeyList](#accountsigningkeylist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountSigningKey` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AccountSigningKeySpec](#accountsigningkeyspec)_ |  |  |  |
| `status` _[AccountSigningKeyStatus](#accountsigningkeystatus)_ |  |  |  |


#### AccountSigningKeyList



AccountSigningKeyList contains a list of AccountSigningKey.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `AccountSigningKeyList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[AccountSigningKey](#accountsigningkey) array_ |  |  |  |


#### AccountSigningKeyRef



AccountSigningKeyRef references a signing-key resource whose public key is
trusted by the Account (and may be used to sign Users). Only AccountSigningKey
is supported; Kind is reserved for future kinds (e.g. issuers).



_Appears in:_
- [AccountSpec](#accountspec)
- [UserSpec](#userspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _[AccountSigningKeyRefKind](#accountsigningkeyrefkind)_ | Kind of the referenced resource. Defaults to AccountSigningKey. | AccountSigningKey | Enum: [AccountSigningKey] <br /> |
| `name` _string_ | Name of the referenced resource. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Required <br /> |
| `namespace` _string_ | Namespace of the referenced resource. When empty, defaults to the<br />referrer's namespace. Cross-namespace references let multiple Accounts<br />trust a shared signing key (e.g. a cluster-wide Auth Callout service). |  | MaxLength: 253 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?)?$` <br /> |


#### AccountSigningKeyRefKind

_Underlying type:_ _string_

AccountSigningKeyRefKind is the kind of resource referenced as an account
signing key.

_Validation:_
- Enum: [AccountSigningKey]

_Appears in:_
- [AccountSigningKeyRef](#accountsigningkeyref)

| Field | Description |
| --- | --- |
| `AccountSigningKey` |  |


#### AccountSigningKeySpec



AccountSigningKeySpec defines the desired state of AccountSigningKey.



_Appears in:_
- [AccountSigningKey](#accountsigningkey)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secretName` _string_ | SecretName names the Kubernetes Secret that holds the account signing-key seed.<br />In managed mode (default), SecretName is optional and defaults to<br /><resourceName>-ac-sign; the Secret is created and owned by this AccountSigningKey.<br />In observe mode (label nauth.io/management-policy=observe), SecretName is<br />required and identifies the existing Secret to read; the operator never falls<br />back to the managed default name and never modifies the Secret.<br />Immutable. |  | MaxLength: 253 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br /> |


#### AccountSigningKeyStatus



AccountSigningKeyStatus defines the observed state of AccountSigningKey.



_Appears in:_
- [AccountSigningKey](#accountsigningkey)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `publicKey` _string_ | PublicKey is the resolved NATS public key (A-prefixed nkey) for this signing key. |  |  |
| `secretName` _string_ | SecretName is the resolved name of the Secret holding the seed. |  |  |
| `managementPolicy` _string_ | ManagementPolicy reflects the effective management policy for this resource.<br />Empty means managed (default); "observe" means the Secret is only read. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ |  |  |  |
| `operatorVersion` _string_ |  |  |  |


#### AccountSpec



AccountSpec defines the desired state of Account.



_Appears in:_
- [Account](#account)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `natsClusterRef` _[NatsClusterRef](#natsclusterref)_ | NatsClusterRef references the NatsCluster to use for this account.<br />If not specified, the controller uses the operator-level NATS_CLUSTER_REF when configured.<br />Otherwise, reconciliation fails because the target NatsCluster cannot be resolved. |  |  |
| `displayName` _string_ | DisplayName is an optional name for the NATS resource representing the account. May be derived if absent. |  |  |
| `jetStreamEnabled` _boolean_ | JetStreamEnabled indicates whether JetStream should be explicitly enabled or disabled.<br />If absent, JetStream will be implicitly enabled/disabled based on the effective JetStreamLimits. |  |  |
| `accountLimits` _[AccountLimits](#accountlimits)_ |  |  |  |
| `exports` _[Exports](#exports)_ |  |  |  |
| `imports` _[Imports](#imports)_ |  |  |  |
| `jetStreamLimits` _[JetStreamLimits](#jetstreamlimits)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |
| `signingKeyRefs` _[AccountSigningKeyRef](#accountsigningkeyref) array_ | SigningKeyRefs lists references whose public keys are trusted as additional<br />signing keys for this account. The implicit default signing key is always<br />present and is not listed here. |  | MaxItems: 64 <br /> |


#### AccountStatus



AccountStatus defines the observed state of Account.



_Appears in:_
- [Account](#account)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `claims` _[AccountClaims](#accountclaims)_ |  |  |  |
| `claimsHash` _string_ | ClaimsHash is a hash of the Account JWT claims, used to determine if the claims have changed and a new JWT needs to be generated. |  |  |
| `adoptions` _[AccountAdoptions](#accountadoptions)_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ |  |  |  |
| `operatorVersion` _string_ |  |  |  |


#### CIDRList

_Underlying type:_ _[TagList](#taglist)_





_Appears in:_
- [UserLimits](#userlimits)



#### Export







_Appears in:_
- [Exports](#exports)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ |  |  |  |
| `type` _[ExportType](#exporttype)_ |  |  | Enum: [stream service] <br /> |
| `tokenReq` _boolean_ |  |  |  |
| `revocations` _[RevocationList](#revocationlist)_ |  |  |  |
| `responseType` _[ResponseType](#responsetype)_ |  |  | Enum: [Singleton Stream Chunked] <br /> |
| `responseThreshold` _[Duration](#duration)_ |  |  |  |
| `serviceLatency` _[ServiceLatency](#servicelatency)_ |  |  |  |
| `accountTokenPosition` _integer_ |  |  |  |
| `advertise` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |


#### ExportType

_Underlying type:_ _string_

ExportType defines the type of import/export.

_Validation:_
- Enum: [stream service]

_Appears in:_
- [AccountExportRule](#accountexportrule)
- [AccountImportRule](#accountimportrule)
- [AccountImportRuleDerived](#accountimportrulederived)
- [Export](#export)
- [Import](#import)

| Field | Description |
| --- | --- |
| `stream` | Stream defines the type field value for a stream "stream"<br /> |
| `service` | Service defines the type field value for a service "service"<br /> |


#### Exports

_Underlying type:_ _[Export](#export)_





_Appears in:_
- [AccountClaims](#accountclaims)
- [AccountSpec](#accountspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ |  |  |  |
| `type` _[ExportType](#exporttype)_ |  |  | Enum: [stream service] <br /> |
| `tokenReq` _boolean_ |  |  |  |
| `revocations` _[RevocationList](#revocationlist)_ |  |  |  |
| `responseType` _[ResponseType](#responsetype)_ |  |  | Enum: [Singleton Stream Chunked] <br /> |
| `responseThreshold` _[Duration](#duration)_ |  |  |  |
| `serviceLatency` _[ServiceLatency](#servicelatency)_ |  |  |  |
| `accountTokenPosition` _integer_ |  |  |  |
| `advertise` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |


#### Import







_Appears in:_
- [Imports](#imports)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountRef` _[AccountRef](#accountref)_ | AccountRefName references the account used to create the user. |  |  |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ | Subject field in an import is always from the perspective of the<br />initial publisher - in the case of a stream it is the account owning<br />the stream (the exporter), and in the case of a service it is the<br />account making the request (the importer). |  |  |
| `account` _string_ |  |  |  |
| `localSubject` _[RenamingSubject](#renamingsubject)_ | Local subject used to subscribe (for streams) and publish (for services) to.<br />This value only needs setting if you want to change the value of Subject.<br />If the value of Subject ends in > then LocalSubject needs to end in > as well.<br />LocalSubject can contain $<number> wildcard references where number references the nth wildcard in Subject.<br />The sum of wildcard reference and * tokens needs to match the number of * token in Subject. |  |  |
| `type` _[ExportType](#exporttype)_ |  |  | Enum: [stream service] <br /> |
| `share` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |


#### Imports

_Underlying type:_ _[Import](#import)_





_Appears in:_
- [AccountClaims](#accountclaims)
- [AccountSpec](#accountspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountRef` _[AccountRef](#accountref)_ | AccountRefName references the account used to create the user. |  |  |
| `name` _string_ |  |  |  |
| `subject` _[Subject](#subject)_ | Subject field in an import is always from the perspective of the<br />initial publisher - in the case of a stream it is the account owning<br />the stream (the exporter), and in the case of a service it is the<br />account making the request (the importer). |  |  |
| `account` _string_ |  |  |  |
| `localSubject` _[RenamingSubject](#renamingsubject)_ | Local subject used to subscribe (for streams) and publish (for services) to.<br />This value only needs setting if you want to change the value of Subject.<br />If the value of Subject ends in > then LocalSubject needs to end in > as well.<br />LocalSubject can contain $<number> wildcard references where number references the nth wildcard in Subject.<br />The sum of wildcard reference and * tokens needs to match the number of * token in Subject. |  |  |
| `type` _[ExportType](#exporttype)_ |  |  | Enum: [stream service] <br /> |
| `share` _boolean_ |  |  |  |
| `allowTrace` _boolean_ |  |  |  |




#### JetStreamLimits







_Appears in:_
- [AccountClaims](#accountclaims)
- [AccountSpec](#accountspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `memStorage` _integer_ |  | -1 |  |
| `diskStorage` _integer_ |  | -1 |  |
| `streams` _integer_ |  | -1 |  |
| `consumer` _integer_ |  | -1 |  |
| `maxAckPending` _integer_ |  | -1 |  |
| `memMaxStreamBytes` _integer_ |  | -1 |  |
| `diskMaxStreamBytes` _integer_ |  | -1 |  |
| `maxBytesRequired` _boolean_ |  | false |  |


#### NatsCluster



NatsCluster is the Schema for the natsclusters API



_Appears in:_
- [NatsClusterList](#natsclusterlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `NatsCluster` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[NatsClusterSpec](#natsclusterspec)_ |  |  |  |
| `status` _[NatsClusterStatus](#natsclusterstatus)_ |  |  |  |


#### NatsClusterList



NatsClusterList contains a list of NatsCluster





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `NatsClusterList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[NatsCluster](#natscluster) array_ |  |  |  |


#### NatsClusterRef



NatsClusterRef references a NatsCluster resource



_Appears in:_
- [AccountSpec](#accountspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the NatsCluster |  |  |
| `namespace` _string_ | Namespace of the NatsCluster |  |  |


#### NatsClusterSpec



NatsClusterSpec defines the desired state of NatsCluster



_Appears in:_
- [NatsCluster](#natscluster)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the NATS server URL for this cluster. Mutually exclusive with urlFrom. |  |  |
| `urlFrom` _[URLFromReference](#urlfromreference)_ | URLFrom loads the NATS URL from a ConfigMap or Secret. Mutually exclusive with url. |  |  |
| `operatorSigningKeySecretRef` _[SecretKeyReference](#secretkeyreference)_ |  |  |  |
| `systemAccountUserCredsSecretRef` _[SecretKeyReference](#secretkeyreference)_ |  |  |  |


#### NatsClusterStatus



NatsClusterStatus defines the observed state of NatsCluster.



_Appears in:_
- [NatsCluster](#natscluster)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ |  |  |  |
| `operatorVersion` _string_ |  |  |  |


#### NatsLimits







_Appears in:_
- [AccountClaims](#accountclaims)
- [AccountSpec](#accountspec)
- [UserClaims](#userclaims)
- [UserSpec](#userspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subs` _integer_ |  | -1 |  |
| `data` _integer_ |  | -1 |  |
| `payload` _integer_ |  | -1 |  |


#### Permission



Permission defines allow/deny subjects



_Appears in:_
- [Permissions](#permissions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allow` _[StringList](#stringlist)_ |  |  |  |
| `deny` _[StringList](#stringlist)_ |  |  |  |


#### Permissions



Permissions are used to restrict subject access, either on a user or for everyone on a server by default



_Appears in:_
- [UserClaims](#userclaims)
- [UserSpec](#userspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pub` _[Permission](#permission)_ |  |  |  |
| `sub` _[Permission](#permission)_ |  |  |  |
| `resp` _[ResponsePermission](#responsepermission)_ |  |  |  |


#### RenamingSubject

_Underlying type:_ _[Subject](#subject)_





_Appears in:_
- [AccountImportRule](#accountimportrule)
- [AccountImportRuleDerived](#accountimportrulederived)
- [Import](#import)



#### ResponsePermission



ResponsePermission can be used to allow responses to any reply subject
that is received on a valid subscription.



_Appears in:_
- [Permissions](#permissions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `max` _integer_ |  |  |  |
| `ttl` _[Duration](#duration)_ |  |  |  |


#### ResponseType

_Underlying type:_ _string_

ResponseType is used to store an export response type

_Validation:_
- Enum: [Singleton Stream Chunked]

_Appears in:_
- [AccountExportRule](#accountexportrule)
- [Export](#export)



#### RevocationList

_Underlying type:_ _object_





_Appears in:_
- [Export](#export)



#### SamplingRate

_Underlying type:_ _integer_





_Appears in:_
- [ServiceLatency](#servicelatency)



#### SecretKeyReference



SecretKeyReference contains information to locate a secret in the same namespace



_Appears in:_
- [NatsClusterSpec](#natsclusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret. |  | Required <br /> |
| `key` _string_ | Key in the Secret, when not specified an implementation-specific default key is used. |  |  |


#### ServiceLatency







_Appears in:_
- [AccountExportRule](#accountexportrule)
- [Export](#export)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sampling` _[SamplingRate](#samplingrate)_ |  |  |  |
| `results` _[Subject](#subject)_ |  |  |  |


#### SigningKey







_Appears in:_
- [SigningKeys](#signingkeys)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ |  |  |  |


#### SigningKeys

_Underlying type:_ _[SigningKey](#signingkey)_





_Appears in:_
- [AccountClaims](#accountclaims)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ |  |  |  |


#### StringList

_Underlying type:_ _string array_

StringList is a wrapper for an array of strings



_Appears in:_
- [Permission](#permission)



#### Subject

_Underlying type:_ _string_

Subject is a string that represents a NATS subject



_Appears in:_
- [AccountExportRule](#accountexportrule)
- [AccountImportRule](#accountimportrule)
- [AccountImportRuleDerived](#accountimportrulederived)
- [Export](#export)
- [Import](#import)
- [RenamingSubject](#renamingsubject)
- [ServiceLatency](#servicelatency)



#### TagList

_Underlying type:_ _string array_

TagList is a unique array of lower case strings
All tag list methods lower case the strings in the arguments



_Appears in:_
- [CIDRList](#cidrlist)



#### TimeRange



TimeRange is used to represent a start and end time



_Appears in:_
- [UserLimits](#userlimits)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `start` _string_ |  |  |  |
| `end` _string_ |  |  |  |


#### URLFromKind

_Underlying type:_ _string_

URLFromKind is the type of resource to load the NATS URL from.

_Validation:_
- Enum: [ConfigMap Secret]

_Appears in:_
- [URLFromReference](#urlfromreference)

| Field | Description |
| --- | --- |
| `ConfigMap` |  |
| `Secret` |  |


#### URLFromReference



URLFromReference describes how to load the NATS URL from a ConfigMap or Secret.



_Appears in:_
- [NatsClusterSpec](#natsclusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _[URLFromKind](#urlfromkind)_ | Kind is the type of resource to load from: ConfigMap or Secret. |  | Enum: [ConfigMap Secret] <br />Required <br /> |
| `name` _string_ | Name of the ConfigMap or Secret. |  | Required <br /> |
| `namespace` _string_ | Namespace of the resource. When empty, defaults to the NatsCluster's namespace. |  |  |
| `key` _string_ | Key in the ConfigMap or Secret whose value is the NATS URL. |  | Required <br /> |


#### User



User is the Schema for the users API.



_Appears in:_
- [UserList](#userlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `User` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[UserSpec](#userspec)_ |  |  |  |
| `status` _[UserStatus](#userstatus)_ |  |  |  |


#### UserClaims







_Appears in:_
- [UserStatus](#userstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountName` _string_ | Deprecated. Will be removed in a future release (>v0.5.0). Ref: https://github.com/WirelessCar/nauth/issues/102 |  |  |
| `displayName` _string_ | DisplayName is an optional name for the NATS resource representing the user. |  |  |
| `expiresAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | ExpiresAt is the absolute time when the generated user JWT expires. |  |  |
| `permissions` _[Permissions](#permissions)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |
| `userLimits` _[UserLimits](#userlimits)_ |  |  |  |




#### UserLimits







_Appears in:_
- [UserClaims](#userclaims)
- [UserSpec](#userspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `src` _[CIDRList](#cidrlist)_ | Src is a comma separated list of CIDR specifications |  |  |
| `times` _[TimeRange](#timerange) array_ |  |  |  |
| `timesLocation` _string_ |  |  |  |


#### UserList



UserList contains a list of User.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `UserList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[User](#user) array_ |  |  |  |


#### UserSpec



UserSpec defines the desired state of User.



_Appears in:_
- [User](#user)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountName` _string_ | AccountName references the account used to create the user. |  |  |
| `displayName` _string_ | DisplayName is an optional name for the NATS resource representing the user. May be derived if absent. |  |  |
| `expiresAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | ExpiresAt is an optional absolute time when the generated user JWT expires. |  |  |
| `permissions` _[Permissions](#permissions)_ |  |  |  |
| `userLimits` _[UserLimits](#userlimits)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |
| `signingKeyRef` _[AccountSigningKeyRef](#accountsigningkeyref)_ | SigningKeyRef optionally references the signing key used to sign this User's<br />JWT. When absent, the Account's implicit signing key is used. The referenced<br />AccountSigningKey's public key must appear in Account.status.claims.signingKeys<br />at reconciliation time. |  |  |


#### UserStatus



UserStatus defines the observed state of User.



_Appears in:_
- [User](#user)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ |  |  |  |
| `claims` _[UserClaims](#userclaims)_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ |  |  |  |
| `operatorVersion` _string_ |  |  |  |
