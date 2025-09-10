package clientProvider

import "renovate-operator/assert"

var staticClientProvider K8sClientProvider

// Initiaalize a static global client provider
func InitializeStaticClientProvider() error {
	provider, err := NewClientProvider()
	if err != nil {
		return err
	}
	staticClientProvider = provider
	return nil
}

// Retrieve the static global client provider
func StaticClientProvider() K8sClientProvider {
	assert.Assert(staticClientProvider != nil, "StaticClientProvider must be initialized before usage")
	return staticClientProvider
}
