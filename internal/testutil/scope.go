package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func SanitizeTestName(name string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", ":", "-", "#", "-", "_", "-")
	return strings.ToLower(replacer.Replace(name))
}

func ScopedTestName(prefix, testName string) string {
	slug := SanitizeTestName(testName)
	hash := ShortHash(testName)
	maxSlugLen := 63 - len(prefix) - len(hash) - 2
	if maxSlugLen < 1 {
		return prefix + "-" + hash
	}
	if len(slug) > maxSlugLen {
		slug = slug[:maxSlugLen]
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return prefix + "-" + hash
	}
	return prefix + "-" + slug + "-" + hash
}

func ShortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:6]
}
