package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fhs/gompd/v2/mpd"
)

func TestMPDEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantNetwork string
		wantAddress string
	}{
		{name: "default", address: "", wantNetwork: "tcp", wantAddress: "localhost:6600"},
		{name: "tcp", address: "musicbox:6601", wantNetwork: "tcp", wantAddress: "musicbox:6601"},
		{name: "unix socket", address: "/run/user/1000/mpd/socket", wantNetwork: "unix", wantAddress: "/run/user/1000/mpd/socket"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotNetwork, gotAddress := mpdEndpoint(tc.address)
			if gotNetwork != tc.wantNetwork || gotAddress != tc.wantAddress {
				t.Fatalf("mpdEndpoint(%q) = (%q, %q), want (%q, %q)", tc.address, gotNetwork, gotAddress, tc.wantNetwork, tc.wantAddress)
			}
		})
	}
}

func TestDefaultBindToAddress(t *testing.T) {
	binds := defaultBindToAddress()
	if len(binds) != 2 {
		t.Fatalf("defaultBindToAddress() len = %d, want 2", len(binds))
	}
	if binds[0] != "0.0.0.0:6601" {
		t.Fatalf("defaultBindToAddress()[0] = %q, want %q", binds[0], "0.0.0.0:6601")
	}
	if binds[1] == "" {
		t.Fatalf("defaultBindToAddress()[1] should not be empty")
	}
}

func TestShouldRefreshForMPDEvent(t *testing.T) {
	tests := []struct {
		event string
		want  bool
	}{
		{event: "database", want: true},
		{event: " database ", want: true},
		{event: "update", want: false},
		{event: "player", want: false},
		{event: "", want: false},
	}

	for _, tc := range tests {
		if got := shouldRefreshForMPDEvent(tc.event); got != tc.want {
			t.Fatalf("shouldRefreshForMPDEvent(%q) = %v, want %v", tc.event, got, tc.want)
		}
	}
}

func TestCacheStateRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	app := &app{
		paths: paths{
			CacheStateFile: filepath.Join(tempDir, "cache.state"),
		},
	}

	want := newCacheState(time.Unix(123, 456).UTC())
	if err := app.saveCacheState(want); err != nil {
		t.Fatalf("saveCacheState() failed: %v", err)
	}

	got, err := app.loadCacheState()
	if err != nil {
		t.Fatalf("loadCacheState() failed: %v", err)
	}
	if got != want {
		t.Fatalf("loadCacheState() = %#v, want %#v", got, want)
	}
}

func TestCacheIsStale(t *testing.T) {
	state := cacheState{
		Version:   time.Unix(100, 0).UTC().UnixNano(),
		UpdatedAt: time.Unix(100, 0).UTC().Format(time.RFC3339Nano),
	}

	if cacheIsStale(state, 0) {
		t.Fatal("cacheIsStale() with zero db update should be false")
	}
	if cacheIsStale(state, 100) {
		t.Fatal("cacheIsStale() with equal timestamps should be false")
	}
	if !cacheIsStale(state, 101) {
		t.Fatal("cacheIsStale() with newer db update should be true")
	}
}

func TestHandleCacheStatus(t *testing.T) {
	tempDir := t.TempDir()
	app := &app{
		paths: paths{
			CacheStateFile: filepath.Join(tempDir, "cache.state"),
		},
	}

	state := newCacheState(time.Unix(500, 0).UTC())
	if err := app.saveCacheState(state); err != nil {
		t.Fatalf("saveCacheState() failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil)
	rec := httptest.NewRecorder()
	app.handleCacheStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleCacheStatus() status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Clerk-Cache-Version"); got != "500000000000" {
		t.Fatalf("X-Clerk-Cache-Version = %q, want %q", got, "500000000000")
	}
	if got := rec.Header().Get("X-Clerk-Cache-Updated-At"); got != state.UpdatedAt {
		t.Fatalf("X-Clerk-Cache-Updated-At = %q, want %q", got, state.UpdatedAt)
	}

	var payload cacheState
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload != state {
		t.Fatalf("response payload = %#v, want %#v", payload, state)
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

type fakeAlbumFinder struct {
	findCalls [][]string
	results   map[string][]mpd.Attrs
	err       error
}

func (f *fakeAlbumFinder) Find(args ...string) ([]mpd.Attrs, error) {
	f.findCalls = append(f.findCalls, append([]string(nil), args...))
	if f.err != nil {
		return nil, f.err
	}
	return f.results[strings.Join(args, "\x00")], nil
}

func TestFindAlbumTracksFallsBackToArtistTag(t *testing.T) {
	album := map[string]any{
		"albumartist": "Example Artist",
		"album":       "Example Album",
		"date":        "2024",
	}
	finder := &fakeAlbumFinder{
		results: map[string][]mpd.Attrs{
			strings.Join([]string{"artist", "Example Artist", "album", "Example Album", "date", "2024"}, "\x00"): {
				{"file": "music/example.flac"},
			},
		},
	}

	attrs, err := findAlbumTracks(finder, album)
	if err != nil {
		t.Fatalf("findAlbumTracks() error = %v", err)
	}
	if len(attrs) != 1 || attrs[0]["file"] != "music/example.flac" {
		t.Fatalf("findAlbumTracks() attrs = %#v, want fallback artist match", attrs)
	}
	if len(finder.findCalls) != 2 {
		t.Fatalf("Find() call count = %d, want 2", len(finder.findCalls))
	}
}

func TestFindAlbumTracksErrorsWhenNoTracksFound(t *testing.T) {
	album := map[string]any{
		"albumartist": "Missing Artist",
		"album":       "Missing Album",
		"date":        "1999",
	}
	finder := &fakeAlbumFinder{results: map[string][]mpd.Attrs{}}

	_, err := findAlbumTracks(finder, album)
	if err == nil {
		t.Fatal("findAlbumTracks() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no tracks found in MPD for Missing Artist - Missing Album (1999)") {
		t.Fatalf("findAlbumTracks() error = %q, want missing album message", err.Error())
	}
}

func TestFindAlbumTracksPropagatesMPDErrors(t *testing.T) {
	finder := &fakeAlbumFinder{err: errors.New("mpd down")}
	album := map[string]any{
		"albumartist": "Example Artist",
		"album":       "Example Album",
		"date":        "2024",
	}

	_, err := findAlbumTracks(finder, album)
	if err == nil || err.Error() != "mpd down" {
		t.Fatalf("findAlbumTracks() error = %v, want %q", err, "mpd down")
	}
}
