package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/spf13/cobra"
)

const systemClaimsUpdateSubject = "$SYS.REQ.CLAIMS.UPDATE"

type accountUploadJWTOptions struct {
	jwtFile        string
	natsCluster    string
	natsNamespace  string
	natsService    string
	natsURL        string
	timeoutSeconds int
	log            logger
}

type accountAnnotateIDOptions struct {
	namespace   string
	accountName string
	annotation  string
	log         logger
}

type accountDeleteOptions struct {
	namespace   string
	accountName string
	wait        bool
	log         logger
}

func newAccountCommand(ctx context.Context, log logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manage Account test data during e2e tests",
	}
	cmd.AddCommand(
		newAccountUploadJWTCommand(ctx, log),
		newAccountAnnotateIDCommand(ctx, log),
		newAccountDeleteCommand(ctx, log),
	)
	return cmd
}

func newAccountUploadJWTCommand(ctx context.Context, log logger) *cobra.Command {
	opts := accountUploadJWTOptions{
		natsCluster:   "local-nats",
		natsNamespace: defaultNATSNamespace,
		natsService:   defaultNATSService,
		log:           log,
	}

	cmd := &cobra.Command{
		Use:   "upload-jwt",
		Short: "Upload an Account JWT through the NATS system account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAccountUploadJWT(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.jwtFile, "jwt-file", "", "account JWT file to upload")
	cmd.Flags().StringVar(&opts.natsCluster, "nats-cluster", opts.natsCluster, "NatsCluster name")
	cmd.Flags().StringVar(&opts.natsNamespace, "nats-namespace", opts.natsNamespace, "NatsCluster namespace")
	cmd.Flags().StringVar(&opts.natsService, "nats-service", opts.natsService, "NATS service name used for port-forward")
	cmd.Flags().StringVar(&opts.natsURL, "nats-url", "", "NATS URL; skips port-forward when set")
	cmd.Flags().IntVar(&opts.timeoutSeconds, "timeout", 0, "timeout in seconds for the upload operation")
	mustMarkFlagRequired(cmd, "jwt-file")
	mustMarkFlagRequired(cmd, "timeout")
	return cmd
}

func newAccountAnnotateIDCommand(ctx context.Context, log logger) *cobra.Command {
	opts := accountAnnotateIDOptions{
		log: log,
	}

	cmd := &cobra.Command{
		Use:   "annotate-id",
		Short: "Annotate an Account with its generated account id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAccountAnnotateID(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.namespace, "namespace", "", "account namespace; defaults to KUTTL NAMESPACE")
	cmd.Flags().StringVar(&opts.accountName, "account", "", "account name")
	cmd.Flags().StringVar(&opts.annotation, "annotation", "", "annotation key to store the account id in")
	mustMarkFlagRequired(cmd, "account")
	mustMarkFlagRequired(cmd, "annotation")
	return cmd
}

func newAccountDeleteCommand(ctx context.Context, log logger) *cobra.Command {
	opts := accountDeleteOptions{
		wait: true,
		log:  log,
	}

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an Account resource",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAccountDelete(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.namespace, "namespace", "", "account namespace; defaults to KUTTL NAMESPACE")
	cmd.Flags().StringVar(&opts.accountName, "account", "", "account name")
	cmd.Flags().BoolVar(&opts.wait, "wait", opts.wait, "wait for Account deletion")
	mustMarkFlagRequired(cmd, "account")
	return cmd
}

func runAccountUploadJWT(ctx context.Context, opts accountUploadJWTOptions) error {
	if opts.timeoutSeconds < 1 {
		return fmt.Errorf("--timeout must be at least 1")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(opts.timeoutSeconds)*time.Second)
	defer cancel()

	opts.log.Infof("read Account JWT from %s", opts.jwtFile)
	accountJWT, err := os.ReadFile(opts.jwtFile)
	if err != nil {
		return fmt.Errorf("read account JWT file %s: %w", opts.jwtFile, err)
	}
	accountJWT = []byte(strings.TrimSpace(string(accountJWT)))
	if len(accountJWT) == 0 {
		return fmt.Errorf("account JWT file %s is empty", opts.jwtFile)
	}

	opts.log.Infof("resolve NatsCluster %s/%s", opts.natsNamespace, opts.natsCluster)
	cluster, err := getNatsCluster(timeoutCtx, opts.natsNamespace, opts.natsCluster)
	if err != nil {
		return err
	}

	resolvedURL, err := resolveNATSURL(timeoutCtx, cluster, opts.natsNamespace)
	if err != nil {
		return err
	}
	opts.log.Infof("resolved NATS URL from NatsCluster: %s", resolvedURL)

	systemCreds, err := resolveSystemAccountCreds(timeoutCtx, cluster, opts.natsNamespace)
	if err != nil {
		return err
	}

	natsURL := opts.natsURL
	var forward *kubectlPortForward
	if natsURL == "" {
		resource := "svc/" + opts.natsService
		opts.log.Infof("open port-forward for %s/%s", opts.natsNamespace, resource)
		forward, err = startKubectlPortForward(timeoutCtx, opts.natsNamespace, resource, defaultNATSPort)
		if err != nil {
			return err
		}
		defer func() {
			if err := forward.Close(); err != nil {
				opts.log.Errorf("failed to close port-forward: %v", err)
			}
		}()
		natsURL = fmt.Sprintf("nats://127.0.0.1:%d", forward.localPort)
	}

	opts.log.Infof("connect to NATS at %s", natsURL)
	conn, err := nats.Connect(
		natsURL,
		nats.Name("e2e-ctl"),
		nats.NoReconnect(),
		nats.Timeout(natsRequestTimeout),
		nats.UserJWTAndSeed(systemCreds.jwt, systemCreds.seed),
	)
	if err != nil {
		return fmt.Errorf("connect to NATS at %s: %w", natsURL, err)
	}
	defer conn.Close()

	opts.log.Infof("upload Account JWT through NATS system account")
	msg, err := conn.RequestWithContext(timeoutCtx, systemClaimsUpdateSubject, accountJWT)
	if err != nil {
		return fmt.Errorf("upload account JWT: %w", err)
	}

	if err := assertClaimsUpdateResponse(msg.Data); err != nil {
		return err
	}
	opts.log.Infof("Account JWT upload succeeded")
	return nil
}

func runAccountAnnotateID(ctx context.Context, opts accountAnnotateIDOptions) error {
	namespace, err := namespaceFromFlagOrEnv(opts.namespace)
	if err != nil {
		return err
	}

	opts.log.Infof("resolve Account id for %s/%s", namespace, opts.accountName)
	accountID, err := getAccountID(ctx, namespace, opts.accountName)
	if err != nil {
		return err
	}

	opts.log.Infof("annotate Account %s/%s with %s", namespace, opts.accountName, opts.annotation)
	_, err = kubectl(ctx,
		"annotate", "accounts.nauth.io", opts.accountName,
		"-n", namespace,
		"--overwrite",
		fmt.Sprintf("%s=%s", opts.annotation, accountID),
	)
	if err != nil {
		return fmt.Errorf("annotate Account %s/%s with account id: %w", namespace, opts.accountName, err)
	}
	return nil
}

func runAccountDelete(ctx context.Context, opts accountDeleteOptions) error {
	namespace, err := namespaceFromFlagOrEnv(opts.namespace)
	if err != nil {
		return err
	}

	opts.log.Infof("delete Account %s/%s", namespace, opts.accountName)
	_, err = kubectl(ctx,
		"delete", "accounts.nauth.io", opts.accountName,
		"-n", namespace,
		fmt.Sprintf("--wait=%t", opts.wait),
	)
	if err != nil {
		return fmt.Errorf("delete Account %s/%s: %w", namespace, opts.accountName, err)
	}
	return nil
}

func getAccountID(ctx context.Context, namespace, accountName string) (string, error) {
	accountID, err := kubectl(ctx,
		"get", "accounts.nauth.io", accountName,
		"-n", namespace,
		"-o", `jsonpath={.metadata.labels.account\.nauth\.io/id}`,
	)
	if err != nil {
		return "", fmt.Errorf("resolve account id for %s/%s: %w", namespace, accountName, err)
	}
	if accountID == "" {
		return "", fmt.Errorf("account id is missing for %s/%s", namespace, accountName)
	}
	return accountID, nil
}

func getAccountRootSeed(ctx context.Context, namespace, accountName, accountID string) (string, error) {
	rootSeedB64, err := kubectl(ctx,
		"get", "secret",
		"-n", namespace,
		"-l", fmt.Sprintf("account.nauth.io/id=%s,nauth.io/secret-type=account-root", accountID),
		"-o", "jsonpath={.items[0].data.default}",
	)
	if err != nil {
		return "", fmt.Errorf("resolve account root seed secret for %s/%s: %w", namespace, accountName, err)
	}
	if rootSeedB64 == "" {
		return "", fmt.Errorf("account root seed secret is missing for %s/%s", namespace, accountName)
	}

	rootSeed, err := base64.StdEncoding.DecodeString(rootSeedB64)
	if err != nil {
		return "", fmt.Errorf("decode account root seed for %s/%s: %w", namespace, accountName, err)
	}
	return string(rootSeed), nil
}

func getNatsCluster(ctx context.Context, namespace, name string) (*v1alpha1.NatsCluster, error) {
	output, err := kubectl(ctx, "get", "natsclusters.nauth.io", name, "-n", namespace, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("get NatsCluster %s/%s: %w", namespace, name, err)
	}

	cluster := &v1alpha1.NatsCluster{}
	if err := json.Unmarshal([]byte(output), cluster); err != nil {
		return nil, fmt.Errorf("decode NatsCluster %s/%s: %w", namespace, name, err)
	}
	return cluster, nil
}

func resolveNATSURL(ctx context.Context, cluster *v1alpha1.NatsCluster, fallbackNamespace string) (string, error) {
	if cluster.Spec.URL != "" {
		return cluster.Spec.URL, nil
	}
	if cluster.Spec.URLFrom == nil {
		return "", fmt.Errorf("NATS URL must be specified via url or urlFrom")
	}

	ref := cluster.Spec.URLFrom
	namespace := ref.Namespace
	if namespace == "" {
		namespace = fallbackNamespace
	}

	switch ref.Kind {
	case v1alpha1.URLFromKindConfigMap:
		return getConfigMapData(ctx, namespace, ref.Name, ref.Key)
	case v1alpha1.URLFromKindSecret:
		value, err := getSecretData(ctx, namespace, ref.Name, ref.Key)
		if err != nil {
			return "", err
		}
		return string(value), nil
	default:
		return "", fmt.Errorf("unsupported urlFrom.kind %q", ref.Kind)
	}
}

func resolveSystemAccountCreds(ctx context.Context, cluster *v1alpha1.NatsCluster, namespace string) (natsUserCreds, error) {
	ref := cluster.Spec.SystemAccountUserCredsSecretRef
	if ref.Name == "" {
		return natsUserCreds{}, fmt.Errorf("nats cluster does not define spec.systemAccountUserCredsSecretRef.name")
	}
	key := ref.Key
	if key == "" {
		key = "default"
	}

	creds, err := getSecretData(ctx, namespace, ref.Name, key)
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("resolve system account user creds from secret %s/%s key %q: %w", namespace, ref.Name, key, err)
	}
	return parseDecoratedUserCreds(creds)
}

func parseDecoratedUserCreds(contents []byte) (natsUserCreds, error) {
	userJWT, err := jwt.ParseDecoratedJWT(contents)
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("parse user JWT from creds: %w", err)
	}

	userKeyPair, err := nkeys.ParseDecoratedNKey(contents)
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("parse user seed from creds: %w", err)
	}

	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("derive user seed from creds: %w", err)
	}

	return natsUserCreds{
		jwt:       userJWT,
		seed:      string(userSeed),
		formatted: contents,
	}, nil
}

