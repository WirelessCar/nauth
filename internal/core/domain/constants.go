package domain

const (
	DefaultSecretKeyName        = "default"
	LabelAccountId              = "account.nauth.io/id"
	LabelAccountSignedBy        = "account.nauth.io/signed-by"
	LabelUserId                 = "user.nauth.io/id"
	LabelUserAccountId          = "user.nauth.io/account-id"
	LabelUserSignedBy           = "user.nauth.io/signed-by"
	UserCredentialSecretKeyName = "user.creds"

	LabelSecretType = "nauth.io/secret-type"
	LabelManaged    = "nauth.io/managed"
)

const (
	SecretNameAccountRoot = "%s-ac-root-%s"
	SecretNameAccountSign = "%s-ac-sign-%s"
)

const (
	SecretTypeAccountRoot     = "ac-root"
	SecretTypeAccountSign     = "ac-sign"
	SecretTypeOperatorSign    = "op-sign"
	SecretTypeOperatorCreds   = "op-creds"
	SecretTypeUserCredentials = "us-creds"
)

const (
	DeprecatedSecretNameAccountRoot = "%s-ac-root"
	DeprecatedSecretNameAccountSign = "%s-ac-sign"
)

const (
	OperatorVersion = "OPERATOR_VERSION"
)
