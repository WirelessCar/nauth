package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func main() {
	if len(os.Args) != 3 {
		_, _ = fmt.Fprintf(os.Stderr, "usage: %s <account-id> <account-root-seed>\n", os.Args[0])
		os.Exit(1)
	}

	accountID := os.Args[1]
	accountRootSeed := os.Args[2]

	accountKeyPair, err := nkeys.FromSeed([]byte(accountRootSeed))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build account key pair from root seed: %v\n", err)
		os.Exit(1)
	}

	userKeyPair, err := nkeys.CreateUser()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create temporary user key pair: %v\n", err)
		os.Exit(1)
	}

	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "derive temporary user public key: %v\n", err)
		os.Exit(1)
	}

	userSeed, err := userKeyPair.Seed()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "derive temporary user seed: %v\n", err)
		os.Exit(1)
	}

	claims := jwt.NewUserClaims(userPublicKey)
	claims.IssuerAccount = accountID
	claims.Expires = time.Now().Add(30 * time.Second).Unix()
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
		_, _ = fmt.Fprintf(os.Stderr, "sign temporary user JWT: %v\n", err)
		os.Exit(1)
	}

	userCreds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "format temporary user creds: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(string(userCreds))
}
