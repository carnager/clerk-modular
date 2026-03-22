package shared

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LocalAPIConfigValue = "local"
	LocalAPIBaseURL     = "http://clerkd/api/v1"
)

func IsLocalAPIConfigValue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), LocalAPIConfigValue)
}

func IsLoopbackAPIBaseURL(base string) bool {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func DefaultSocketPath() string {
	runtimeDir := Getenv("XDG_RUNTIME_DIR", filepath.Join(os.TempDir(), fmt.Sprintf("clerk-%d", os.Getuid())))
	return filepath.Join(runtimeDir, "clerk", "clerkd.sock")
}

func NewLocalHTTPClient(timeout time.Duration, socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
