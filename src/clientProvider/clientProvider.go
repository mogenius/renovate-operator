package clientProvider

import (
	"errors"
	"fmt"
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
	inCluster    bool
}

func NewClientProvider() (K8sClientProvider, error) {
	kubeConfig, inCluster, err := createKubernetesConfig()
	if err != nil {
		return nil, err
	}
	return k8sClientProvider{
		clientConfig: kubeConfig,
		inCluster:    inCluster,
	}, nil
}
func (provider k8sClientProvider) ClientConfig() *rest.Config {
	return provider.clientConfig
}
func (provider k8sClientProvider) RunsInCluster() bool {
	return provider.inCluster
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

// createKubernetesConfig resolves the client configuration: an explicit
// KUBECONFIG wins, then the in-cluster service account, then the default
// kubeconfig at ~/.kube/config — so a local run works without exporting
// KUBECONFIG, matching ctrl.GetConfigOrDie in main.go. The bool reports
// whether the in-cluster configuration was used.
func createKubernetesConfig() (*rest.Config, bool, error) {
	if os.Getenv(clientcmd.RecommendedConfigPathEnvVar) != "" {
		config, err := loadKubeConfig()
		if err != nil {
			return nil, false, fmt.Errorf("unable to load the kubeconfig from %s=%s: %w",
				clientcmd.RecommendedConfigPathEnvVar, os.Getenv(clientcmd.RecommendedConfigPathEnvVar), err)
		}
		return config, false, nil
	}

	config, err := rest.InClusterConfig()
	if err == nil {
		return config, true, nil
	}
	// Any other error means we ARE in a cluster with a broken service account
	// setup; falling back to a kubeconfig would mask that.
	if !errors.Is(err, rest.ErrNotInCluster) {
		return nil, false, err
	}

	config, err = loadKubeConfig()
	if err != nil {
		return nil, false, fmt.Errorf("not running in a cluster and no usable kubeconfig found at %s (set KUBECONFIG to use another path): %w",
			clientcmd.RecommendedHomeFile, err)
	}
	return config, false, nil
}

// loadKubeConfig defers to client-go's own loading rules instead of reading a
// single file: KUBECONFIG may name a whole precedence list of files separated
// by filepath.ListSeparator, which the rules merge the way kubectl does, and
// they fall back to ~/.kube/config when the variable is unset.
//
// Deliberately not NewNonInteractiveDeferredLoadingClientConfig: that
// substitutes the in-cluster configuration whenever the files turn out to be
// unusable, which would both hide a broken kubeconfig behind a working client
// and report the result as out-of-cluster when it is anything but. Loading the
// rules directly keeps an unusable kubeconfig an error, and the caller decides
// what the in-cluster case means.
func loadKubeConfig() (*rest.Config, error) {
	apiConfig, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return nil, err
	}
	return clientcmd.NewDefaultClientConfig(*apiConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
}
