package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

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
		// Headers are already sent, so the status can't change; a failure
		// here is almost always the client hanging up mid-response.
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Debug("write response body", "err", err)
		}
	}
}

// writeBody writes a raw response body after headers are sent, logging (but
// not failing on) short writes — typically the client hanging up.
func writeBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		slog.Debug("write response body", "err", err)
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

// queryInt parses an integer query parameter, returning def when absent and
// an error when present but not a number.
func queryInt(q url.Values, name string, def int) (int, error) {
	v := q.Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q is not an integer", name, v)
	}
	return n, nil
}
