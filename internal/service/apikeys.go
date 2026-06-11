package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
)

// APIKeyService manages admin API keys.
type APIKeyService struct {
	store *store.Store
}

func NewAPIKeyService(st *store.Store) *APIKeyService { return &APIKeyService{store: st} }

// CreatedKey is returned once at creation and includes the plaintext secret.
type CreatedKey struct {
	Key       *domain.APIKey
	Plaintext string
}

func (s *APIKeyService) Create(ctx context.Context, name string) (*CreatedKey, error) {
	if strings.TrimSpace(name) == "" {
		return nil, domain.Validationf("name is required")
	}
	plaintext := "xapi_" + randToken(24)
	k := &domain.APIKey{
		ID:      uuid.NewString(),
		Name:    name,
		KeyHash: HashKey(plaintext),
	}
	if err := s.store.CreateAPIKey(ctx, k); err != nil {
		return nil, err
	}
	return &CreatedKey{Key: k, Plaintext: plaintext}, nil
}

func (s *APIKeyService) List(ctx context.Context) ([]*domain.APIKey, error) {
	return s.store.ListAPIKeys(ctx)
}

func (s *APIKeyService) Revoke(ctx context.Context, id string) error {
	return s.store.RevokeAPIKey(ctx, id)
}

// Verify checks a presented plaintext key against stored hashes. The bootstrap
// key, if configured, is always accepted.
func (s *APIKeyService) Verify(ctx context.Context, plaintext, bootstrap string) bool {
	if plaintext == "" {
		return false
	}
	if bootstrap != "" && subtleEqual(plaintext, bootstrap) {
		return true
	}
	_, err := s.store.FindAPIKeyByHash(ctx, HashKey(plaintext))
	return err == nil
}
