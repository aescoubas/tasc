package agent

import "context"

// RunContext stops renewal when the caller cancels or the optional agent PID exits.
func RunContext(parent context.Context, pid int) (context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(parent)
	if pid != 0 {
		if err := watchProcess(ctx, pid, cancel); err != nil {
			cancel()
			return nil, nil, err
		}
	}
	return ctx, cancel, nil
}
