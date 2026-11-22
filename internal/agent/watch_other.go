//go:build !linux

package agent

import (
	"context"
	"errors"
)

func watchProcess(ctx context.Context, pid int, exited context.CancelFunc) error {
	return errors.New("PID-bound CLI claims require Linux; use the stdio MCP claim tool on this platform")
}
