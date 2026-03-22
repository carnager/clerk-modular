package main

import (
	"testing"

	"github.com/carnager/clerk-modular/internal/shared"
)

func TestResolveAPIBaseURLDetectsLocalEndpoints(t *testing.T) {
	tests := []struct {
		name          string
		cfgURL        string
		override      string
		wantURL       string
		wantLocal     bool
		wantUseSocket bool
	}{
		{name: "default", wantURL: shared.LocalAPIBaseURL, wantLocal: true, wantUseSocket: true},
		{name: "local keyword", cfgURL: "local", wantURL: shared.LocalAPIBaseURL, wantLocal: true, wantUseSocket: true},
		{name: "localhost custom port", cfgURL: "http://localhost:7777/api/v1", wantURL: "http://localhost:7777/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "ipv4 loopback", cfgURL: "http://127.0.0.1:6601/api/v1", wantURL: "http://127.0.0.1:6601/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "ipv6 loopback", cfgURL: "http://[::1]:6601/api/v1/", wantURL: "http://[::1]:6601/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "remote host", cfgURL: "http://musicbox:6601/api/v1", wantURL: "http://musicbox:6601/api/v1", wantLocal: false, wantUseSocket: false},
		{name: "override wins", cfgURL: "http://musicbox:6601/api/v1", override: "http://localhost:6602/api/v1", wantURL: "http://localhost:6602/api/v1", wantLocal: true, wantUseSocket: false},
		{name: "malformed url", cfgURL: "localhost:6601/api/v1", wantURL: "localhost:6601/api/v1", wantLocal: false, wantUseSocket: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config{}
			cfg.API.BaseURL = tc.cfgURL

			gotURL, gotLocal, gotUseSocket := resolveAPIBaseURL(cfg, tc.override)
			if gotURL != tc.wantURL {
				t.Fatalf("resolveAPIBaseURL() url = %q, want %q", gotURL, tc.wantURL)
			}
			if gotLocal != tc.wantLocal {
				t.Fatalf("resolveAPIBaseURL() local = %v, want %v", gotLocal, tc.wantLocal)
			}
			if gotUseSocket != tc.wantUseSocket {
				t.Fatalf("resolveAPIBaseURL() useSocket = %v, want %v", gotUseSocket, tc.wantUseSocket)
			}
		})
	}
}
