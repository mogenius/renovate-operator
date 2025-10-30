package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		httpError  HttpResultError
		wantStatus int
	}{
		{
			name: "internal server error",
			httpError: HttpResultError{
				Message:    "Something went wrong",
				StatusCode: http.StatusInternalServerError,
				Error:      errors.New("internal error"),
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "bad request error",
			httpError: HttpResultError{
				Message:    "Invalid input",
				StatusCode: http.StatusBadRequest,
				Error:      errors.New("validation failed"),
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found error",
			httpError: HttpResultError{
				Message:    "Resource not found",
				StatusCode: http.StatusNotFound,
				Error:      errors.New("not found"),
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			writeError(w, tt.httpError)

			if w.Code != tt.wantStatus {
				t.Errorf("writeError() status = %v, want %v", w.Code, tt.wantStatus)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("writeError() Content-Type = %v, want application/json", contentType)
			}

			// Decode into a map to avoid error field deserialization issues
			var result map[string]interface{}
			err := json.NewDecoder(w.Body).Decode(&result)
			if err != nil {
				t.Fatalf("Failed to decode response body: %v", err)
			}

			if result["Message"] != tt.httpError.Message {
				t.Errorf("writeError() message = %v, want %v", result["Message"], tt.httpError.Message)
			}
		})
	}
}

func TestInternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	err := errors.New("test error")
	message := "Internal server error occurred"

	internalServerError(w, err, message)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("internalServerError() status = %v, want %v", w.Code, http.StatusInternalServerError)
	}

	var result map[string]interface{}
	decodeErr := json.NewDecoder(w.Body).Decode(&result)
	if decodeErr != nil {
		t.Fatalf("Failed to decode response body: %v", decodeErr)
	}

	if result["Message"] != message {
		t.Errorf("internalServerError() message = %v, want %v", result["Message"], message)
	}

	if int(result["StatusCode"].(float64)) != http.StatusInternalServerError {
		t.Errorf("internalServerError() StatusCode = %v, want %v", result["StatusCode"], http.StatusInternalServerError)
	}
}

func TestBadRequestError(t *testing.T) {
	w := httptest.NewRecorder()
	err := errors.New("validation error")
	message := "Invalid request data"

	badRequestError(w, err, message)

	if w.Code != http.StatusBadRequest {
		t.Errorf("badRequestError() status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	var result map[string]interface{}
	decodeErr := json.NewDecoder(w.Body).Decode(&result)
	if decodeErr != nil {
		t.Fatalf("Failed to decode response body: %v", decodeErr)
	}

	if result["Message"] != message {
		t.Errorf("badRequestError() message = %v, want %v", result["Message"], message)
	}

	if int(result["StatusCode"].(float64)) != http.StatusBadRequest {
		t.Errorf("badRequestError() StatusCode = %v, want %v", result["StatusCode"], http.StatusBadRequest)
	}
}
