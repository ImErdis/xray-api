package httpapi

import (
	"net/http"

	"github.com/ImErdis/xray-api/api"
	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/version"
)

// handleOpenAPI serves the embedded OpenAPI specification.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(api.OpenAPISpec)
}

// handleSystemStats returns a lightweight overview: node status counts and
// build info. User/plan counts are derived cheaply from list endpoints.
func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodes, err := s.nodes.List(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	byStatus := map[domain.NodeStatus]int{}
	for _, n := range nodes {
		byStatus[n.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": version.Version,
		"commit":  version.Commit,
		"nodes": map[string]any{
			"total":    len(nodes),
			"online":   byStatus[domain.NodeStatusOnline],
			"offline":  byStatus[domain.NodeStatusOffline],
			"unknown":  byStatus[domain.NodeStatusUnknown],
			"disabled": byStatus[domain.NodeStatusDisabled],
		},
	})
}
