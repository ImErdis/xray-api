package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/worker"
	"github.com/ImErdis/xray-api/internal/xray"
)

// NodeService manages nodes and their inbounds and keeps the worker manager in
// sync as nodes come and go.
type NodeService struct {
	store *store.Store
	mgr   *worker.Manager
	dial  worker.DialFunc
}

func NewNodeService(st *store.Store, mgr *worker.Manager, dial worker.DialFunc) *NodeService {
	if dial == nil {
		dial = worker.DefaultDial
	}
	return &NodeService{store: st, mgr: mgr, dial: dial}
}

// NodeInput is the create/update payload for a node.
type NodeInput struct {
	Name             string
	APIAddress       string
	APIPort          int
	APITLS           bool
	APITLSServerName string
	APITLSInsecure   bool

	// mTLS PEM material. Pointer semantics: nil leaves the stored value
	// unchanged (so a PATCH never wipes the write-only client key); a non-nil
	// empty string clears it.
	APICACert     *string
	APIClientCert *string
	APIClientKey  *string
}

// applyTLSMaterial applies the optional cert/CA fields and validates them.
func (in NodeInput) applyTLSMaterial(n *domain.Node) error {
	if in.APICACert != nil {
		n.APICACert = *in.APICACert
	}
	if in.APIClientCert != nil {
		n.APIClientCert = *in.APIClientCert
	}
	if in.APIClientKey != nil {
		n.APIClientKey = *in.APIClientKey
	}
	if err := xray.ValidateTLSMaterial(n.APICACert, n.APIClientCert, n.APIClientKey); err != nil {
		return domain.Validationf(err.Error())
	}
	n.APIHasClientKey = n.APIClientKey != ""
	return nil
}

func (s *NodeService) Create(ctx context.Context, in NodeInput) (*domain.Node, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, domain.Validationf("name is required")
	}
	if in.APIAddress == "" {
		return nil, domain.Validationf("api_address is required")
	}
	if in.APIPort <= 0 || in.APIPort > 65535 {
		return nil, domain.Validationf("api_port must be 1-65535")
	}
	n := &domain.Node{
		ID:               uuid.NewString(),
		Name:             in.Name,
		APIAddress:       in.APIAddress,
		APIPort:          in.APIPort,
		APITLS:           in.APITLS,
		APITLSServerName: in.APITLSServerName,
		APITLSInsecure:   in.APITLSInsecure,
		Status:           domain.NodeStatusUnknown,
	}
	if err := in.applyTLSMaterial(n); err != nil {
		return nil, err
	}
	if err := s.store.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	// Best-effort connectivity probe so the create response reflects reality.
	n.Status, n.LastError = s.probe(ctx, n)
	if n.Status == domain.NodeStatusOnline {
		now := time.Now()
		n.LastSeenAt = &now
	}
	_ = s.store.SetNodeHealth(ctx, n.ID, n.Status, n.LastError, n.LastSeenAt)

	s.mgr.StartNode(n)
	return s.store.GetNode(ctx, n.ID)
}

// probe dials the node and pings it once, returning the resulting status.
func (s *NodeService) probe(ctx context.Context, n *domain.Node) (domain.NodeStatus, string) {
	c, err := s.dial(n)
	if err != nil {
		return domain.NodeStatusOffline, err.Error()
	}
	defer c.Close()
	pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if _, err := c.Ping(pctx); err != nil {
		return domain.NodeStatusOffline, err.Error()
	}
	return domain.NodeStatusOnline, ""
}

func (s *NodeService) Get(ctx context.Context, id string) (*domain.Node, error) {
	return s.store.GetNode(ctx, id)
}

func (s *NodeService) List(ctx context.Context) ([]*domain.Node, error) {
	return s.store.ListNodes(ctx)
}

func (s *NodeService) Update(ctx context.Context, id string, in NodeInput) (*domain.Node, error) {
	n, err := s.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		n.Name = in.Name
	}
	if in.APIAddress != "" {
		n.APIAddress = in.APIAddress
	}
	if in.APIPort != 0 {
		n.APIPort = in.APIPort
	}
	n.APITLS = in.APITLS
	n.APITLSServerName = in.APITLSServerName
	n.APITLSInsecure = in.APITLSInsecure
	if err := in.applyTLSMaterial(n); err != nil {
		return nil, err
	}
	if err := s.store.UpdateNode(ctx, n); err != nil {
		return nil, err
	}
	// Reconnect with new settings.
	s.mgr.StartNode(n)
	return s.store.GetNode(ctx, id)
}

