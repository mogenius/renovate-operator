package clientProvider

import (
	"os"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

var (
	cachedKubernetesClient    kubernetes.Interface
	cachedDynamicClient       dynamic.Interface
	cachedApiextensionsClient apiextensionsclient.Interface
	cachedDiscoveryClient     discovery.DiscoveryInterface
)

type K8sClientProvider interface {
	K8sClientSet() (kubernetes.Interface, error)
	DynamicClient() (dynamic.Interface, error)
	ApiExtensionsClient() (apiextensionsclient.Interface, error)
	DiscoveryClient() (discovery.DiscoveryInterface, error)
	RunsInCluster() bool
	ClientConfig() *rest.Config
}
type k8sClientProvider struct {
	clientConfig *rest.Config
}

func NewClientProvider() (K8sClientProvider, error) {
	kubeConfig, err := createKubernetesConfig()
	if err != nil {
		return nil, err
	}
	return k8sClientProvider{
		clientConfig: kubeConfig,
	}, nil
}
func (provider k8sClientProvider) ClientConfig() *rest.Config {
	return provider.clientConfig
}
func (provider k8sClientProvider) RunsInCluster() bool {
	return getKubernetesConfig() == ""
}

func (provider k8sClientProvider) K8sClientSet() (clientset kubernetes.Interface, err error) {
	if cachedKubernetesClient != nil {
		return cachedKubernetesClient, nil
	}

	clientset, err = kubernetes.NewForConfig(provider.clientConfig)
	if err == nil {
		cachedKubernetesClient = clientset
	}
	return cachedKubernetesClient, err
}

func (provider k8sClientProvider) DiscoveryClient() (discovery.DiscoveryInterface, error) {
	if cachedDiscoveryClient != nil {
		return cachedDiscoveryClient, nil
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(provider.clientConfig)
	if err == nil {
		cachedDiscoveryClient = discoveryClient
	}
	return cachedDiscoveryClient, err
}

func (provider k8sClientProvider) DynamicClient() (dynamicClient dynamic.Interface, err error) {
	if cachedDynamicClient != nil {
		return cachedDynamicClient, nil
	}

	dynamicClient, err = dynamic.NewForConfig(provider.clientConfig)
	if err == nil {
		cachedDynamicClient = dynamicClient
	}
	return cachedDynamicClient, err
}

func (provider k8sClientProvider) ApiExtensionsClient() (clientset apiextensionsclient.Interface, err error) {
	if cachedApiextensionsClient != nil {
		return cachedApiextensionsClient, nil
	}

	clientset, err = apiextensionsclient.NewForConfig(provider.clientConfig)
	if err == nil {
		cachedApiextensionsClient = clientset
	}
	return cachedApiextensionsClient, err
}

func createKubernetesConfig() (*rest.Config, error) {
	cfg := getKubernetesConfig()
	if cfg != "" {
		return clientcmd.BuildConfigFromFlags("", cfg)
	} else {
		return rest.InClusterConfig()
	}
}

func getKubernetesConfig() string {
	return os.Getenv("KUBECONFIG")
}
