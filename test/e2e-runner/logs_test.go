package main

import (
	"path/filepath"
	"testing"
)

func TestSplitKuttlLogs(t *testing.T) {
	output := []byte("" +
		"=== NAME  kuttl/harness/second\n" +
		"    second log\n" +
		"=== NAME  kuttl/harness/first\n" +
		"    first log\n" +
		"--- FAIL: kuttl/harness/second (1.00s)\n" +
		"--- PASS: kuttl/harness/first (2.00s)\n" +
		"--- FAIL: kuttl/harness (2.00s)\n")

	logs := splitKuttlLogs(output)
	if len(logs) != 2 {
		t.Fatalf("expected two test logs, got %d", len(logs))
	}

	if logs[0].name != "first" || logs[0].status != "PASS" {
		t.Errorf("unexpected first log: %#v", logs[0])
	}
	if string(logs[0].data) != "=== NAME  kuttl/harness/first\n    first log\n--- PASS: kuttl/harness/first (2.00s)\n" {
		t.Errorf("unexpected first log data: %q", logs[0].data)
	}

	if logs[1].name != "second" || logs[1].status != "FAIL" {
		t.Errorf("unexpected second log: %#v", logs[1])
	}
	if string(logs[1].data) != "=== NAME  kuttl/harness/second\n    second log\n--- FAIL: kuttl/harness/second (1.00s)\n" {
		t.Errorf("unexpected second log data: %q", logs[1].data)
	}
}

func TestWriteKuttlLogs(t *testing.T) {
	rootDir := t.TempDir()
	failedPaths, err := writeKuttlLogs([]byte("--- FAIL: kuttl/harness/account-export (1.00s)\n    failure\n"), rootDir)
	if err != nil {
		t.Fatal(err)
	}

	expectedPath := filepath.Join(rootDir, "account-export.log")
	if len(failedPaths) != 1 || failedPaths[0] != expectedPath {
		t.Fatalf("unexpected failed paths: %#v", failedPaths)
	}
}
