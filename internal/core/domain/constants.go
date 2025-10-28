package domain

const (
	DefaultSecretKeyName        = "default"
	LabelAccountId              = "account.nauth.io/id"
	LabelAccountSignedBy        = "account.nauth.io/signed-by"
	LabelAccountSecretType      = "account.nauth.io/secret-type"
	LabelUserId                 = "user.nauth.io/id"
	LabelUserAccountId          = "user.nauth.io/account-id"
	LabelUserSignedBy           = "user.nauth.io/signed-by"
	UserCredentialSecretKeyName = "user.creds"
)

const (
	SecretNameAccountRoot  = "%s-ac-root-%s"
	SecretNameAccountSign  = "%s-ac-sign-%s"
	SecretNameOperatorSign = "operator-op-sign"
)

const (
	SecretTypeAccountRoot = "ac-root"
	SecretTypeAccountSign = "ac-sign"
)

const (
	DeprecatedSecretNameAccountRoot = "%s-ac-root"
	DeprecatedSecretNameAccountSign = "%s-ac-sign"
)

const (
	OperatorVersion = "OPERATOR_VERSION"
)
