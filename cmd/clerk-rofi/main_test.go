package main

import (
	"net/http/httptest"
	"testing"

	"github.com/carnager/clerk-modular/internal/shared"
)

func TestResolveAPIBaseURLDetectsLocalEndpoints(t *testing.T) {
	tests := []struct {
		name          string
		cfgAddress    string
		override      string
		wantURL       string
		wantLocal     bool
		wantUseSocket bool
	}{
		{name: "default", wantURL: shared.LocalAPIBaseURL, wantLocal: true, wantUseSocket: true},
		{name: "local keyword", cfgAddress: "local", wantURL: shared.LocalAPIBaseURL, wantLocal: true, wantUseSocket: true},
		{name: "unix socket path", cfgAddress: "/tmp/clerkd.sock", wantURL: shared.LocalAPIBaseURL, wantLocal: true, wantUseSocket: true},
		{name: "localhost custom port", cfgAddress: "localhost:7777", wantURL: "http://localhost:7777/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "ipv4 loopback", cfgAddress: "127.0.0.1:6601", wantURL: "http://127.0.0.1:6601/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "ipv6 loopback", cfgAddress: "[::1]:6601", wantURL: "http://[::1]:6601/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "remote host", cfgAddress: "musicbox:6601", wantURL: "http://musicbox:6601/api/v1", wantLocal: false, wantUseSocket: false},
		{name: "override wins", cfgAddress: "musicbox:6601", override: "localhost:6602", wantURL: "http://localhost:6602/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "malformed address", cfgAddress: "musicbox", wantURL: "http://musicbox/api/v1", wantLocal: false, wantUseSocket: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config{}
			cfg.API.Address = tc.cfgAddress

			gotURL, gotLocal, gotUseSocket, _, err := resolveAPIAddress(cfg, tc.override)
			if err != nil {
				t.Fatalf("resolveAPIAddress() error = %v", err)
			}
			if gotURL != tc.wantURL {
				t.Fatalf("resolveAPIAddress() url = %q, want %q", gotURL, tc.wantURL)
			}
			if gotLocal != tc.wantLocal {
				t.Fatalf("resolveAPIAddress() local = %v, want %v", gotLocal, tc.wantLocal)
			}
			if gotUseSocket != tc.wantUseSocket {
				t.Fatalf("resolveAPIAddress() useSocket = %v, want %v", gotUseSocket, tc.wantUseSocket)
			}
		})
	}
}

func TestDecodeAPIMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "error field", body: `{"error":"no tracks found in MPD"}`, want: "no tracks found in MPD"},
		{name: "message field", body: `{"message":"Cache updated"}`, want: "Cache updated"},
		{name: "not json", body: `plain error`, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeAPIMessage([]byte(tc.body)); got != tc.want {
				t.Fatalf("decodeAPIMessage(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestAPIErrorMessagePrefersStructuredError(t *testing.T) {
	req := httptest.NewRequest("POST", "http://musicbox:6601/api/v1/playlist/add/albums", nil)
	got := apiErrorMessage(req, 500, []byte(`{"error":"no tracks found in MPD for Example Artist - Example Album (2024)"}`))
	want := "no tracks found in MPD for Example Artist - Example Album (2024)"
	if got != want {
		t.Fatalf("apiErrorMessage() = %q, want %q", got, want)
	}
}
