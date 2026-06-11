package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.apiKeys.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
}

type createAPIKeyRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createAPIKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	created, err := s.apiKeys.Create(r.Context(), req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	// Plaintext key is shown exactly once.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   created.Key.ID,
		"name": created.Key.Name,
		"key":  created.Plaintext,
	})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if err := s.apiKeys.Revoke(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
