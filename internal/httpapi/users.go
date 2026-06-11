package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/service"
	"github.com/ImErdis/xray-api/internal/store"
)

// userResponse augments a user with its subscription URL.
type userResponse struct {
	*domain.User
	SubURL string `json:"sub_url"`
}

func (s *Server) userResponse(u *domain.User) userResponse {
	return userResponse{User: u, SubURL: s.users.SubURL(u)}
}

type createUserRequest struct {
	Email          string     `json:"email"`
	PlanID         *string    `json:"plan_id"`
	InboundIDs     []string   `json:"inbound_ids"`
	DataLimitBytes *int64     `json:"data_limit_bytes"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Note           string     `json:"note"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	u, err := s.users.Create(r.Context(), service.CreateInput{
		Email:          req.Email,
		PlanID:         req.PlanID,
		InboundIDs:     req.InboundIDs,
		DataLimitBytes: req.DataLimitBytes,
		ExpiresAt:      req.ExpiresAt,
		Note:           req.Note,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.userResponse(u))
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}
	users, total, err := s.users.List(r.Context(), store.UserFilter{
		Status: q.Get("status"),
		PlanID: q.Get("plan_id"),
		Query:  q.Get("q"),
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, s.userResponse(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users": resp, "total": total, "page": page, "per_page": perPage,
	})
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	u, err := s.users.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userResponse(u))
}

type updateUserRequest struct {
	PlanID         *string    `json:"plan_id"`
	DataLimitBytes *int64     `json:"data_limit_bytes"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ClearExpiry    bool       `json:"clear_expiry"`
	Note           *string    `json:"note"`
	Status         *string    `json:"status"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	in := service.UpdateInput{
		PlanID:         req.PlanID,
		DataLimitBytes: req.DataLimitBytes,
		ExpiresAt:      req.ExpiresAt,
		ClearExpiry:    req.ClearExpiry,
		Note:           req.Note,
	}
	if req.Status != nil {
		st := domain.UserStatus(*req.Status)
		in.Status = &st
	}
	u, err := s.users.Update(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userResponse(u))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.users.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setInboundsRequest struct {
	InboundIDs []string `json:"inbound_ids"`
}

func (s *Server) handleSetUserInbounds(w http.ResponseWriter, r *http.Request) {
	var req setInboundsRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	u, err := s.users.SetInbounds(r.Context(), chi.URLParam(r, "id"), req.InboundIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userResponse(u))
}

func (s *Server) handleSuspendUser(w http.ResponseWriter, r *http.Request) {
	s.userAction(w, r, s.users.Suspend)
}

func (s *Server) handleResumeUser(w http.ResponseWriter, r *http.Request) {
	s.userAction(w, r, s.users.Resume)
}

func (s *Server) handleResetTraffic(w http.ResponseWriter, r *http.Request) {
	s.userAction(w, r, s.users.ResetTraffic)
}

type renewUserRequest struct {
	Days         int     `json:"days"`
	ResetTraffic bool    `json:"reset_traffic"`
	PlanID       *string `json:"plan_id"`
}

// handleRenewUser is the billing-cycle endpoint: extend expiry, optionally
// reset traffic, switch plans, and reactivate — atomically.
func (s *Server) handleRenewUser(w http.ResponseWriter, r *http.Request) {
	var req renewUserRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	u, err := s.users.Renew(r.Context(), chi.URLParam(r, "id"), service.RenewInput{
		Days:         req.Days,
		ResetTraffic: req.ResetTraffic,
		PlanID:       req.PlanID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userResponse(u))
}

func (s *Server) handleRotateSubToken(w http.ResponseWriter, r *http.Request) {
	s.userAction(w, r, s.users.RotateSubToken)
}

// userAction runs a (ctx,id)->(*User,error) service method and renders it,
// keeping the simple action handlers uniform.
func (s *Server) userAction(w http.ResponseWriter, r *http.Request,
	fn func(ctx context.Context, id string) (*domain.User, error)) {
	u, err := fn(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userResponse(u))
}

func (s *Server) handleUserUsage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		} else {
			badRequest(w, "invalid from (want RFC3339)")
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		} else {
			badRequest(w, "invalid to (want RFC3339)")
			return
		}
	}
	groupBy := q.Get("group_by")
	if groupBy != "node" {
		groupBy = "day"
	}
	buckets, err := s.users.Usage(r.Context(), chi.URLParam(r, "id"), from, to, groupBy)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "group_by": groupBy, "buckets": buckets,
	})
}
