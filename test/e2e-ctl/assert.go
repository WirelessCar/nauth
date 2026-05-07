package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
)

type assertOperatorSecretsOptions struct {
	namespace          string
	natsCluster        string
	operatorSignSecret string
	systemCredsSecret  string
	log                logger
}

type assertAccountSecretsOptions struct {
	namespace                  string
	accountName                string
	forbidLegacyClusterSecrets bool
	log                        logger
}

type assertUserCredsSecretOptions struct {
	namespace  string
	userName   string
	secretName string
	log        logger
}

func newAssertCommand(ctx context.Context, log logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assert",
		Short: "Run e2e assertions that are awkward to express in KUTTL YAML",
	}
	cmd.AddCommand(
		newAssertOperatorSecretsCommand(ctx, log),
		newAssertAccountSecretsCommand(ctx, log),
		newAssertUserCredsSecretCommand(ctx, log),
	)
	return cmd
}

func newAssertOperatorSecretsCommand(ctx context.Context, log logger) *cobra.Command {
	opts := assertOperatorSecretsOptions{
		log: log,
	}

	cmd := &cobra.Command{
		Use:   "operator-secrets",
		Short: "Assert operator NatsCluster secret references",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssertOperatorSecrets(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.namespace, "namespace", "", "NatsCluster namespace")
	cmd.Flags().StringVar(&opts.natsCluster, "nats-cluster", "", "NatsCluster name")
	cmd.Flags().StringVar(&opts.operatorSignSecret, "operator-sign-secret", "", "expected operator signing key Secret")
	cmd.Flags().StringVar(&opts.systemCredsSecret, "system-creds-secret", "", "expected system account user creds Secret")
	mustMarkFlagRequired(cmd, "namespace")
	mustMarkFlagRequired(cmd, "nats-cluster")
	mustMarkFlagRequired(cmd, "operator-sign-secret")
	mustMarkFlagRequired(cmd, "system-creds-secret")
	return cmd
}

func newAssertAccountSecretsCommand(ctx context.Context, log logger) *cobra.Command {
	opts := assertAccountSecretsOptions{
		log: log,
	}

	cmd := &cobra.Command{
		Use:   "account-secrets",
		Short: "Assert generated Account secrets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssertAccountSecrets(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.namespace, "namespace", "", "account namespace; defaults to KUTTL NAMESPACE")
	cmd.Flags().StringVar(&opts.accountName, "account", "", "account name")
	cmd.Flags().BoolVar(&opts.forbidLegacyClusterSecrets, "forbid-legacy-cluster-secrets", false, "assert that legacy NatsCluster secret copies do not exist")
	mustMarkFlagRequired(cmd, "account")
	return cmd
}

func newAssertUserCredsSecretCommand(ctx context.Context, log logger) *cobra.Command {
	opts := assertUserCredsSecretOptions{
		log: log,
	}

	cmd := &cobra.Command{
		Use:   "user-creds-secret",
		Short: "Assert generated User creds Secret",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssertUserCredsSecret(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.namespace, "namespace", "", "user namespace; defaults to KUTTL NAMESPACE")
	cmd.Flags().StringVar(&opts.userName, "user", "", "user name")
	cmd.Flags().StringVar(&opts.secretName, "secret", "", "user creds Secret name")
	mustMarkFlagRequired(cmd, "user")
	mustMarkFlagRequired(cmd, "secret")
	return cmd
}

func runAssertOperatorSecrets(ctx context.Context, opts assertOperatorSecretsOptions) error {
	opts.log.Infof("validate operator NatsCluster secrets and references")
	if _, err := getSecretData(ctx, opts.namespace, opts.operatorSignSecret, "default"); err != nil {
		return err
	}
	if _, err := getSecretData(ctx, opts.namespace, opts.systemCredsSecret, "default"); err != nil {
		return err
	}

	cluster, err := getNatsCluster(ctx, opts.namespace, opts.natsCluster)
	if err != nil {
		return err
	}
	if cluster.Spec.OperatorSigningKeySecretRef.Name != opts.operatorSignSecret {
		return fmt.Errorf("expected operator signing key Secret %q, got %q", opts.operatorSignSecret, cluster.Spec.OperatorSigningKeySecretRef.Name)
	}
	if cluster.Spec.SystemAccountUserCredsSecretRef.Name != opts.systemCredsSecret {
		return fmt.Errorf("expected system account user creds Secret %q, got %q", opts.systemCredsSecret, cluster.Spec.SystemAccountUserCredsSecretRef.Name)
	}
	return nil
}

func runAssertAccountSecrets(ctx context.Context, opts assertAccountSecretsOptions) error {
	namespace, err := namespaceFromFlagOrEnv(opts.namespace)
	if err != nil {
		return err
	}

	opts.log.Infof("validate Account id and generated Account secrets")
	if opts.forbidLegacyClusterSecrets {
		if err := assertNoSecretsWithLabel(ctx, namespace, "nauth.io/secret-type=operator-sign"); err != nil {
			return err
		}
		if err := assertNoSecretsWithLabel(ctx, namespace, "nauth.io/secret-type=system-account-user-creds"); err != nil {
			return err
		}
	}

	accountID, err := getAccountID(ctx, namespace, opts.accountName)
	if err != nil {
		return err
	}
	rootSecretSelector := fmt.Sprintf("account.nauth.io/id=%s,nauth.io/secret-type=account-root", accountID)
	if err := assertSecretDataByLabels(ctx, namespace, rootSecretSelector, "default"); err != nil {
		return err
	}
	signSecretSelector := fmt.Sprintf("account.nauth.io/id=%s,nauth.io/secret-type=account-sign", accountID)
	if err := assertSecretDataByLabels(ctx, namespace, signSecretSelector, "default"); err != nil {
		return err
	}
	return nil
}

func runAssertUserCredsSecret(ctx context.Context, opts assertUserCredsSecretOptions) error {
	namespace, err := namespaceFromFlagOrEnv(opts.namespace)
	if err != nil {
		return err
	}

	opts.log.Infof("validate User id and generated User creds Secret")
	userID, err := getUserID(ctx, namespace, opts.userName)
	if err != nil {
		return err
	}
	if userID == "" {
		return fmt.Errorf("user id is missing for %s/%s", namespace, opts.userName)
	}

	secret, err := getSecret(ctx, namespace, opts.secretName)
	if err != nil {
		return err
	}
	if len(secret.Data["user.creds"]) == 0 {
		return fmt.Errorf("secret %s/%s does not contain user.creds", namespace, opts.secretName)
	}
	secretType := secret.Labels["nauth.io/secret-type"]
	if secretType != "user-creds" {
		return fmt.Errorf("secret %s/%s has unexpected nauth.io/secret-type label %q", namespace, opts.secretName, secretType)
	}
	managedLabel := secret.Labels["nauth.io/managed"]
	if managedLabel != "true" {
		return fmt.Errorf("secret %s/%s has unexpected nauth.io/managed label %q", namespace, opts.secretName, managedLabel)
	}
	if len(secret.OwnerReferences) == 0 || secret.OwnerReferences[0].Name != opts.userName {
		return fmt.Errorf("secret %s/%s is not owned by User %s", namespace, opts.secretName, opts.userName)
	}
	return nil
}

func getUserID(ctx context.Context, namespace, userName string) (string, error) {
	userID, err := kubectl(ctx,
		"get", "users.nauth.io", userName,
		"-n", namespace,
		"-o", `jsonpath={.metadata.labels.user\.nauth\.io/id}`,
	)
	if err != nil {
		return "", fmt.Errorf("resolve user id for %s/%s: %w", namespace, userName, err)
	}
	return userID, nil
}

func assertSecretDataByLabels(ctx context.Context, namespace, labelSelector, key string) error {
	value, err := kubectl(ctx,
		"get", "secret",
		"-n", namespace,
		"-l", labelSelector,
		"-o", fmt.Sprintf("jsonpath={.items[0].data.%s}", key),
	)
	if err != nil {
		return fmt.Errorf("resolve Secret in namespace %s with labels %q: %w", namespace, labelSelector, err)
	}
	if value == "" {
		return fmt.Errorf("secret in namespace %s with labels %q does not contain key %q", namespace, labelSelector, key)
	}

	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return fmt.Errorf("decode Secret in namespace %s with labels %q key %q: %w", namespace, labelSelector, key, err)
	}
	return nil
}

func assertNoSecretsWithLabel(ctx context.Context, namespace, labelSelector string) error {
	output, err := kubectl(ctx, "get", "secret", "-n", namespace, "-l", labelSelector, "-o", "json")
	if err != nil {
		return fmt.Errorf("list Secrets in namespace %s with labels %q: %w", namespace, labelSelector, err)
	}

	list := struct {
		Items []json.RawMessage `json:"items"`
	}{}
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return fmt.Errorf("decode Secret list in namespace %s with labels %q: %w", namespace, labelSelector, err)
	}
	if len(list.Items) != 0 {
		return fmt.Errorf("found %d Secret(s) in namespace %s with forbidden labels %q", len(list.Items), namespace, labelSelector)
	}
	return nil
}

func getSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	output, err := kubectl(ctx, "get", "secret", name, "-n", namespace, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("get Secret %s/%s: %w", namespace, name, err)
	}

	secret := &corev1.Secret{}
	if err := json.Unmarshal([]byte(output), secret); err != nil {
		return nil, fmt.Errorf("decode Secret %s/%s: %w", namespace, name, err)
	}
	return secret, nil
}
