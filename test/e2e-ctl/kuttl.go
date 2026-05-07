package main

import (
	"fmt"
	"os"
)

const kuttlNamespaceEnv = "NAMESPACE"

func namespaceFromFlagOrEnv(namespace string) (string, error) {
	if namespace != "" {
		return namespace, nil
	}
	if namespace = os.Getenv(kuttlNamespaceEnv); namespace != "" {
		return namespace, nil
	}
	return "", fmt.Errorf("--namespace must be set or %s must be present", kuttlNamespaceEnv)
}
