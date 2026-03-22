package main

import (
	"path/filepath"
	"testing"
)

func TestParseHostPortString(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		fallbackPort int
		wantHost     string
		wantPort     int
	}{
		{name: "empty", value: "", fallbackPort: 0, wantHost: "localhost", wantPort: 6600},
		{name: "host only", value: "musicbox", fallbackPort: 6600, wantHost: "musicbox", wantPort: 6600},
		{name: "host and port", value: "musicbox:6601", fallbackPort: 6600, wantHost: "musicbox", wantPort: 6601},
		{name: "ipv4 and port", value: "127.0.0.1:6602", fallbackPort: 6600, wantHost: "127.0.0.1", wantPort: 6602},
		{name: "bracketed ipv6 host only", value: "[::1]", fallbackPort: 6600, wantHost: "::1", wantPort: 6600},
		{name: "bracketed ipv6 and port", value: "[::1]:6603", fallbackPort: 6600, wantHost: "::1", wantPort: 6603},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotHost, gotPort := parseHostPortString(tc.value, tc.fallbackPort)
			if gotHost != tc.wantHost || gotPort != tc.wantPort {
				t.Fatalf("parseHostPortString(%q, %d) = (%q, %d), want (%q, %d)", tc.value, tc.fallbackPort, gotHost, gotPort, tc.wantHost, tc.wantPort)
			}
		})
	}
}

func TestMPDAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "defaults", host: "", port: 0, want: "localhost:6600"},
		{name: "host only", host: "musicbox", port: 0, want: "musicbox:6600"},
		{name: "host and port", host: "musicbox", port: 6601, want: "musicbox:6601"},
		{name: "ipv6 host", host: "::1", port: 6600, want: "[::1]:6600"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mpdAddress(tc.host, tc.port); got != tc.want {
				t.Fatalf("mpdAddress(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
		})
	}
}

func TestUpdateTrackCacheRating(t *testing.T) {
	tempDir := t.TempDir()
	app := &app{
		paths: paths{
			TracksCacheFile: filepath.Join(tempDir, "tracks.cache"),
		},
	}

	initial := []map[string]any{
		{"id": "0", "file": "music/one.flac", "rating": nil},
		{"id": "1", "file": "music/two.flac", "rating": "4"},
	}
	if err := app.writeMapSlice(app.paths.TracksCacheFile, initial); err != nil {
		t.Fatalf("writeMapSlice() failed: %v", err)
	}

	if err := app.updateTrackCacheRating("music/one.flac", "7"); err != nil {
		t.Fatalf("updateTrackCacheRating(set) failed: %v", err)
	}
	tracks, err := app.readMapSlice(app.paths.TracksCacheFile)
	if err != nil {
		t.Fatalf("readMapSlice() after set failed: %v", err)
	}
	if got := stringify(tracks[0]["rating"]); got != "7" {
		t.Fatalf("track[0] rating after set = %q, want %q", got, "7")
	}

	if err := app.updateTrackCacheRating("music/two.flac", ""); err != nil {
		t.Fatalf("updateTrackCacheRating(delete) failed: %v", err)
	}
	tracks, err = app.readMapSlice(app.paths.TracksCacheFile)
	if err != nil {
		t.Fatalf("readMapSlice() after delete failed: %v", err)
	}
	if tracks[1]["rating"] != nil {
		t.Fatalf("track[1] rating after delete = %#v, want nil", tracks[1]["rating"])
	}
}

func TestUpdateTrackCacheRatingIgnoresMissingFile(t *testing.T) {
	tempDir := t.TempDir()
	app := &app{
		paths: paths{
			TracksCacheFile: filepath.Join(tempDir, "tracks.cache"),
		},
	}

	initial := []map[string]any{
		{"id": "0", "file": "music/one.flac", "rating": "3"},
	}
	if err := app.writeMapSlice(app.paths.TracksCacheFile, initial); err != nil {
		t.Fatalf("writeMapSlice() failed: %v", err)
	}

	if err := app.updateTrackCacheRating("music/missing.flac", "9"); err != nil {
		t.Fatalf("updateTrackCacheRating(missing) failed: %v", err)
	}
	tracks, err := app.readMapSlice(app.paths.TracksCacheFile)
	if err != nil {
		t.Fatalf("readMapSlice() failed: %v", err)
	}
	if got := stringify(tracks[0]["rating"]); got != "3" {
		t.Fatalf("track[0] rating = %q, want unchanged %q", got, "3")
	}
}
