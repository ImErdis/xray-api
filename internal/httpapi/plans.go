package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ImErdis/xray-api/internal/service"
)

type planRequest struct {
	Name           string `json:"name"`
	DataLimitBytes *int64 `json:"data_limit_bytes"`
	DurationDays   *int   `json:"duration_days"`
}

func (req planRequest) toInput() service.PlanInput {
	return service.PlanInput{
		Name:           req.Name,
		DataLimitBytes: req.DataLimitBytes,
		DurationDays:   req.DurationDays,
	}
}

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.plans.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	var req planRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	p, err := s.plans.Create(r.Context(), req.toInput())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.plans.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	var req planRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	p, err := s.plans.Update(r.Context(), chi.URLParam(r, "id"), req.toInput())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	if err := s.plans.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
