package xray

import (
	"errors"
	"testing"
)

func TestMapAlterErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		op      alterOp
		wantNil bool
	}{
		{"add nil", nil, opAdd, true},
		{"add already exists", errors.New("User alice already exists."), opAdd, true},
		{"add other error", errors.New("connection refused"), opAdd, false},
		{"remove not found", errors.New("User bob not found."), opRemove, true},
		{"remove not exist", errors.New("user does not exist"), opRemove, true},
		{"remove other error", errors.New("timeout"), opRemove, false},
		{"add not-found is not idempotent", errors.New("not found"), opAdd, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapAlterErr(tt.err, tt.op)
			if (got == nil) != tt.wantNil {
				t.Errorf("mapAlterErr(%v) = %v, wantNil=%v", tt.err, got, tt.wantNil)
			}
		})
	}
}
