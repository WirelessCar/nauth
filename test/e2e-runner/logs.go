package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const kuttlLogDir = "test/logs/kuttl"

// KUTTL uses Go's testing output format. Parallel test logs are emitted under
// NAME markers while each test is running, followed by PASS/FAIL/SKIP markers
// after the test completes.
var kuttlTestResult = regexp.MustCompile(`^--- (PASS|FAIL|SKIP): ([^[:space:]]+) \(`)
var kuttlTestName = regexp.MustCompile(`^=== NAME[[:space:]]+kuttl/harness/([^[:space:]]+)$`)

type testLog struct {
	name   string
	status string
	data   []byte
}

func splitKuttlLogs(output []byte) []testLog {
	logsByName := make(map[string]*testLog)
	var current *testLog

	getLog := func(name string) *testLog {
		if log, ok := logsByName[name]; ok {
			return log
		}
		log := &testLog{name: name}
		logsByName[name] = log
		return log
	}

	for _, line := range bytes.SplitAfter(output, []byte("\n")) {
		lineWithoutNewline := bytes.TrimSuffix(line, []byte("\n"))
		if match := kuttlTestName.FindSubmatch(lineWithoutNewline); len(match) != 0 {
			name := string(match[1])
			if !strings.Contains(name, "/") {
				current = getLog(name)
				current.data = append(current.data, line...)
			}
			continue
		}

		if bytes.HasPrefix(lineWithoutNewline, []byte("=== NAME")) ||
			bytes.HasPrefix(lineWithoutNewline, []byte("=== CONT")) {
			current = nil
			continue
		}

		if match := kuttlTestResult.FindSubmatch(lineWithoutNewline); len(match) != 0 {
			name := string(match[2])
			if strings.HasPrefix(name, "kuttl/harness/") {
				testName := strings.TrimPrefix(name, "kuttl/harness/")
				if !strings.Contains(testName, "/") {
					current = getLog(testName)
					current.status = string(match[1])
					current.data = append(current.data, line...)
				}
			}
			current = nil
			continue
		}

		if current != nil {
			current.data = append(current.data, line...)
		}
	}

	logs := make([]testLog, 0, len(logsByName))
	for _, log := range logsByName {
		logs = append(logs, *log)
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].name < logs[j].name
	})
	return logs
}

func writeKuttlLogs(output []byte, rootDir string) ([]string, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		if err := os.Remove(filepath.Join(rootDir, entry.Name())); err != nil {
			return nil, err
		}
	}

	logs := splitKuttlLogs(output)
	failedPaths := make([]string, 0)
	for _, log := range logs {
		path := filepath.Join(rootDir, log.name+".log")
		if err := os.WriteFile(path, log.data, 0o644); err != nil {
			return nil, err
		}
		if log.status == "FAIL" {
			failedPaths = append(failedPaths, path)
		}
	}
	return failedPaths, nil
}

func writeRawKuttlLog(output []byte, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, output, 0o644)
}

func logPathMessage(paths []string) string {
	if len(paths) == 0 {
		return "No failed per-test KUTTL logs were produced."
	}

	var message strings.Builder
	fmt.Fprintln(&message, "Per-test KUTTL logs for failed tests:")
	for _, path := range paths {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			absolutePath = path
		}
		fmt.Fprintf(&message, "  %s\n", absolutePath)
	}
	return strings.TrimSuffix(message.String(), "\n")
}
