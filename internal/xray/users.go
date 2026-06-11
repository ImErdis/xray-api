package xray

import (
	"context"
	"fmt"
	"strings"

	hcommand "github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/serial"
)

func (c *grpcClient) AddUser(ctx context.Context, inboundTag string, acc Account) error {
	user, err := buildUser(acc)
	if err != nil {
		return err
	}
	_, err = c.handler.AlterInbound(ctx, &hcommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&hcommand.AddUserOperation{
			User: user,
		}),
	})
	return mapAlterErr(err, opAdd)
}

func (c *grpcClient) RemoveUser(ctx context.Context, inboundTag, email string) error {
	_, err := c.handler.AlterInbound(ctx, &hcommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&hcommand.RemoveUserOperation{
			Email: email,
		}),
	})
	return mapAlterErr(err, opRemove)
}

type alterOp int

const (
	opAdd alterOp = iota
	opRemove
)

// mapAlterErr centralizes the only place we depend on Xray's plain-string
// error semantics. Xray returns ordinary errors (not gRPC status codes we can
// switch on), so we string-match the idempotent cases: re-adding an existing
// user and removing an absent one are both successes for our converge model.
func mapAlterErr(err error, op alterOp) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch op {
	case opAdd:
		if strings.Contains(msg, "already exists") {
			return nil
		}
	case opRemove:
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not exist") {
			return nil
		}
	}
	return fmt.Errorf("alter inbound: %w", err)
}
