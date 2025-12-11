package github

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockRoundTripper allows mocking HTTP responses
type mockRoundTripper struct {
	responseFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.responseFunc(req)
}

// generateTestRSAKey generates an RSA private key for testing
func generateTestRSAKey() (*rsa.PrivateKey, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", err
	}

	// Convert to PKCS1 PEM format
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return privateKey, string(privateKeyPEM), nil
}

func TestCreateGithubAppToken_Success(t *testing.T) {
	_, pemString, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Create mock HTTP client
	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			responseFunc: func(req *http.Request) (*http.Response, error) {
				// Verify request
				if req.Method != "POST" {
					t.Errorf("Expected POST request, got %s", req.Method)
				}
				if req.Header.Get("Accept") != "application/vnd.github+json" {
					t.Errorf("Expected GitHub API accept header")
				}
				if req.Header.Get("Authorization") == "" {
					t.Errorf("Expected Authorization header")
				}

				// Return successful response
				tokenResponse := map[string]string{"token": "test-github-token-123"}
				body, _ := json.Marshal(tokenResponse)
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreatorWithHTTPClient(fakeClient, mockClient)

	token, err := tokenCreator.CreateGithubAppToken("123456", "78910", pemString, "https://api.github.com")
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if token != "test-github-token-123" {
		t.Errorf("Expected token 'test-github-token-123', got %q", token)
	}
}

func TestCreateGithubAppToken_GitHubAPIError(t *testing.T) {
	_, pemString, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Create mock HTTP client that returns error
	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			responseFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 401,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"message":"Bad credentials"}`))),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreatorWithHTTPClient(fakeClient, mockClient)

	_, err = tokenCreator.CreateGithubAppToken("123456", "78910", pemString, "https://api.github.com")
	if err == nil {
		t.Error("Expected error for 401 response")
	}
	if err != nil && err.Error() != `GitHub error: {"message":"Bad credentials"}` {
		t.Errorf("Expected GitHub error message, got: %v", err)
	}
}

func TestCreateGithubAppToken_InvalidPEM(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreator(fakeClient)

	tests := []struct {
		name      string
		appID     string
		installID string
		pemString string
		wantError string
	}{
		{
			name:      "empty PEM",
			appID:     "123",
			installID: "456",
			pemString: "",
			wantError: "PEM data does not start with BEGIN marker",
		},
		{
			name:      "invalid PEM format",
			appID:     "123",
			installID: "456",
			pemString: "not-a-pem-string",
			wantError: "PEM data does not start with BEGIN marker",
		},
		{
			name:      "PEM without proper structure",
			appID:     "123",
			installID: "456",
			pemString: "-----BEGIN RSA PRIVATE KEY-----\ninvalid-data\n-----END RSA PRIVATE KEY-----",
			wantError: "failed to parse PEM block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tokenCreator.CreateGithubAppToken(tt.appID, tt.installID, tt.pemString, "https://api.github.com")
			if err == nil {
				t.Error("Expected error but got none")
				return
			}
			if tt.wantError != "" && err.Error()[:len(tt.wantError)] != tt.wantError {
				t.Errorf("Expected error containing %q, got %q", tt.wantError, err.Error())
			}
		})
	}
}

func TestCreateGithubAppToken_PKCS8Format(t *testing.T) {
	// Generate key and convert to PKCS8 format
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Convert to PKCS8 PEM format
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal PKCS8: %v", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Create mock HTTP client
	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			responseFunc: func(req *http.Request) (*http.Response, error) {
				tokenResponse := map[string]string{"token": "pkcs8-test-token"}
				body, _ := json.Marshal(tokenResponse)
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreatorWithHTTPClient(fakeClient, mockClient)

	// Should handle PKCS8 format correctly
	token, err := tokenCreator.CreateGithubAppToken("123456", "78910", string(privateKeyPEM), "https://api.github.com")
	if err != nil {
		t.Errorf("PKCS8 format should be supported, got error: %v", err)
	}
	if token != "pkcs8-test-token" {
		t.Errorf("Expected token 'pkcs8-test-token', got %q", token)
	}
}

func TestCreateGithubAppTokenFromJob_NoReference(t *testing.T) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	err = api.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add api to scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreator(fakeClient)

	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: api.RenovateJobSpec{
			GithubAppReference: nil,
		},
	}

	_, err = tokenCreator.CreateGithubAppTokenFromJob(job)
	if err == nil {
		t.Error("Expected error when GithubAppReference is nil")
	}
	if err.Error() != "github App Reference needs to be defined" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestCreateGithubAppTokenFromJob_SecretNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	err = api.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add api to scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreator(fakeClient)

	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: api.RenovateJobSpec{
			GithubAppReference: &api.GithubAppReference{
				SecretName:              "github-app-secret",
				AppIdSecretKey:          "app-id",
				InstallationIdSecretKey: "installation-id",
				PemSecretKey:            "private-key",
			},
		},
	}

	_, err = tokenCreator.CreateGithubAppTokenFromJob(job)
	if err == nil {
		t.Error("Expected error when secret doesn't exist")
	}
}

func TestCreateGithubAppTokenFromJob_MissingSecretKeys(t *testing.T) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	err = api.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add api to scheme: %v", err)
	}

	tests := []struct {
		name       string
		secretData map[string][]byte
		wantError  string
	}{
		{
			name: "missing PEM key",
			secretData: map[string][]byte{
				"app-id":          []byte("12345"),
				"installation-id": []byte("67890"),
			},
			wantError: "the given key does not exist in the secret: key=private-key",
		},
		{
			name: "missing app ID key",
			secretData: map[string][]byte{
				"private-key":     []byte("fake-pem"),
				"installation-id": []byte("67890"),
			},
			wantError: "the given key does not exist in the secret: key=app-id",
		},
		{
			name: "missing installation ID key",
			secretData: map[string][]byte{
				"private-key": []byte("fake-pem"),
				"app-id":      []byte("12345"),
			},
			wantError: "the given key does not exist in the secret: key=installation-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "github-app-secret",
					Namespace: "default",
				},
				Data: tt.secretData,
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(secret).
				Build()

			tokenCreator := NewGitHubAppTokenCreator(fakeClient)

			job := &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: api.RenovateJobSpec{
					GithubAppReference: &api.GithubAppReference{
						SecretName:              "github-app-secret",
						AppIdSecretKey:          "app-id",
						InstallationIdSecretKey: "installation-id",
						PemSecretKey:            "private-key",
					},
				},
			}

			_, err := tokenCreator.CreateGithubAppTokenFromJob(job)
			if err == nil {
				t.Error("Expected error for missing secret key")
			}
			if tt.wantError != "" && err.Error()[:len(tt.wantError)] != tt.wantError {
				t.Errorf("Expected error containing %q, got %q", tt.wantError, err.Error())
			}
		})
	}
}

func TestCreateGithubAppTokenFromJob_WithValidSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	err = api.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add api to scheme: %v", err)
	}

	_, pemString, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "github-app-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"app-id":          []byte("123456"),
			"installation-id": []byte("78910"),
			"private-key":     []byte(pemString),
		},
	}

	// Create mock HTTP client
	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			responseFunc: func(req *http.Request) (*http.Response, error) {
				tokenResponse := map[string]string{"token": "ghs_validTokenFromSecret"}
				body, _ := json.Marshal(tokenResponse)
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	tokenCreator := NewGitHubAppTokenCreatorWithHTTPClient(fakeClient, mockClient)

	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: api.RenovateJobSpec{
			GithubAppReference: &api.GithubAppReference{
				SecretName:              "github-app-secret",
				AppIdSecretKey:          "app-id",
				InstallationIdSecretKey: "installation-id",
				PemSecretKey:            "private-key",
			},
		},
	}

	token, err := tokenCreator.CreateGithubAppTokenFromJob(job)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if token != "ghs_validTokenFromSecret" {
		t.Errorf("Expected token 'ghs_validTokenFromSecret', got %q", token)
	}
}

func TestCreateGithubAppToken_WithWhitespace(t *testing.T) {
	_, pemString, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Add whitespace to PEM
	pemWithWhitespace := fmt.Sprintf("  \n%s\n  ", pemString)

	// Create mock HTTP client
	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			responseFunc: func(req *http.Request) (*http.Response, error) {
				tokenResponse := map[string]string{"token": "whitespace-test-token"}
				body, _ := json.Marshal(tokenResponse)
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	tokenCreator := NewGitHubAppTokenCreatorWithHTTPClient(fakeClient, mockClient)

	// Should handle whitespace correctly
	token, err := tokenCreator.CreateGithubAppToken("123456", "78910", pemWithWhitespace, "https://api.github.com")
	if err != nil {
		t.Errorf("Should handle PEM with whitespace, got error: %v", err)
	}
	if token != "whitespace-test-token" {
		t.Errorf("Expected token 'whitespace-test-token', got %q", token)
	}
}