func getSecretData(ctx context.Context, namespace, name, key string) ([]byte, error) {
	value, err := kubectl(ctx,
		"get", "secret", name,
		"-n", namespace,
		"-o", fmt.Sprintf("go-template={{ index .data %q }}", key),
	)
	if err != nil {
		return nil, err
	}
	if value == "" || value == "<no value>" {
		return nil, fmt.Errorf("secret %s/%s does not contain key %q", namespace, name, key)
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode Secret %s/%s key %q: %w", namespace, name, key, err)
	}
	return decoded, nil
}

func getConfigMapData(ctx context.Context, namespace, name, key string) (string, error) {
	value, err := kubectl(ctx,
		"get", "configmap", name,
		"-n", namespace,
		"-o", fmt.Sprintf("go-template={{ index .data %q }}", key),
	)
	if err != nil {
		return "", err
	}
	if value == "" || value == "<no value>" {
		return "", fmt.Errorf("configmap %s/%s does not contain key %q", namespace, name, key)
	}
	return value, nil
}

func assertClaimsUpdateResponse(data []byte) error {
	type claimsUpdateResponseData struct {
		Code  int `json:"code"`
		Error *struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	}

	response := struct {
		Code  int `json:"code"`
		Error *struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
		Data *claimsUpdateResponseData `json:"data"`
	}{}

	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("decode account JWT upload response %q: %w", string(data), err)
	}

	result := claimsUpdateResponseData{
		Code:  response.Code,
		Error: response.Error,
	}
	if response.Data != nil {
		result = *response.Data
	}
	if result.Error != nil {
		return fmt.Errorf("account JWT upload returned error code=%d description=%q", result.Error.Code, result.Error.Description)
	}
	if result.Code != 200 {
		return fmt.Errorf("account JWT upload returned unexpected response code %d: %s", result.Code, string(data))
	}
	return nil
}
