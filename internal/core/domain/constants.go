package domain

const (
	DefaultSecretKeyName        = "default"
	LabelAccountId              = "account.nauth.io/id"
	LabelAccountSignedBy        = "account.nauth.io/signed-by"
	LabelUserId                 = "user.nauth.io/id"
	LabelUserAccountId          = "user.nauth.io/account-id"
	LabelUserSignedBy           = "user.nauth.io/signed-by"
	UserCredentialSecretKeyName = "user.creds"

	LabelSecretType   = "nauth.io/secret-type"
	LabelManaged      = "nauth.io/managed"
	LabelManagedValue = "true"
)

const (
	SecretNameAccountRootTemplate = "%s-ac-root-%s"
	SecretNameAccountSignTemplate = "%s-ac-sign-%s"
)

const (
	SecretTypeAccountRoot             = "account-root"
	SecretTypeAccountSign             = "account-sign"
	SecretTypeOperatorSign            = "operator-sign"
	SecretTypeSystemAccountAdminCreds = "system-account-admin-creds"
	SecretTypeUserCredentials         = "user-creds"
)

const (
	DeprecatedSecretNameAccountRootTemplate = "%s-ac-root"
	DeprecatedSecretNameAccountSignTemplate = "%s-ac-sign"
)

const (
	OperatorVersion = "OPERATOR_VERSION"
)
