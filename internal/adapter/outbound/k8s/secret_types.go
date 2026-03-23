package k8s

const (
	DeprecatedSecretNameAccountRootTemplate = "%s-ac-root"
	DeprecatedSecretNameAccountSignTemplate = "%s-ac-sign"
)

const (
	SecretTypeAccountRoot       = "account-root"
	SecretTypeAccountSign       = "account-sign"
	SecretTypeUserCredentials   = "user-creds"
	DefaultSecretKeyName        = "default"
	UserCredentialSecretKeyName = "user.creds"
)
