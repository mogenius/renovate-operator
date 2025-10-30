package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"renovate-operator/config"
	"testing"

	"github.com/go-logr/logr"
)

func TestHandleCssStyles_DefaultFile(t *testing.T) {
	// Initialize config module
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "CUSTOM_CSS_FILE_PATH", Optional: true},
	})
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Create a temporary CSS file
	tempDir := t.TempDir()
	cssFile := filepath.Join(tempDir, "styles.css")
	cssContent := "body { color: red; }"
	err = os.WriteFile(cssFile, []byte(cssContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test CSS file: %v", err)
	}

	// Temporarily change working directory for the test
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Logf("failed to restore working dir: %v", err)
		}
	}()

	// Create static directory structure
	staticDir := filepath.Join(tempDir, "static", "css")
	err = os.MkdirAll(staticDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create static directory: %v", err)
	}

	defaultCssFile := filepath.Join(staticDir, "styles.css")
	err = os.WriteFile(defaultCssFile, []byte(cssContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create default CSS file: %v", err)
	}

	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	server := &Server{
		logger: logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/css/styles.css", nil)
	w := httptest.NewRecorder()

	server.handleCssStyles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/css" {
		t.Errorf("Expected Content-Type 'text/css', got '%s'", contentType)
	}

	if w.Body.String() != cssContent {
		t.Errorf("Expected body '%s', got '%s'", cssContent, w.Body.String())
	}
}

func TestHandleCssStyles_FileNotFound(t *testing.T) {
	// Initialize config module
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "CUSTOM_CSS_FILE_PATH", Optional: true},
	})
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	tempDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Logf("failed to restore working dir: %v", err)
		}
	}()

	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	server := &Server{
		logger: logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/css/styles.css", nil)
	w := httptest.NewRecorder()

	server.handleCssStyles(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleFavicon_DefaultFile(t *testing.T) {
	// Initialize config module
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "CUSTOM_FAVICON_FILE_PATH", Optional: true},
	})
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	// Create a temporary favicon file
	tempDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Logf("failed to restore working dir: %v", err)
		}
	}()

	// Create static directory structure
	staticDir := filepath.Join(tempDir, "static")
	err = os.MkdirAll(staticDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create static directory: %v", err)
	}

	defaultFaviconFile := filepath.Join(staticDir, "favicon.ico")
	faviconContent := []byte("fake favicon content")
	err = os.WriteFile(defaultFaviconFile, faviconContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create default favicon file: %v", err)
	}

	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	server := &Server{
		logger: logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	w := httptest.NewRecorder()

	server.handleFavicon(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "image/x-icon" {
		t.Errorf("Expected Content-Type 'image/x-icon', got '%s'", contentType)
	}

	if w.Body.String() != string(faviconContent) {
		t.Errorf("Expected body '%s', got '%s'", string(faviconContent), w.Body.String())
	}
}

func TestHandleFavicon_FileNotFound(t *testing.T) {
	// Initialize config module
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "CUSTOM_FAVICON_FILE_PATH", Optional: true},
	})
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}

	tempDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Logf("failed to restore working dir: %v", err)
		}
	}()

	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	server := &Server{
		logger: logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	w := httptest.NewRecorder()

	server.handleFavicon(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
