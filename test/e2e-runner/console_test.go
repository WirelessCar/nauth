package main

import (
	"bytes"
	"testing"
)

func TestKuttlConsoleWriter(t *testing.T) {
	var output bytes.Buffer
	writer := &kuttlConsoleWriter{output: &output}

	input := "" +
		"=== RUN   kuttl\n" +
		"    logger.go:42: 09:32:08 | setup command output\n" +
		"=== RUN   kuttl/harness\n" +
		"=== RUN   kuttl/harness/account-export\n" +
		"=== PAUSE kuttl/harness/account-export\n" +
		"=== CONT  kuttl/harness/account-export\n" +
		"=== NAME  kuttl/harness/account-export\n" +
		"    logger.go:42: 09:32:08 | noisy test output\n" +
		"    harness.go:430: run tests finished\n" +
		"--- PASS: kuttl/harness/account-export (1.00s)\n" +
		"=== NAME  kuttl\n" +
		"    harness.go:547: cleaning up\n" +
		"--- PASS: kuttl (1.00s)\n" +
		"    --- PASS: kuttl/harness/account-export (1.00s)\n" +
		"PAS"
	if _, err := writer.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}

	expected := "" +
		"=== RUN   kuttl\n" +
		"    logger.go:42: 09:32:08 | setup command output\n" +
		"=== RUN   kuttl/harness\n" +
		"=== RUN   kuttl/harness/account-export\n" +
		"=== PAUSE kuttl/harness/account-export\n" +
		"=== CONT  kuttl/harness/account-export\n" +
		"    harness.go:430: run tests finished\n" +
		"--- PASS: kuttl/harness/account-export (1.00s)\n" +
		"=== NAME  kuttl\n" +
		"    harness.go:547: cleaning up\n" +
		"--- PASS: kuttl (1.00s)\n" +
		"    --- PASS: kuttl/harness/account-export (1.00s)\n" +
		"PAS"
	if output.String() != expected {
		t.Errorf("unexpected console output: %q", output.String())
	}
}
