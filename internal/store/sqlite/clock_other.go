//go:build !linux && !darwin

package sqlite

import "errors"

func leaseClock() (string, int64, error) {
	return "", 0, errors.New("agent leases require a supported awake-time clock")
}
