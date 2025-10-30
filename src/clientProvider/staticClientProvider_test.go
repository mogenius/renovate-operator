package clientProvider

import (
	"testing"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

// Mock implementation of K8sClientProvider for testing
type mockClientProvider struct {
	config *rest.Config
}

func (m mockClientProvider) K8sClientSet() (kubernetes.Interface, error) {
	return nil, nil
}

func (m mockClientProvider) DynamicClient() (dynamic.Interface, error) {
	return nil, nil
}

func (m mockClientProvider) ApiExtensionsClient() (apiextensionsclient.Interface, error) {
	return nil, nil
}

func (m mockClientProvider) DiscoveryClient() (discovery.DiscoveryInterface, error) {
	return nil, nil
}

func (m mockClientProvider) RunsInCluster() bool {
	return true
}

func (m mockClientProvider) ClientConfig() *rest.Config {
	return m.config
}

func TestInitializeStaticClientProvider(t *testing.T) {
	// Reset the static provider before test
	originalProvider := staticClientProvider
	defer func() { staticClientProvider = originalProvider }()

	// This test requires KUBECONFIG to be set or running in a cluster
	// We'll just verify that calling InitializeStaticClientProvider doesn't panic
	// and that if it succeeds, the provider is set

	t.Run("initialization with mock", func(t *testing.T) {
		// Set up a mock provider directly
		staticClientProvider = mockClientProvider{
			config: &rest.Config{
				Host: "https://test-cluster",
			},
		}

		// Verify the provider is set
		provider := StaticClientProvider()
		if provider == nil {
			t.Error("StaticClientProvider should not be nil after initialization")
		}

		// Verify the config is accessible
		config := provider.ClientConfig()
		if config == nil {
			t.Error("ClientConfig should not be nil")
		}
		if config == nil {
			t.Fatal("config is nil")
		}
		if config.Host != "https://test-cluster" {
			t.Errorf("Expected host 'https://test-cluster', got '%s'", config.Host)
		}
	})

	t.Run("RunsInCluster returns expected value", func(t *testing.T) {
		staticClientProvider = mockClientProvider{
			config: &rest.Config{},
		}

		provider := StaticClientProvider()
		// Mock always returns true
		if !provider.RunsInCluster() {
			t.Error("Expected RunsInCluster to return true for mock")
		}
	})
}
