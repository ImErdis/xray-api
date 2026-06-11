package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ImErdis/xray-api/internal/domain"
)

// errorBody is the JSON error envelope: {"error": {"code", "message"}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError maps domain errors to HTTP status codes and a JSON envelope.
func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusBadRequest, "validation_error"
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: err.Error()}})
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errorBody{
		Error: errorDetail{Code: "validation_error", Message: msg},
	})
}

// decodeJSON reads a JSON request body into dst, rejecting unknown fields.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
