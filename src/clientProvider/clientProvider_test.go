package clientProvider

import (
	"os"
	"path/filepath"
	"testing"
)

const testKubeConfig = `apiVersion: v1
kind: Config
current-context: test
clusters:
  - name: test
    cluster:
      server: https://kubernetes.test:6443
contexts:
  - name: test
    context:
      cluster: test
      user: test
users:
  - name: test
    user:
      token: test-token
`

// KUBECONFIG holds a precedence list, not a single file: a missing entry is
// skipped and the remaining files are merged, the way kubectl resolves them.
func TestCreateKubernetesConfigWithKubeConfigList(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "config")
	if err := os.WriteFile(existing, []byte(testKubeConfig), 0o600); err != nil {
		t.Fatalf("writing the kubeconfig: %v", err)
	}

	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing-cluster")+string(filepath.ListSeparator)+existing)

	config, inCluster, err := createKubernetesConfig()
	if err != nil {
		t.Fatalf("resolving the kubeconfig: %v", err)
	}
	if inCluster {
		t.Error("expected the resolution to report an out-of-cluster config")
	}
	if config.Host != "https://kubernetes.test:6443" {
		t.Errorf("host = %q, want the server of the merged kubeconfig", config.Host)
	}
}

func TestCreateKubernetesConfigWithUnusableKubeConfig(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))

	if _, _, err := createKubernetesConfig(); err == nil {
		t.Fatal("expected an error when KUBECONFIG names no usable file")
	}
}
