# API Reference

## Packages
- [nauth.io/v1alpha1](#nauthiov1alpha1)


## nauth.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the nats v1alpha1 API group.

### Resource Types
- [Account](#account)
- [AccountList](#accountlist)
- [User](#user)
- [UserList](#userlist)



#### Account



Account is the Schema for the accounts API.



_Appears in:_
- [AccountList](#accountlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `Account` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AccountSpec](#accountspec)_ |  |  |  |
| `status` _[AccountStatus](#accountstatus)_ |  |  |  |


#### AccountClaims







_Appears in:_
- [AccountStatus](#accountstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountLimits` _[AccountLimits](#accountlimits)_ |  |  |  |
| `exports` _[Exports](#exports)_ |  |  |  |
| `imports` _[Imports](#imports)_ |  |  |  |
| `jetStreamLimits` _[JetStreamLimits](#jetstreamlimits)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |


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
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[Account](#account) array_ |  |  |  |


#### AccountRef







_Appears in:_
- [Import](#import)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `namespace` _string_ |  |  |  |


#### AccountSpec



AccountSpec defines the desired state of Account.



_Appears in:_
- [Account](#account)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountLimits` _[AccountLimits](#accountlimits)_ |  |  |  |
| `exports` _[Exports](#exports)_ |  |  |  |
| `imports` _[Imports](#imports)_ |  |  |  |
| `jetStreamLimits` _[JetStreamLimits](#jetstreamlimits)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |


#### AccountStatus



AccountStatus defines the observed state of Account.



_Appears in:_
- [Account](#account)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `claims` _[AccountClaims](#accountclaims)_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta) array_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |
| `signingKey` _[KeyInfo](#keyinfo)_ |  |  |  |


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


#### KeyInfo







_Appears in:_
- [AccountStatus](#accountstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `creationDate` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |
| `expirationDate` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |


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
- [Export](#export)



#### RevocationList

_Underlying type:_ _object_





_Appears in:_
- [Export](#export)



#### SamplingRate

_Underlying type:_ _integer_





_Appears in:_
- [ServiceLatency](#servicelatency)



#### ServiceLatency







_Appears in:_
- [Export](#export)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sampling` _[SamplingRate](#samplingrate)_ |  |  |  |
| `results` _[Subject](#subject)_ |  |  |  |


#### StringList

_Underlying type:_ _string array_

StringList is a wrapper for an array of strings



_Appears in:_
- [Permission](#permission)



#### Subject

_Underlying type:_ _string_

Subject is a string that represents a NATS subject



_Appears in:_
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


#### User



User is the Schema for the users API.



_Appears in:_
- [UserList](#userlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `nauth.io/v1alpha1` | | |
| `kind` _string_ | `User` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[UserSpec](#userspec)_ |  |  |  |
| `status` _[UserStatus](#userstatus)_ |  |  |  |


#### UserClaims







_Appears in:_
- [UserStatus](#userstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountName` _string_ |  |  |  |
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
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[User](#user) array_ |  |  |  |


#### UserSpec



UserSpec defines the desired state of User.



_Appears in:_
- [User](#user)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accountName` _string_ | AccountName references the account used to create the user. |  |  |
| `permissions` _[Permissions](#permissions)_ |  |  |  |
| `userLimits` _[UserLimits](#userlimits)_ |  |  |  |
| `natsLimits` _[NatsLimits](#natslimits)_ |  |  |  |


#### UserStatus



UserStatus defines the observed state of User.



_Appears in:_
- [User](#user)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta) array_ |  |  |  |
| `claims` _[UserClaims](#userclaims)_ |  |  |  |
| `observedGeneration` _integer_ |  |  |  |
| `reconcileTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ |  |  |  |


