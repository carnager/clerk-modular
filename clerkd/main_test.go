package main

import (
	"path/filepath"
	"testing"
)

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
