package main

import (
	"net/http"
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

func TestEnsureFreshCacheTriggersUpdateWhenStale(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/cache/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":1,"updated_at":"2026-04-12T00:00:00Z","stale":true,"mpd_connected":true,"mpd_updating":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cache/update":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"Cache updated"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := &apiClient{
		baseURL:    server.URL + "/api/v1",
		httpClient: server.Client(),
	}

	if err := client.ensureFreshCache(); err != nil {
		t.Fatalf("ensureFreshCache() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
}

func TestEnsureFreshCacheSkipsUpdateWhileMPDIsUpdating(t *testing.T) {
	requests := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/cache/status" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":1,"updated_at":"2026-04-12T00:00:00Z","stale":true,"mpd_connected":true,"mpd_updating":true}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client := &apiClient{
		baseURL:    server.URL + "/api/v1",
		httpClient: server.Client(),
	}

	if err := client.ensureFreshCache(); err != nil {
		t.Fatalf("ensureFreshCache() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
}
