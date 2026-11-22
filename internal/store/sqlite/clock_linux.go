package sqlite

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func leaseClock() (string, int64, error) {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", 0, err
	}
	var ts unix.Timespec
	err = unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	return strings.TrimSpace(string(boot)), ts.Nano(), err
}
