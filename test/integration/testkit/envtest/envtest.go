package envtest

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	crenvtest "sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Environment struct {
	ctx     context.Context
	cancel  context.CancelFunc
	testEnv *crenvtest.Environment
	Config  *rest.Config
	Client  ctrlclient.Client
	Scheme  *k8sruntime.Scheme
}

func Start() (*Environment, error) {
	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	testEnv := &crenvtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(repoRoot(), "charts", "nauth", "resources", "crds")},
		ErrorIfCRDPathMissing: true,
	}

	if binaryAssetsDir := firstEnvtestBinaryDir(); binaryAssetsDir != "" {
		testEnv.BinaryAssetsDirectory = binaryAssetsDir
	}

	cfg, err := testEnv.Start()
	if err != nil {
		cancel()
		return nil, err
	}

	client, err := ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		cancel()
		_ = testEnv.Stop()
		return nil, err
	}

	return &Environment{
		ctx:     ctx,
		cancel:  cancel,
		testEnv: testEnv,
		Config:  cfg,
		Client:  client,
		Scheme:  scheme,
	}, nil
}

func (e *Environment) Stop() error {
	e.cancel()
	return e.testEnv.Stop()
}

func repoRoot() string {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		panic("failed to resolve envtest helper path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func firstEnvtestBinaryDir() string {
	basePath := filepath.Join(repoRoot(), "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
