package nauth

import (
	"testing"

	"github.com/WirelessCar/nauth/internal/domain"
)

func TestHashAccountImportDependencies(t *testing.T) {
	dependencies := []AccountImportDependency{
		{AccountRef: domain.NewNamespacedName("source-b", "account"), ClaimsHash: "claims-b"},
		{AccountRef: domain.NewNamespacedName("source-a", "account"), ClaimsHash: "claims-a"},
	}

	fingerprint := HashAccountImportDependencies(dependencies)

	if fingerprint == "" {
		t.Fatal("expected a fingerprint for non-empty dependencies")
	}
	if fingerprint != HashAccountImportDependencies([]AccountImportDependency{dependencies[1], dependencies[0]}) {
		t.Fatal("expected dependency order not to affect the fingerprint")
	}
	if fingerprint == HashAccountImportDependencies([]AccountImportDependency{
		{AccountRef: dependencies[0].AccountRef, ClaimsHash: "claims-changed"},
		dependencies[1],
	}) {
		t.Fatal("expected a changed source claims hash to change the fingerprint")
	}
	if HashAccountImportDependencies(nil) != "" {
		t.Fatal("expected no fingerprint for accounts without import dependencies")
	}
}