func (s *NodeService) Delete(ctx context.Context, id string) error {
	s.mgr.StopNode(id)
	return s.store.DeleteNode(ctx, id)
}

// Reconcile triggers an immediate reconcile pass for a node.
func (s *NodeService) Reconcile(ctx context.Context, id string) error {
	if _, err := s.store.GetNode(ctx, id); err != nil {
		return err
	}
	s.mgr.Reconcile(id)
	return nil
}

// --- Inbounds ---

// InboundInput is the create/update payload for an inbound.
type InboundInput struct {
	Tag              string
	Protocol         domain.Protocol
	ListenPort       int
	PublicHost       string
	PublicPort       int
	Network          domain.Network
	Security         domain.Security
	WSPath           string
	HostHeader       string
	GRPCServiceName  string
	SNI              string
	Fingerprint      string
	RealityPublicKey string
	RealityShortID   string
	Flow             string
	Method           string
	XHTTPMode        string
	Remark           string
}

func (in InboundInput) validate() error {
	if in.Tag == "" {
		return domain.Validationf("tag is required")
	}
	if !in.Protocol.Valid() {
		return domain.Validationf("protocol must be vless|vmess|trojan")
	}
	if in.PublicHost == "" {
		return domain.Validationf("public_host is required")
	}
	if in.PublicPort <= 0 || in.PublicPort > 65535 {
		return domain.Validationf("public_port must be 1-65535")
	}
	if in.Network != "" && !in.Network.Valid() {
		return domain.Validationf("invalid network")
	}
	if in.Security != "" && !in.Security.Valid() {
		return domain.Validationf("invalid security")
	}
	if in.Protocol == domain.ProtocolShadowsocks {
		if in.Method == "" {
			return domain.Validationf("method is required for shadowsocks")
		}
		if !domain.ShadowsocksMethods[in.Method] {
			return domain.Validationf("unsupported shadowsocks method")
		}
	}
	if in.XHTTPMode != "" {
		switch in.XHTTPMode {
		case "auto", "packet-up", "stream-up", "stream-one":
		default:
			return domain.Validationf("invalid xhttp_mode")
		}
	}
	return nil
}

func (s *NodeService) CreateInbound(ctx context.Context, nodeID string, in InboundInput) (*domain.Inbound, error) {
	if _, err := s.store.GetNode(ctx, nodeID); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	ib := inboundFromInput(in)
	ib.ID = uuid.NewString()
	ib.NodeID = nodeID
	if err := s.store.CreateInbound(ctx, ib); err != nil {
		return nil, err
	}
	return s.store.GetInbound(ctx, ib.ID)
}

func (s *NodeService) GetInbound(ctx context.Context, id string) (*domain.Inbound, error) {
	return s.store.GetInbound(ctx, id)
}

func (s *NodeService) ListInbounds(ctx context.Context, nodeID string) ([]*domain.Inbound, error) {
	return s.store.ListInboundsByNode(ctx, nodeID)
}

func (s *NodeService) UpdateInbound(ctx context.Context, id string, in InboundInput) (*domain.Inbound, error) {
	existing, err := s.store.GetInbound(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	ib := inboundFromInput(in)
	ib.ID = existing.ID
	ib.NodeID = existing.NodeID
	if err := s.store.UpdateInbound(ctx, ib); err != nil {
		return nil, err
	}
	// Tag/protocol changes affect what should be pushed; reconcile the node.
	s.mgr.Reconcile(existing.NodeID)
	return s.store.GetInbound(ctx, id)
}

func (s *NodeService) DeleteInbound(ctx context.Context, id string) error {
	return s.store.DeleteInbound(ctx, id)
}

func inboundFromInput(in InboundInput) *domain.Inbound {
	network := in.Network
	if network == "" {
		network = domain.NetworkTCP
	}
	security := in.Security
	if security == "" {
		security = domain.SecurityNone
	}
	return &domain.Inbound{
		Tag:              in.Tag,
		Protocol:         in.Protocol,
		ListenPort:       in.ListenPort,
		PublicHost:       in.PublicHost,
		PublicPort:       in.PublicPort,
		Network:          network,
		Security:         security,
		WSPath:           in.WSPath,
		HostHeader:       in.HostHeader,
		GRPCServiceName:  in.GRPCServiceName,
		SNI:              in.SNI,
		Fingerprint:      in.Fingerprint,
		RealityPublicKey: in.RealityPublicKey,
		RealityShortID:   in.RealityShortID,
		Flow:             in.Flow,
		Method:           in.Method,
		XHTTPMode:        in.XHTTPMode,
		Remark:           in.Remark,
	}
}
