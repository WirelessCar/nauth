package types

const (
	DefaultSecretKeyName        = "default"
	LabelAccountID              = "account.nauth.io/id"
	LabelAccountSignedBy        = "account.nauth.io/signed-by"
	LabelUserID                 = "user.nauth.io/id"
	LabelUserAccountID          = "user.nauth.io/account-id"
	LabelUserSignedBy           = "user.nauth.io/signed-by"
	UserCredentialSecretKeyName = "user.creds"

	LabelSecretType   = "nauth.io/secret-type"
	LabelManaged      = "nauth.io/managed"
	LabelManagedValue = "true"

	LabelManagementPolicy             = "nauth.io/management-policy"
	LabelManagementPolicyObserveValue = "observe"
)

const (
	SecretTypeAccountRoot            = "account-root"
	SecretTypeAccountSign            = "account-sign"
	SecretTypeOperatorSign           = "operator-sign"
	SecretTypeSystemAccountUserCreds = "system-account-user-creds"
	SecretTypeUserCredentials        = "user-creds"
)

const (
	DeprecatedSecretNameAccountRootTemplate = "%s-ac-root"
	DeprecatedSecretNameAccountSignTemplate = "%s-ac-sign"
)
