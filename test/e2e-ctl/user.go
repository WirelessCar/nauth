package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

const userCredsTTL = 2 * time.Minute

type natsUserCreds struct {
	jwt       string
	seed      string
	formatted []byte
}

func createUserCredsForAccount(ctx context.Context, log logger, namespace, accountName string) (natsUserCreds, error) {
	log.Infof("resolve user creds for Account %s/%s", namespace, accountName)

	accountID, err := getAccountID(ctx, namespace, accountName)
	if err != nil {
		return natsUserCreds{}, err
	}

	accountRootSeed, err := getAccountRootSeed(ctx, namespace, accountName, accountID)
	if err != nil {
		return natsUserCreds{}, err
	}

	return createUserCreds(accountID, accountRootSeed)
}

func createUserCreds(accountID string, accountRootSeed string) (natsUserCreds, error) {
	accountKeyPair, err := nkeys.FromSeed([]byte(accountRootSeed))
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("build account key pair from root seed: %w", err)
	}

	userKeyPair, err := nkeys.CreateUser()
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("create user key pair: %w", err)
	}

	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("derive user public key: %w", err)
	}

	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("derive user seed: %w", err)
	}

	claims := jwt.NewUserClaims(userPublicKey)
	claims.IssuerAccount = accountID
	claims.Expires = time.Now().Add(userCredsTTL).Unix()
	claims.Permissions = jwt.Permissions{
		Pub: jwt.Permission{
			Allow: jwt.StringList{"$JS.API.>"},
		},
		Sub: jwt.Permission{
			Allow: jwt.StringList{"_INBOX.>"},
		},
	}

	userJWT, err := claims.Encode(accountKeyPair)
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("sign user JWT: %w", err)
	}

	userCreds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return natsUserCreds{}, fmt.Errorf("format user creds: %w", err)
	}
	return natsUserCreds{
		jwt:       userJWT,
		seed:      string(userSeed),
		formatted: userCreds,
	}, nil
}
