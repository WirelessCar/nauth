package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	approvals "github.com/approvals/go-approval-tests"
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
