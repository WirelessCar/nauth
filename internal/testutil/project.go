package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	projectModule = "module github.com/WirelessCar/nauth"
)

func GetProjectBinaryAssetsDir() string {
	basePath := projectPath("bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

func GetProjectCRDDirectoryPaths() []string {
	return []string{projectPath("charts", "nauth", "resources", "crds")}
}

func projectPath(elem ...string) string {
	root, err := findProjectRoot()
	if err != nil {
		panic(fmt.Errorf("failed to find project root: %w", err))
	}
	return filepath.Join(append([]string{root}, elem...)...)
}

func findProjectRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		panic(fmt.Errorf("failed to determine working directory: %w", err))
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %q: %w", start, err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		content, err := os.ReadFile(goModPath)
		if err == nil && hasProjectModule(content) {
			return dir, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to read %s: %w", goModPath, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("failed to find %s from %s", projectModule, start)
		}
		dir = parent
	}
}

func hasProjectModule(goMod []byte) bool {
	for _, line := range strings.Split(string(goMod), "\n") {
		if strings.TrimSpace(line) == projectModule {
			return true
		}
	}
	return false
}
