package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	approvals "github.com/approvals/go-approval-tests"
	approvalscore "github.com/approvals/go-approval-tests/core"
	"github.com/stretchr/testify/suite"
)

func TestMain(m *testing.M) {
	approvals.UseFolder("approvals")
	os.Exit(m.Run())
}

type TestCaseInputFile struct {
	TestName  string
	InputFile string
}

func discoverTestCases(pattern string) []TestCaseInputFile {
	testCasePlaceholder := "{TestCase}"
	if !strings.Contains(pattern, testCasePlaceholder) {
		panic(fmt.Sprintf("pattern must contain %s placeholder: %s", testCasePlaceholder, pattern))
	}
	globPattern := strings.ReplaceAll(pattern, testCasePlaceholder, "*")
	files, err := filepath.Glob(globPattern)
	if err != nil {
		panic(fmt.Sprintf("unable to glob pattern %q: %s", globPattern, err.Error()))
	}
	testPattern := strings.ReplaceAll(pattern, testCasePlaceholder, "(?P<TestCase>.*)")
	regex := regexp.MustCompile(testPattern)
	var testCases []TestCaseInputFile
	for _, file := range files {
		if regex.MatchString(file) {
			match := regex.FindStringSubmatch(file)
			for i, name := range regex.SubexpNames() {
				if name == "TestCase" {
					testCases = append(testCases, TestCaseInputFile{
						TestName:  match[i],
						InputFile: file,
					})
				}
			}
		}
	}
	return testCases
}

func approvalOptionsForTestSuite(ts *suite.Suite) approvals.VerifyOptions {
	t := ts.T()
	t.Helper()

	_, file, _, ok := runtime.Caller(1)
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

var _ approvalscore.ApprovalNamer = (*fixedSourceApprovalNamer)(nil)
