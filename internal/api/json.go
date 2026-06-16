package api

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error Error `json:"error"`
}
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func WriteError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	WriteJSON(w, status, ErrorBody{Error: Error{Code: code, Message: message, Retryable: retryable}})
}
