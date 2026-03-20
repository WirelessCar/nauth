package k8s

const (
	DeprecatedSecretNameAccountRootTemplate = "%s-ac-root"
	DeprecatedSecretNameAccountSignTemplate = "%s-ac-sign"
)

const (
	SecretTypeAccountRoot            = "account-root"
	SecretTypeAccountSign            = "account-sign"
	SecretTypeOperatorSign           = "operator-sign"
	SecretTypeSystemAccountUserCreds = "system-account-user-creds"
	SecretTypeUserCredentials        = "user-creds"
	DefaultSecretKeyName             = "default"
	UserCredentialSecretKeyName      = "user.creds"
)
