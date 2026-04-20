package scenariotest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

type TestCaseFile struct {
	TestName  string
	InputFile string
}

type Scenario struct {
	Config  Config                   `yaml:"config"`
	Objects []map[string]interface{} `yaml:"resources"`
	Collect []ObjectRef              `yaml:"collect"`
}

type Config struct {
	OperatorNamespace string                `yaml:"operatorNamespace"`
	OperatorVersion   string                `yaml:"operatorVersion"`
	OperatorCluster   OperatorClusterConfig `yaml:"operatorCluster"`
}

type OperatorClusterConfig struct {
	Namespace string `yaml:"namespace"`
	Name      string `yaml:"name"`
	Optional  bool   `yaml:"optional"`
}

type ObjectRef struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Namespace  string `yaml:"namespace"`
	Name       string `yaml:"name"`
}

func Discover(pattern string) []TestCaseFile {
	testCasePlaceholder := "{TestCase}"
	if !strings.Contains(pattern, testCasePlaceholder) {
		panic(fmt.Sprintf("pattern must contain %s placeholder: %s", testCasePlaceholder, pattern))
	}

	callerDir := callerDir()
	globPattern := filepath.Join(callerDir, strings.ReplaceAll(pattern, testCasePlaceholder, "*"))
	files, err := filepath.Glob(globPattern)
	if err != nil {
		panic(fmt.Sprintf("unable to glob pattern %q: %s", globPattern, err.Error()))
	}

	testPattern := regexp.MustCompile(regexp.QuoteMeta(filepath.Join(callerDir, pattern)))
	testPattern = regexp.MustCompile(strings.ReplaceAll(testPattern.String(), regexp.QuoteMeta(testCasePlaceholder), "(?P<TestCase>.*)"))

	var testCases []TestCaseFile
	for _, file := range files {
		if !testPattern.MatchString(file) {
			continue
		}
		match := testPattern.FindStringSubmatch(file)
		for i, name := range testPattern.SubexpNames() {
			if name == "TestCase" {
				testCases = append(testCases, TestCaseFile{
					TestName:  match[i],
					InputFile: file,
				})
			}
		}
	}

	return testCases
}

func Load(filePath string) (*Scenario, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var scenario Scenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		return nil, err
	}
	return &scenario, nil
}

func DecodeObjects(rawObjects []map[string]interface{}) ([]*unstructured.Unstructured, error) {
	data, err := yaml.Marshal(map[string]interface{}{"resources": rawObjects})
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Resources []map[string]interface{} `yaml:"resources"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	result := make([]*unstructured.Unstructured, 0, len(wrapper.Resources))
	for _, object := range wrapper.Resources {
		un := &unstructured.Unstructured{Object: object}
		result = append(result, un)
	}
	return result, nil
}

func DecodeYAMLDocuments(data []byte) ([]*unstructured.Unstructured, error) {
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var result []*unstructured.Unstructured
	for {
		obj := map[string]interface{}{}
		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(obj) == 0 {
			continue
		}
		result = append(result, &unstructured.Unstructured{Object: obj})
	}
	return result, nil
}

func callerDir() string {
	_, file, _, ok := runtime.Caller(2)
	if !ok {
		panic("failed to resolve scenario caller path")
	}
	return filepath.Dir(file)
}
