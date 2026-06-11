package httpapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ImErdis/xray-api/internal/domain"
)

func decodeErrorBody(t *testing.T, body string) errorBody {
	t.Helper()
	var eb errorBody
	if err := json.Unmarshal([]byte(body), &eb); err != nil {
		t.Fatalf("response is not an error envelope: %v (%s)", err, body)
	}
	return eb
}

func TestWriteErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", domain.ErrNotFound, 404, "not_found"},
		{"wrapped not found", domain.ErrNotFound, 404, "not_found"},
		{"conflict", domain.ErrConflict, 409, "conflict"},
		{"validation", domain.Validationf("bad %s", "input"), 400, "validation_error"},
		{"unknown", errors.New("boom"), 500, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			eb := decodeErrorBody(t, rec.Body.String())
			if eb.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", eb.Error.Code, tc.wantCode)
			}
			if eb.Error.Message == "" {
				t.Error("message is empty")
			}
		})
	}
}

func TestValidationfFormats(t *testing.T) {
	err := domain.Validationf("invalid status %q", "bogus")
	if !errors.Is(err, domain.ErrValidation) {
		t.Error("Validationf must unwrap to ErrValidation")
	}
	if want := `invalid status "bogus"`; err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"a@b.c","bogus":1}`))
	var dst createUserRequest
	if err := decodeJSON(r, &dst); err == nil {
		t.Error("unknown field accepted")
	}

	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"a@b.c"}`))
	if err := decodeJSON(r, &dst); err != nil {
		t.Errorf("valid body rejected: %v", err)
	}
}

func TestQueryInt(t *testing.T) {
	q := url.Values{"page": {"3"}, "bad": {"abc"}, "empty": {""}}

	if n, err := queryInt(q, "page", 1); err != nil || n != 3 {
		t.Errorf("page = %d, %v; want 3, nil", n, err)
	}
	if n, err := queryInt(q, "missing", 50); err != nil || n != 50 {
		t.Errorf("missing = %d, %v; want default 50, nil", n, err)
	}
	if n, err := queryInt(q, "empty", 7); err != nil || n != 7 {
		t.Errorf("empty = %d, %v; want default 7, nil", n, err)
	}
	if _, err := queryInt(q, "bad", 1); err == nil {
		t.Error("non-integer value accepted")
	}
}

func TestUserStatusValid(t *testing.T) {
	for _, s := range []domain.UserStatus{
		domain.UserStatusActive, domain.UserStatusSuspended,
		domain.UserStatusExpired, domain.UserStatusDisabled,
	} {
		if !s.Valid() {
			t.Errorf("%s should be valid", s)
		}
	}
	for _, s := range []domain.UserStatus{"", "Active", "deleted", "bogus"} {
		if s.Valid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}
