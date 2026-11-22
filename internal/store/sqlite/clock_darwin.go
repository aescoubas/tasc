package sqlite

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func leaseClock() (string, int64, error) {
	boot, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return "", 0, fmt.Errorf("boot session: %w", err)
	}
	var ts unix.Timespec
	err = unix.ClockGettime(unix.CLOCK_UPTIME_RAW, &ts)
	return boot, ts.Nano(), err
}
