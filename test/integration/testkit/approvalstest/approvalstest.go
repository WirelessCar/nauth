package approvalstest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	approvals "github.com/approvals/go-approval-tests"
	approvalscore "github.com/approvals/go-approval-tests/core"
)

func VerifyYAML(t *testing.T, data string) {
	t.Helper()
	if err := os.MkdirAll(".approval_tests_temp", 0o755); err != nil {
		t.Fatalf("failed to create approval temp dir: %v", err)
	}
	approvals.VerifyString(t, data, optionsForCurrentFile(t).ForFile().WithExtension(".yaml"))
}

func optionsForCurrentFile(t *testing.T) approvals.VerifyOptions {
	t.Helper()

	_, file, _, ok := runtime.Caller(2)
	if !ok {
		t.Fatal("failed to resolve approval caller file")
	}

	return approvals.Options().ForFile().WithNamer(func(f approvalscore.Failable) approvalscore.ApprovalNamer {
		return &fixedSourceApprovalNamer{
			name:       strings.ReplaceAll(f.Name(), "/", "."),
			sourceFile: file,
		}
	})
}

type fixedSourceApprovalNamer struct {
	name       string
	sourceFile string
}

func (n *fixedSourceApprovalNamer) GetName() string {
	return n.name
}

func (n *fixedSourceApprovalNamer) GetReceivedFile(extWithDot string) string {
	return n.fileName("received", extWithDot)
}

func (n *fixedSourceApprovalNamer) GetApprovalFile(extWithDot string) string {
	return n.fileName("approved", extWithDot)
}

func (n *fixedSourceApprovalNamer) fileName(kind string, extWithDot string) string {
	testFileName := strings.TrimSuffix(filepath.Base(n.sourceFile), filepath.Ext(n.sourceFile))
	return filepath.Join(filepath.Dir(n.sourceFile), "approvals", testFileName+"."+n.name+"."+kind+extWithDot)
}
