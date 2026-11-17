// Package auth handles the runtime-only REST credential shared by server and CLI.
package auth

import (
	"errors"
	"io"
	"net"
	"os"
	"strings"
)

func ValidateToken(token string) error {
	if len(token) < 32 || len(token) > 4096 {
		return errors.New("API token must contain 32 to 4096 printable non-space ASCII characters")
	}
	for _, c := range token {
		if c < 33 || c > 126 {
			return errors.New("API token must contain 32 to 4096 printable non-space ASCII characters")
		}
	}
	return nil
}

// ReadToken never includes credential bytes in an error. A trailing newline is
// allowed, but multi-line files and broadly readable files are refused.
func ReadToken(path string) (string, error) {
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return "", errors.New("API token file must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open API token file")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || (st.Mode().Perm() != 0600 && st.Mode().Perm() != 0400) {
		return "", errors.New("API token file must be a regular file with mode 0600 or 0400")
	}
	data, err := io.ReadAll(io.LimitReader(f, 4098))
	if err != nil || len(data) > 4097 {
		return "", errors.New("cannot read bounded API token file")
	}
	token := strings.TrimSuffix(string(data), "\n")
	if err := ValidateToken(token); err != nil {
		return "", err
	}
	return token, nil
}

func IsLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}
