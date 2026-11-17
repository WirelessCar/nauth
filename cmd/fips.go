package main

import (
	"crypto/fips140"
	"fmt"
)

// fipsVersionLatest is what crypto/fips140.Version reports when the binary was built against the
// in-tree Go Cryptographic Module rather than a frozen, CMVP-validated snapshot (GOFIPS140=vX.Y.Z).
const fipsVersionLatest = "latest"

type fipsStatus struct {
	// Enabled reports whether the Go Cryptographic Module runs in FIPS 140-3 mode (GODEBUG=fips140=on).
	Enabled bool
	// Version is the Go Cryptographic Module version, e.g. "v1.0.0", or "latest".
	Version string
}

func currentFIPSStatus() fipsStatus {
	return fipsStatus{
		Enabled: fips140.Enabled(),
		Version: fips140.Version(),
	}
}

// checkFIPS returns an error when FIPS mode is required but the binary does not run in FIPS 140-3
// mode with a frozen module. Build with GOFIPS140=v1.0.0 to satisfy it.
func checkFIPS(status fipsStatus, required bool) error {
	if !required {
		return nil
	}
	if !status.Enabled {
		return fmt.Errorf("FIPS 140-3 mode is required but not enabled; " +
			"use the -fips image or build with GOFIPS140=<validated module version>")
	}
	if status.Version == fipsVersionLatest {
		return fmt.Errorf("FIPS 140-3 mode is required but the Go Cryptographic Module version %q "+
			"is not a frozen, validated module; build with GOFIPS140=<validated module version>", status.Version)
	}
	return nil
}
