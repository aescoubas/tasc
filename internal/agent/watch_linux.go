package agent

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func watchProcess(ctx context.Context, pid int, exited context.CancelFunc) error {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return fmt.Errorf("watch agent PID %d: %w", pid, err)
	}
	// A pidfd refers to this process instance even if the numeric PID is reused.
	go func() {
		defer unix.Close(fd)
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		for ctx.Err() == nil {
			n, err := unix.Poll(fds, 1000)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil || n > 0 {
				exited()
				return
			}
		}
	}()
	return nil
}
