package shared

import "testing"

func TestResolveSocketPath(t *testing.T) {
	if got := ResolveSocketPath("local"); got != DefaultSocketPath() {
		t.Fatalf("ResolveSocketPath(local) = %q, want %q", got, DefaultSocketPath())
	}

	explicit := "/tmp/clerk-test.sock"
	if got := ResolveSocketPath(explicit); got != explicit {
		t.Fatalf("ResolveSocketPath(explicit) = %q, want %q", got, explicit)
	}
}

func TestAPIBaseURLFromAddress(t *testing.T) {
	tests := []struct {
		name          string
		address       string
		wantURL       string
		wantUseSocket bool
		wantSocket    string
	}{
		{name: "local keyword", address: "local", wantURL: LocalAPIBaseURL, wantUseSocket: true, wantSocket: DefaultSocketPath()},
		{name: "unix socket", address: "/tmp/clerkd.sock", wantURL: LocalAPIBaseURL, wantUseSocket: true, wantSocket: "/tmp/clerkd.sock"},
		{name: "tcp address", address: "musicbox:6601", wantURL: "http://musicbox:6601/api/v1", wantUseSocket: false, wantSocket: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotUseSocket, gotSocket, err := APIBaseURLFromAddress(tc.address)
			if err != nil {
				t.Fatalf("APIBaseURLFromAddress() error = %v", err)
			}
			if gotURL != tc.wantURL || gotUseSocket != tc.wantUseSocket || gotSocket != tc.wantSocket {
				t.Fatalf("APIBaseURLFromAddress(%q) = (%q, %v, %q), want (%q, %v, %q)", tc.address, gotURL, gotUseSocket, gotSocket, tc.wantURL, tc.wantUseSocket, tc.wantSocket)
			}
		})
	}
}
