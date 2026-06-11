package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/service"
)

type nodeRequest struct {
	Name             string `json:"name"`
	APIAddress       string `json:"api_address"`
	APIPort          int    `json:"api_port"`
	APITLS           bool   `json:"api_tls"`
	APITLSServerName string `json:"api_tls_server_name"`
	APITLSInsecure   bool   `json:"api_tls_insecure"`
	// mTLS PEM material. Omit to leave unchanged on update; send "" to clear.
	APICACert     *string `json:"api_ca_cert"`
	APIClientCert *string `json:"api_client_cert"`
	APIClientKey  *string `json:"api_client_key"`
}

func (req nodeRequest) toInput() service.NodeInput {
	return service.NodeInput{
		Name:             req.Name,
		APIAddress:       req.APIAddress,
		APIPort:          req.APIPort,
		APITLS:           req.APITLS,
		APITLSServerName: req.APITLSServerName,
		APITLSInsecure:   req.APITLSInsecure,
		APICACert:        req.APICACert,
		APIClientCert:    req.APIClientCert,
		APIClientKey:     req.APIClientKey,
	}
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.nodes.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var req nodeRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	n, err := s.nodes.Create(r.Context(), req.toInput())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	n, err := s.nodes.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	var req nodeRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	n, err := s.nodes.Update(r.Context(), chi.URLParam(r, "id"), req.toInput())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	if err := s.nodes.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReconcileNode(w http.ResponseWriter, r *http.Request) {
	if err := s.nodes.Reconcile(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reconcile_triggered"})
}

// --- Inbounds ---

type inboundRequest struct {
	Tag              string          `json:"tag"`
	Protocol         domain.Protocol `json:"protocol"`
	ListenPort       int             `json:"listen_port"`
	PublicHost       string          `json:"public_host"`
	PublicPort       int             `json:"public_port"`
	Network          domain.Network  `json:"network"`
	Security         domain.Security `json:"security"`
	WSPath           string          `json:"ws_path"`
	HostHeader       string          `json:"host_header"`
	GRPCServiceName  string          `json:"grpc_service_name"`
	SNI              string          `json:"sni"`
	Fingerprint      string          `json:"fingerprint"`
	RealityPublicKey string          `json:"reality_public_key"`
	RealityShortID   string          `json:"reality_short_id"`
	Flow             string          `json:"flow"`
	Method           string          `json:"method"`
	XHTTPMode        string          `json:"xhttp_mode"`
	Remark           string          `json:"remark"`
}

func (req inboundRequest) toInput() service.InboundInput {
	return service.InboundInput{
		Tag:              req.Tag,
		Protocol:         req.Protocol,
		ListenPort:       req.ListenPort,
		PublicHost:       req.PublicHost,
		PublicPort:       req.PublicPort,
		Network:          req.Network,
		Security:         req.Security,
		WSPath:           req.WSPath,
		HostHeader:       req.HostHeader,
		GRPCServiceName:  req.GRPCServiceName,
		SNI:              req.SNI,
		Fingerprint:      req.Fingerprint,
		RealityPublicKey: req.RealityPublicKey,
		RealityShortID:   req.RealityShortID,
		Flow:             req.Flow,
		Method:           req.Method,
		XHTTPMode:        req.XHTTPMode,
		Remark:           req.Remark,
	}
}

func (s *Server) handleListInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds, err := s.nodes.ListInbounds(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inbounds": inbounds})
}

func (s *Server) handleCreateInbound(w http.ResponseWriter, r *http.Request) {
	var req inboundRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	ib, err := s.nodes.CreateInbound(r.Context(), chi.URLParam(r, "id"), req.toInput())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ib)
}

func (s *Server) handleGetInbound(w http.ResponseWriter, r *http.Request) {
	ib, err := s.nodes.GetInbound(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ib)
}

func (s *Server) handleUpdateInbound(w http.ResponseWriter, r *http.Request) {
	var req inboundRequest
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err.Error())
		return
	}
	ib, err := s.nodes.UpdateInbound(r.Context(), chi.URLParam(r, "id"), req.toInput())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ib)
}

func (s *Server) handleDeleteInbound(w http.ResponseWriter, r *http.Request) {
	if err := s.nodes.DeleteInbound(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
