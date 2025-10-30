package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"renovate-operator/health"
	"testing"

	"github.com/go-logr/logr"
)

func TestHandleHealthCheck(t *testing.T) {
	// Create a health check instance
	h := health.NewHealthCheck()

	// Set some health status
	h.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		e.Running = true
		return e
	})

	// Create server with mock dependencies
	server := &Server{
		health: h,
		logger: logr.Discard(),
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	server.handleHealthCheck(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("handleHealthCheck() status = %v, want %v", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("handleHealthCheck() Content-Type = %v, want application/json", contentType)
	}

	// Decode response
	var result health.ApplicationHealth
	err := json.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	// Verify health status content
	if !result.Scheduler.Running {
		t.Error("Expected scheduler to be running")
	}
}

func TestHandleHealthCheck_EmptyHealth(t *testing.T) {
	// Create a health check instance with no modifications
	h := health.NewHealthCheck()

	server := &Server{
		health: h,
		logger: logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealthCheck(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleHealthCheck() status = %v, want %v", w.Code, http.StatusOK)
	}

	var result health.ApplicationHealth
	err := json.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	// Should return default health status
	if result.Scheduler.Running {
		t.Error("Expected scheduler to not be running in default state")
	}
}
