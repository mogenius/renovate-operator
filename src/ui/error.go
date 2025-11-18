package ui

import (
	"encoding/json"
	"net/http"
)

func writeError(w http.ResponseWriter, err HttpResultError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)
	_ = json.NewEncoder(w).Encode(err)
}

type SuccessResult struct {
	Message string `json:"message"`
}

func writeSuccess(w http.ResponseWriter, result SuccessResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

type HttpResultError struct {
	Message    string
	StatusCode int
	Error      error
}

func internalServerError(w http.ResponseWriter, err error, message string) {
	writeError(w, HttpResultError{
		Message:    message,
		StatusCode: http.StatusInternalServerError,
		Error:      err,
	})
}
func badRequestError(w http.ResponseWriter, err error, message string) {
	writeError(w, HttpResultError{
		Message:    message,
		StatusCode: http.StatusBadRequest,
		Error:      err,
	})
}
