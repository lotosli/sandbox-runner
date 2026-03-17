package collector

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

func Healthy(ctx context.Context, endpoint string, timeout time.Duration) bool {
	if endpoint == "" {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	address := u.Host
	if !strings.Contains(address, ":") {
		if u.Scheme == "https" {
			address += ":443"
		} else {
			address += ":80"
		}
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
