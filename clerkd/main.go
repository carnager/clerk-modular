package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/carnager/clerk-modular/internal/shared"
	"github.com/fhs/gompd/v2/mpd"
	"github.com/vmihailenco/msgpack/v5"
)

type config struct {
	Server struct {
		BindToAddress []string `toml:"bind_to_address"`
	} `toml:"server"`
	MPD struct {
		Address string `toml:"address"`
	} `toml:"mpd"`
	Random struct {
		Tracks    int    `toml:"tracks"`
		ArtistTag string `toml:"artist_tag"`
	} `toml:"random"`
	Cache struct {
		BatchSize int `toml:"batch_size"`
	} `toml:"cache"`
}

type paths struct {
	DataDir          string
	ConfigPath       string
	AlbumCacheFile   string
	TracksCacheFile  string
	LatestCacheFile  string
	RatingsCacheFile string
	CacheStateFile   string
}

type cacheState struct {
	Version   int64  `json:"version" msgpack:"version"`
	UpdatedAt string `json:"updated_at" msgpack:"updated_at"`
}

type app struct {
	cfg       config
	paths     paths
	logger    *log.Logger
	cacheLock sync.Mutex
}

func main() {
	logger := log.New(os.Stdout, "clerkd: ", log.LstdFlags)
	cfg, pathCfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	a := &app{
		cfg:    cfg,
		paths:  pathCfg,
		logger: logger,
	}

	if err := a.ensureStartupState(); err != nil {
		logger.Fatalf("startup failed: %v", err)
	}

	go a.watchMPDDatabaseUpdates()

	if err := a.serve(); err != nil {
		logger.Fatalf("listen and serve: %v", err)
	}
}

func loadConfig() (config, paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, paths{}, err
	}
	xdgData := getenvDefault("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	xdgConfig := getenvDefault("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	pathCfg := paths{
		DataDir:          filepath.Join(xdgData, "clerk"),
		ConfigPath:       filepath.Join(xdgConfig, "clerk", "clerkd.toml"),
		AlbumCacheFile:   filepath.Join(xdgData, "clerk", "album.cache"),
		TracksCacheFile:  filepath.Join(xdgData, "clerk", "tracks.cache"),
		LatestCacheFile:  filepath.Join(xdgData, "clerk", "latest.cache"),
		RatingsCacheFile: filepath.Join(xdgData, "clerk", "ratings.cache"),
		CacheStateFile:   filepath.Join(xdgData, "clerk", "cache.state"),
	}

	if err := os.MkdirAll(pathCfg.DataDir, 0o755); err != nil {
		return config{}, paths{}, err
	}
	if err := os.MkdirAll(filepath.Dir(pathCfg.ConfigPath), 0o755); err != nil {
		return config{}, paths{}, err
	}

	if _, err := os.Stat(pathCfg.ConfigPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(pathCfg.ConfigPath, []byte(defaultDaemonConfig()), 0o644); err != nil {
			return config{}, paths{}, err
		}
	}

	var raw map[string]any
	if _, err := toml.DecodeFile(pathCfg.ConfigPath, &raw); err != nil {
		return config{}, paths{}, err
	}
	var cfg config
	server, _ := raw["server"].(map[string]any)
	mpdSection, _ := raw["mpd"].(map[string]any)
	random, _ := raw["random"].(map[string]any)
	cache, _ := raw["cache"].(map[string]any)
	cfg.Server.BindToAddress = stringSlice(server["bind_to_address"])
	cfg.MPD.Address = stringify(mpdSection["address"])
	cfg.Random.Tracks = intFromAny(random["tracks"], 20)
	cfg.Random.ArtistTag = stringify(random["artist_tag"])
	cfg.Cache.BatchSize = intFromAny(cache["batch_size"], 10000)
	applyDefaults(&cfg)
	return cfg, pathCfg, nil
}

func defaultDaemonConfig() string {
	return `[server]
bind_to_address = ["0.0.0.0:6601", "` + shared.DefaultSocketPath() + `"]

[mpd]
address = "localhost:6600"

[random]
tracks = 20
artist_tag = "albumartist"

[cache]
batch_size = 10000
`
}

func applyDefaults(cfg *config) {
	if cfg.MPD.Address == "" {
		cfg.MPD.Address = "localhost:6600"
	}
	if cfg.Random.Tracks <= 0 {
		cfg.Random.Tracks = 20
	}
	if cfg.Random.ArtistTag == "" {
		cfg.Random.ArtistTag = "albumartist"
	}
	if cfg.Cache.BatchSize <= 0 {
		cfg.Cache.BatchSize = 10000
	}
	if envBind := os.Getenv("CLERKD_BIND_TO_ADDRESS"); envBind != "" {
		cfg.Server.BindToAddress = splitAndTrim(envBind, ",")
	}
	if envMPDAddress := os.Getenv("CLERKD_MPD_ADDRESS"); envMPDAddress != "" {
		cfg.MPD.Address = strings.TrimSpace(envMPDAddress)
	}
	if len(cfg.Server.BindToAddress) == 0 {
		cfg.Server.BindToAddress = defaultBindToAddress()
	}
}

func (a *app) serve() error {
	handler := a.routes()
	listeners, err := a.listenConfigured()
	if err != nil {
		return err
	}

	errCh := make(chan error, len(listeners))
	for _, listener := range listeners {
		l := listener
		go func() {
			errCh <- http.Serve(l, handler)
		}()
	}

	err = <-errCh
	for _, listener := range listeners {
		_ = listener.Close()
	}
	return err
}

func (a *app) listenConfigured() ([]net.Listener, error) {
	listeners := make([]net.Listener, 0, len(a.cfg.Server.BindToAddress))
	for _, bind := range a.cfg.Server.BindToAddress {
		listener, err := a.listenAddress(bind)
		if err != nil {
			for _, existing := range listeners {
				_ = existing.Close()
			}
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func (a *app) listenAddress(bind string) (net.Listener, error) {
	bind = strings.TrimSpace(bind)
	if bind == "" {
		return nil, fmt.Errorf("empty bind_to_address entry")
	}
	if isUnixBindAddress(bind) {
		listener, err := listenUnixSocket(bind)
		if err != nil {
			return nil, err
		}
		a.logger.Printf("serving unix socket on %s", bind)
		return listener, nil
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	a.logger.Printf("serving tcp on %s", bind)
	return listener, nil
}

func listenUnixSocket(socketPath string) (net.Listener, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("empty socket path")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}

func isUnixBindAddress(bind string) bool {
	return strings.Contains(bind, "/")
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", a.handleHealth)
	mux.HandleFunc("GET /api/v1/albums", a.handleAlbums)
	mux.HandleFunc("GET /api/v1/latest_albums", a.handleLatestAlbums)
	mux.HandleFunc("GET /api/v1/tracks", a.handleTracks)
	mux.HandleFunc("GET /api/v1/cache/status", a.handleCacheStatus)
	mux.HandleFunc("GET /api/v1/albums/{album_id}/rating", a.handleAlbumRatingGet)
	mux.HandleFunc("POST /api/v1/albums/{album_id}/rating", a.handleAlbumRatingPost)
	mux.HandleFunc("POST /api/v1/tracks/{track_id}/rating", a.handleTrackRatingPost)
	mux.HandleFunc("POST /api/v1/playlist/add/album/{album_id}", a.handleAddAlbum)
	mux.HandleFunc("POST /api/v1/playlist/add/track/{track_id}", a.handleAddTrack)
	mux.HandleFunc("POST /api/v1/playlist/add/albums", a.handleAddAlbums)
	mux.HandleFunc("POST /api/v1/playlist/add/tracks", a.handleAddTracks)
	mux.HandleFunc("POST /api/v1/playback/random/album", a.handleRandomAlbum)
	mux.HandleFunc("POST /api/v1/playback/random/tracks", a.handleRandomTracks)
	mux.HandleFunc("POST /api/v1/cache/update", a.handleCacheUpdate)
	mux.HandleFunc("GET /api/v1/current_album/rating", a.handleCurrentAlbumRatingGet)
	mux.HandleFunc("POST /api/v1/current_album/rating", a.handleCurrentAlbumRatingPost)
	mux.HandleFunc("GET /api/v1/current_track/rating", a.handleCurrentTrackRatingGet)
	mux.HandleFunc("POST /api/v1/current_track/rating", a.handleCurrentTrackRatingPost)

	return mux
}

func (a *app) ensureStartupState() error {
	if _, err := os.Stat(a.paths.RatingsCacheFile); errors.Is(err, os.ErrNotExist) {
		if err := a.saveRatings(map[string]string{}); err != nil {
			return err
		}
	}
	if a.allCachesExist() {
		if err := a.ensureCacheState(); err != nil {
			return err
		}
		return nil
	}
	return a.rebuildCache("startup")
}

func (a *app) allCachesExist() bool {
	required := []string{a.paths.AlbumCacheFile, a.paths.TracksCacheFile, a.paths.LatestCacheFile, a.paths.RatingsCacheFile}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

func (a *app) ensureCacheState() error {
	if _, err := os.Stat(a.paths.CacheStateFile); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	state, err := a.deriveCacheState()
	if err != nil {
		return err
	}
	return a.saveCacheState(state)
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":        "error",
			"mpd_connected": false,
			"error":         err.Error(),
		})
		return
	}
	defer client.Close()
	status, err := client.Status()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":        "error",
			"mpd_connected": false,
			"error":         err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"mpd_connected": true,
		"state":         status["state"],
	})
}

func (a *app) handleAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := a.readMapSlice(a.paths.AlbumCacheFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ratings, err := a.loadRatings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.applyCacheStateHeaders(w)
	writeJSON(w, http.StatusOK, attachAlbumRatings(albums, ratings))
}

func (a *app) handleLatestAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := a.readMapSlice(a.paths.LatestCacheFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ratings, err := a.loadRatings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.applyCacheStateHeaders(w)
	writeJSON(w, http.StatusOK, attachAlbumRatings(albums, ratings))
}

func (a *app) handleTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := a.readMapSlice(a.paths.TracksCacheFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.applyCacheStateHeaders(w)
	writeJSON(w, http.StatusOK, tracks)
}

func (a *app) handleCacheStatus(w http.ResponseWriter, r *http.Request) {
	state, err := a.loadCacheState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.writeCacheStateHeaders(w, state)
	writeJSON(w, http.StatusOK, state)
}

func (a *app) handleAlbumRatingGet(w http.ResponseWriter, r *http.Request) {
	cachePath, err := a.albumCachePath(strings.TrimSpace(r.URL.Query().Get("list_mode")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	albums, err := a.readMapSlice(cachePath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	album := findByID(albums, r.PathValue("album_id"))
	if album == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Album not found"})
		return
	}
	ratings, err := a.loadRatings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"album_id": r.PathValue("album_id"),
		"rating":   ratings[albumKey(album)],
	})
}

func (a *app) handleAlbumRatingPost(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody(w, r)
	if !ok {
		return
	}
	rating := stringify(body["rating"])
	if rating == "" {
		rating = "---"
	}
	if !validRating(rating) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid rating"})
		return
	}
	listMode := stringify(body["list_mode"])
	if listMode == "" {
		listMode = strings.TrimSpace(r.URL.Query().Get("list_mode"))
	}
	cachePath, err := a.albumCachePath(listMode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	albums, err := a.readMapSlice(cachePath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	album := findByID(albums, r.PathValue("album_id"))
	if album == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Album not found"})
		return
	}
	changed, err := a.updateAlbumRating(album, rating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"changed": changed})
}

func (a *app) handleTrackRatingPost(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody(w, r)
	if !ok {
		return
	}
	rating := stringify(body["rating"])
	if rating == "" {
		rating = "---"
	}
	if !validRating(rating) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid rating. Must be '1'-'10', '---', or 'Delete'."})
		return
	}
	tracks, err := a.readMapSlice(a.paths.TracksCacheFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	track := findByID(tracks, r.PathValue("track_id"))
	if track == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Track not found"})
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	changed, err := a.updateTrackRating(client, track, rating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func (a *app) handleAddAlbum(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBodyOptional(w, r)
	if !ok {
		return
	}
	mode := normalizePlaylistMode(stringify(body["mode"]))
	listMode := stringify(body["list_mode"])
	if listMode == "" {
		listMode = "album"
	}
	cachePath, err := a.albumCachePath(listMode)
	if err != nil {
		a.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	albums, err := a.readMapSlice(cachePath)
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	album := findByID(albums, r.PathValue("album_id"))
	if album == nil {
		a.writeError(w, r, http.StatusNotFound, "Album not found")
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer client.Close()
	if err := addAlbumsToPlaylist(client, []map[string]any{album}, mode); err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Album added to playlist successfully."})
}

func (a *app) handleAddTrack(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBodyOptional(w, r)
	if !ok {
		return
	}
	mode := normalizePlaylistMode(stringify(body["mode"]))
	tracks, err := a.readMapSlice(a.paths.TracksCacheFile)
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	track := findByID(tracks, r.PathValue("track_id"))
	if track == nil {
		a.writeError(w, r, http.StatusNotFound, "Track not found")
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer client.Close()
	if err := addTracksToPlaylist(client, []map[string]any{track}, mode); err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Track added to playlist successfully."})
}

func (a *app) handleAddAlbums(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody(w, r)
	if !ok {
		return
	}
	ids := stringSlice(body["album_ids"])
	if len(ids) == 0 {
		a.writeError(w, r, http.StatusBadRequest, "album_ids must be a non-empty list")
		return
	}
	mode := normalizePlaylistMode(stringify(body["mode"]))
	listMode := stringify(body["list_mode"])
	if listMode == "" {
		listMode = "album"
	}
	cachePath, err := a.albumCachePath(listMode)
	if err != nil {
		a.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	albums, err := a.readMapSlice(cachePath)
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	selected := findManyByID(albums, ids)
	if len(selected) != len(ids) {
		a.writeError(w, r, http.StatusNotFound, "Some albums not found")
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer client.Close()
	if err := addAlbumsToPlaylist(client, selected, mode); err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("%d albums added to playlist successfully", len(selected))})
}

func (a *app) handleAddTracks(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody(w, r)
	if !ok {
		return
	}
	ids := stringSlice(body["track_ids"])
	if len(ids) == 0 {
		a.writeError(w, r, http.StatusBadRequest, "track_ids must be a non-empty list")
		return
	}
	mode := normalizePlaylistMode(stringify(body["mode"]))
	tracks, err := a.readMapSlice(a.paths.TracksCacheFile)
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	selected := findManyByID(tracks, ids)
	if len(selected) != len(ids) {
		a.writeError(w, r, http.StatusNotFound, "Some tracks not found")
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer client.Close()
	if err := addTracksToPlaylist(client, selected, mode); err != nil {
		a.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("%d tracks added to playlist successfully", len(selected))})
}

func (a *app) handleRandomAlbum(w http.ResponseWriter, r *http.Request) {
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	if err := randomAlbum(client, a.cfg.Random.ArtistTag); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Random album playback started"})
}

func (a *app) handleRandomTracks(w http.ResponseWriter, r *http.Request) {
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	if err := randomTracks(client, a.cfg.Random.ArtistTag, a.cfg.Random.Tracks); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Random tracks playback started"})
}

func (a *app) handleCacheUpdate(w http.ResponseWriter, r *http.Request) {
	if err := a.rebuildCache("api request"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Cache update failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Cache updated"})
}

func (a *app) rebuildCache(reason string) error {
	a.cacheLock.Lock()
	defer a.cacheLock.Unlock()

	a.logger.Printf("cache rebuild: started (%s)", reason)
	if err := a.createCache(); err != nil {
		a.logger.Printf("cache rebuild: failed (%s): %v", reason, err)
		return err
	}
	if err := a.saveCacheState(newCacheState(time.Now())); err != nil {
		a.logger.Printf("cache rebuild: failed to save state (%s): %v", reason, err)
		return err
	}
	a.logger.Printf("cache rebuild: finished (%s)", reason)
	return nil
}

func (a *app) watchMPDDatabaseUpdates() {
	network, address := mpdEndpoint(a.cfg.MPD.Address)
	for {
		watcher, err := mpd.NewWatcher(network, address, "", "database")
		if err != nil {
			a.logger.Printf("mpd watcher: connect failed: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		a.logger.Printf("mpd watcher: watching database updates on %s %s", network, address)
		err = a.consumeMPDWatcher(watcher)
		if closeErr := watcher.Close(); closeErr != nil {
			a.logger.Printf("mpd watcher: close failed: %v", closeErr)
		}
		if err != nil {
			a.logger.Printf("mpd watcher: restarting after error: %v", err)
		} else {
			a.logger.Printf("mpd watcher: restarting after disconnect")
		}
		time.Sleep(2 * time.Second)
	}
}

func (a *app) consumeMPDWatcher(watcher *mpd.Watcher) error {
	for {
		select {
		case event, ok := <-watcher.Event:
			if !ok {
				return nil
			}
			if shouldRefreshForMPDEvent(event) {
				if err := a.rebuildCache("mpd database update"); err != nil {
					a.logger.Printf("mpd watcher: cache rebuild failed after %q event: %v", event, err)
				}
			}
		case err, ok := <-watcher.Error:
			if !ok {
				return nil
			}
			return err
		}
	}
}

func shouldRefreshForMPDEvent(event string) bool {
	return strings.TrimSpace(event) == "database"
}

func (a *app) handleCurrentAlbumRatingGet(w http.ResponseWriter, r *http.Request) {
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	song, err := client.CurrentSong()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(song) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "No song playing"})
		return
	}
	ratings, err := a.loadRatings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	album := map[string]any{
		"albumartist": firstNonEmpty(song["AlbumArtist"], song["Artist"]),
		"album":       song["Album"],
		"date":        firstNonEmpty(song["Date"], "0000"),
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rating":      ratings[albumKey(album)],
		"albumartist": album["albumartist"],
		"album":       album["album"],
		"date":        album["date"],
	})
}

func (a *app) handleCurrentAlbumRatingPost(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody(w, r)
	if !ok {
		return
	}
	rating := stringify(body["rating"])
	if rating == "" {
		rating = "---"
	}
	if !validRating(rating) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid rating"})
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	song, err := client.CurrentSong()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(song) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "No song playing"})
		return
	}
	album := map[string]any{
		"albumartist": firstNonEmpty(song["AlbumArtist"], song["Artist"]),
		"album":       song["Album"],
		"date":        firstNonEmpty(song["Date"], "0000"),
	}
	changed, err := a.updateAlbumRating(album, rating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"changed": changed})
}

func (a *app) handleCurrentTrackRatingGet(w http.ResponseWriter, r *http.Request) {
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	song, err := client.CurrentSong()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(song) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "No song playing"})
		return
	}
	file := song["file"]
	if file == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Current track data incomplete (missing 'file' path)."})
		return
	}
	sticker, err := client.StickerGet(file, "rating")
	if err != nil && !strings.Contains(err.Error(), "No such sticker") {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rating := ""
	if err == nil && sticker != nil {
		rating = sticker.Value
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rating": valueOrNil(rating),
		"title":  song["Title"],
		"artist": song["Artist"],
		"album":  song["Album"],
		"date":   firstNonEmpty(song["Date"], "0000"),
		"file":   file,
	})
}

func (a *app) handleCurrentTrackRatingPost(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody(w, r)
	if !ok {
		return
	}
	rating := stringify(body["rating"])
	if rating == "" {
		rating = "---"
	}
	if !validRating(rating) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid rating. Must be '1'-'10', '---', or 'Delete'."})
		return
	}
	client, err := a.dialMPD()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()
	song, err := client.CurrentSong()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(song) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "No song playing"})
		return
	}
	track := map[string]any{
		"file":   song["file"],
		"title":  song["Title"],
		"artist": song["Artist"],
	}
	if stringify(track["file"]) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Current track data incomplete (missing 'file' path)."})
		return
	}
	changed, err := a.updateTrackRating(client, track, rating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
}

func (a *app) dialMPD() (*mpd.Client, error) {
	network, address := mpdEndpoint(a.cfg.MPD.Address)
	client, err := mpd.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func mpdEndpoint(address string) (network, endpoint string) {
	address = strings.TrimSpace(address)
	if address == "" {
		address = "localhost:6600"
	}
	if shared.IsUnixAddress(address) {
		return "unix", address
	}
	return "tcp", address
}

func defaultBindToAddress() []string {
	return []string{
		"0.0.0.0:6601",
		shared.DefaultSocketPath(),
	}
}

func (a *app) createCache() error {
	client, err := a.dialMPD()
	if err != nil {
		return err
	}
	defer client.Close()

	songs, err := a.fetchAllSongsBatched(client)
	if err != nil {
		return err
	}
	trackRatings, err := a.loadTrackRatingsWithClient(client)
	if err != nil {
		return err
	}

	albums := make([]map[string]any, 0)
	tracks := make([]map[string]any, 0, len(songs))
	latestMap := map[string]map[string]any{}
	seenAlbums := map[string]struct{}{}

	for _, song := range songs {
		albumArtist := firstNonEmpty(song["AlbumArtist"], song["Artist"])
		album := song["Album"]
		date := firstNonEmpty(song["Date"], "0000")
		file := song["file"]
		lastModified := song["Last-Modified"]
		if albumArtist == "" || album == "" || date == "" || file == "" {
			continue
		}

		key := albumArtist + "|||" + album + "|||" + date
		if _, ok := seenAlbums[key]; !ok {
			seenAlbums[key] = struct{}{}
			albums = append(albums, map[string]any{
				"albumartist": albumArtist,
				"album":       album,
				"date":        date,
			})
		}

		tracks = append(tracks, map[string]any{
			"track":       song["Track"],
			"tracknumber": parseNumberTag(song["Track"]),
			"discnumber":  parseNumberTag(firstNonEmpty(song["Disc"], song["DiscNumber"])),
			"title":       song["Title"],
			"artist":      song["Artist"],
			"album":       album,
			"date":        date,
			"file":        file,
			"rating":      valueOrNil(trackRatings[file]),
			"id":          strconv.Itoa(len(tracks)),
		})

		prev := latestMap[key]
		if prev == nil || strings.Compare(lastModified, stringify(prev["last-modified"])) > 0 {
			latestMap[key] = map[string]any{
				"albumartist":   albumArtist,
				"album":         album,
				"date":          date,
				"last-modified": lastModified,
			}
		}
	}

	slices.SortFunc(albums, func(a1, a2 map[string]any) int {
		if c := strings.Compare(strings.ToLower(stringify(a1["albumartist"])), strings.ToLower(stringify(a2["albumartist"]))); c != 0 {
			return c
		}
		if c := strings.Compare(stringify(a1["date"]), stringify(a2["date"])); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(stringify(a1["album"])), strings.ToLower(stringify(a2["album"])))
	})
	for i := range albums {
		albums[i]["id"] = strconv.Itoa(i)
	}

	latest := make([]map[string]any, 0, len(latestMap))
	for _, album := range latestMap {
		latest = append(latest, album)
	}
	slices.SortFunc(latest, func(a1, a2 map[string]any) int {
		return strings.Compare(stringify(a2["last-modified"]), stringify(a1["last-modified"]))
	})
	for i := range latest {
		latest[i]["id"] = strconv.Itoa(i)
	}

	if err := a.writeMapSlice(a.paths.AlbumCacheFile, albums); err != nil {
		return err
	}
	if err := a.writeMapSlice(a.paths.TracksCacheFile, tracks); err != nil {
		return err
	}
	if err := a.writeMapSlice(a.paths.LatestCacheFile, latest); err != nil {
		return err
	}
	return nil
}

func (a *app) fetchAllSongsBatched(client *mpd.Client) ([]mpd.Attrs, error) {
	stats, err := client.Stats()
	if err != nil {
		return nil, err
	}

	totalSongs := intFromAny(stats["songs"], 0)
	if totalSongs <= 0 {
		a.logger.Printf("cache rebuild: MPD reports no songs")
		return []mpd.Attrs{}, nil
	}

	batchSize := a.cfg.Cache.BatchSize
	if batchSize <= 0 {
		batchSize = 10000
	}

	a.logger.Printf("cache rebuild: fetching %d songs in batches of %d", totalSongs, batchSize)

	allSongs := make([]mpd.Attrs, 0, totalSongs)
	for offset := 0; offset < totalSongs; {
		startIndex := offset + 1
		endIndex := offset + batchSize
		window := fmt.Sprintf("%d:%d", startIndex, endIndex)

		batch, err := client.Search("filename", "", "window", window)
		if err != nil {
			return nil, fmt.Errorf("batch search at offset %d: %w", offset, err)
		}
		if len(batch) == 0 {
			a.logger.Printf("cache rebuild: stopped early after %d/%d songs; empty batch for window %s", offset, totalSongs, window)
			break
		}

		allSongs = append(allSongs, batch...)
		offset += len(batch)
		a.logger.Printf("cache rebuild: fetched %d/%d songs", offset, totalSongs)
	}

	return allSongs, nil
}

func (a *app) updateAlbumRating(album map[string]any, rating string) (bool, error) {
	key := albumKey(album)
	if key == "" {
		return false, fmt.Errorf("cannot generate album key")
	}
	ratings, err := a.loadRatings()
	if err != nil {
		return false, err
	}
	current, exists := ratings[key]
	changed := false

	switch rating {
	case "Delete":
		if exists {
			delete(ratings, key)
			changed = true
		}
	case "---":
		return false, nil
	default:
		if current != rating {
			ratings[key] = rating
			changed = true
		}
	}

	if !changed {
		return false, nil
	}
	if err := a.saveRatings(ratings); err != nil {
		return false, err
	}
	return true, nil
}

func (a *app) updateTrackRating(client *mpd.Client, track map[string]any, rating string) (bool, error) {
	file := stringify(track["file"])
	if file == "" {
		return false, fmt.Errorf("missing file key")
	}
	switch rating {
	case "Delete":
		if err := client.StickerDelete(file, "rating"); err != nil {
			return false, err
		}
		if err := a.updateTrackCacheRating(file, ""); err != nil {
			return false, err
		}
		return true, nil
	case "---":
		return false, nil
	default:
		if err := client.StickerSet(file, "rating", rating); err != nil {
			return false, err
		}
		if err := a.updateTrackCacheRating(file, rating); err != nil {
			return false, err
		}
		return true, nil
	}
}

type albumFinder interface {
	Find(args ...string) ([]mpd.Attrs, error)
}

func addAlbumsToPlaylist(client *mpd.Client, albums []map[string]any, mode string) error {
	pos, err := preparePlaylist(client, mode)
	if err != nil {
		return err
	}
	for _, album := range albums {
		attrs, err := findAlbumTracks(client, album)
		if err != nil {
			return err
		}
		slices.SortFunc(attrs, func(a1, a2 mpd.Attrs) int {
			if c := compareInts(parseNumberTag(firstNonEmpty(a1["Disc"], a1["DiscNumber"])), parseNumberTag(firstNonEmpty(a2["Disc"], a2["DiscNumber"]))); c != 0 {
				return c
			}
			if c := compareInts(parseNumberTag(a1["Track"]), parseNumberTag(a2["Track"])); c != 0 {
				return c
			}
			return strings.Compare(a1["Title"], a2["Title"])
		})
		for _, track := range attrs {
			file := track["file"]
			if file == "" {
				continue
			}
			if pos >= 0 {
				if _, err := client.AddID(file, pos); err != nil {
					return err
				}
				pos++
			} else {
				if _, err := client.AddID(file, -1); err != nil {
					return err
				}
			}
		}
	}
	if mode == "replace" || mode == "insert" {
		return client.Play(-1)
	}
	return nil
}

func findAlbumTracks(client albumFinder, album map[string]any) ([]mpd.Attrs, error) {
	artist := stringify(album["albumartist"])
	name := stringify(album["album"])
	date := stringify(album["date"])
	displayArtist := artist
	displayName := name
	displayDate := date
	if displayArtist == "" {
		displayArtist = "Unknown Artist"
	}
	if displayName == "" {
		displayName = "Unknown Album"
	}
	if displayDate == "" {
		displayDate = "0000"
	}

	queries := [][]string{
		{"albumartist", artist, "album", name, "date", date},
	}
	if artist != "" {
		queries = append(queries, []string{"artist", artist, "album", name, "date", date})
	}

	for _, args := range queries {
		attrs, err := client.Find(args...)
		if err != nil {
			return nil, err
		}
		if len(attrs) > 0 {
			return attrs, nil
		}
	}

	return nil, fmt.Errorf("no tracks found in MPD for %s - %s (%s)", displayArtist, displayName, displayDate)
}

func addTracksToPlaylist(client *mpd.Client, tracks []map[string]any, mode string) error {
	pos, err := preparePlaylist(client, mode)
	if err != nil {
		return err
	}
	for _, track := range tracks {
		file := stringify(track["file"])
		if file == "" {
			continue
		}
		if pos >= 0 {
			if _, err := client.AddID(file, pos); err != nil {
				return err
			}
			pos++
		} else {
			if _, err := client.AddID(file, -1); err != nil {
				return err
			}
		}
	}
	if mode == "replace" || mode == "insert" {
		return client.Play(-1)
	}
	return nil
}

func preparePlaylist(client *mpd.Client, mode string) (int, error) {
	switch mode {
	case "replace":
		if err := client.Clear(); err != nil {
			return -1, err
		}
		return -1, nil
	case "insert":
		song, err := client.CurrentSong()
		if err != nil {
			return -1, nil
		}
		pos, err := strconv.Atoi(song["Pos"])
		if err != nil {
			return -1, nil
		}
		return pos + 1, nil
	default:
		return -1, nil
	}
}

func randomAlbum(client *mpd.Client, tag string) error {
	values, err := client.List(tag)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == 0 {
		return fmt.Errorf("no values found for tag %q", tag)
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomValue := filtered[r.Intn(len(filtered))]
	allTracks, err := client.Find(tag, randomValue)
	if err != nil {
		return err
	}
	type albumRef struct {
		Album string
		Date  string
	}
	seen := map[albumRef]struct{}{}
	refs := make([]albumRef, 0)
	for _, track := range allTracks {
		album := track["Album"]
		if album == "" {
			continue
		}
		ref := albumRef{Album: album, Date: firstNonEmpty(track["Date"], "0000")}
		if _, ok := seen[ref]; !ok {
			seen[ref] = struct{}{}
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return fmt.Errorf("no albums found for %q", randomValue)
	}
	ref := refs[r.Intn(len(refs))]
	if err := client.Clear(); err != nil {
		return err
	}
	tracks, err := client.Find(tag, randomValue, "album", ref.Album, "date", ref.Date)
	if err != nil {
		return err
	}
	for _, track := range tracks {
		file := track["file"]
		if file == "" {
			continue
		}
		if _, err := client.AddID(file, -1); err != nil {
			return err
		}
	}
	return client.Play(-1)
}

func randomTracks(client *mpd.Client, tag string, count int) error {
	values, err := client.List(tag)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == 0 {
		return fmt.Errorf("no values found for tag %q", tag)
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(filtered), func(i, j int) { filtered[i], filtered[j] = filtered[j], filtered[i] })
	if count > len(filtered) {
		count = len(filtered)
	}
	if err := client.Clear(); err != nil {
		return err
	}
	for _, value := range filtered[:count] {
		tracks, err := client.Find(tag, value)
		if err != nil {
			return err
		}
		if len(tracks) == 0 {
			continue
		}
		track := tracks[r.Intn(len(tracks))]
		if _, err := client.AddID(track["file"], -1); err != nil {
			return err
		}
	}
	return client.Play(-1)
}

func (a *app) albumCachePath(mode string) (string, error) {
	if mode == "" {
		mode = "album"
	}
	switch mode {
	case "album":
		return a.paths.AlbumCacheFile, nil
	case "latest":
		return a.paths.LatestCacheFile, nil
	default:
		return "", fmt.Errorf("invalid list mode")
	}
}

func (a *app) loadRatings() (map[string]string, error) {
	data, err := os.ReadFile(a.paths.RatingsCacheFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]string{}, nil
	}
	var ratings map[string]string
	if err := msgpack.Unmarshal(data, &ratings); err != nil {
		return nil, err
	}
	if ratings == nil {
		ratings = map[string]string{}
	}
	return ratings, nil
}

func (a *app) loadTrackRatingsWithClient(client *mpd.Client) (map[string]string, error) {
	files, stickers, err := client.StickerFind("", "rating")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such sticker") {
			return map[string]string{}, nil
		}
		return nil, err
	}

	ratings := make(map[string]string, len(files))
	for i, file := range files {
		if file == "" || i >= len(stickers) {
			continue
		}
		ratings[file] = stickers[i].Value
	}
	return ratings, nil
}

func newCacheState(updatedAt time.Time) cacheState {
	updatedAt = updatedAt.UTC()
	return cacheState{
		Version:   updatedAt.UnixNano(),
		UpdatedAt: updatedAt.Format(time.RFC3339Nano),
	}
}

func (a *app) deriveCacheState() (cacheState, error) {
	paths := []string{a.paths.AlbumCacheFile, a.paths.TracksCacheFile, a.paths.LatestCacheFile}
	var newest time.Time
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return cacheState{}, err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		newest = time.Now()
	}
	return newCacheState(newest), nil
}

func (a *app) loadCacheState() (cacheState, error) {
	data, err := os.ReadFile(a.paths.CacheStateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return a.deriveCacheState()
		}
		return cacheState{}, err
	}
	if len(data) == 0 {
		return a.deriveCacheState()
	}
	var state cacheState
	if err := msgpack.Unmarshal(data, &state); err != nil {
		return cacheState{}, err
	}
	if state.Version == 0 || strings.TrimSpace(state.UpdatedAt) == "" {
		return a.deriveCacheState()
	}
	return state, nil
}

func (a *app) saveCacheState(state cacheState) error {
	data, err := msgpack.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(a.paths.CacheStateFile, data, 0o644)
}

func (a *app) applyCacheStateHeaders(w http.ResponseWriter) {
	state, err := a.loadCacheState()
	if err != nil {
		return
	}
	a.writeCacheStateHeaders(w, state)
}

func (a *app) writeCacheStateHeaders(w http.ResponseWriter, state cacheState) {
	w.Header().Set("X-Clerk-Cache-Version", strconv.FormatInt(state.Version, 10))
	w.Header().Set("X-Clerk-Cache-Updated-At", state.UpdatedAt)
	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", state.Version))
}

func (a *app) updateTrackCacheRating(file, rating string) error {
	tracks, err := a.readMapSlice(a.paths.TracksCacheFile)
	if err != nil {
		return err
	}
	changed := false
	for _, track := range tracks {
		if stringify(track["file"]) != file {
			continue
		}
		track["rating"] = valueOrNil(rating)
		changed = true
		break
	}
	if !changed {
		return nil
	}
	return a.writeMapSlice(a.paths.TracksCacheFile, tracks)
}

func (a *app) saveRatings(ratings map[string]string) error {
	data, err := msgpack.Marshal(ratings)
	if err != nil {
		return err
	}
	return os.WriteFile(a.paths.RatingsCacheFile, data, 0o644)
}

func (a *app) readMapSlice(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []map[string]any{}, nil
	}
	var items []map[string]any
	if err := msgpack.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []map[string]any{}
	}
	return items, nil
}

func (a *app) writeMapSlice(path string, items []map[string]any) error {
	data, err := msgpack.Marshal(items)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func attachAlbumRatings(items []map[string]any, ratings map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := cloneMap(item)
		entry["rating"] = ratings[albumKey(item)]
		out = append(out, entry)
	}
	return out
}

func decodeBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Request Content-Type is not 'application/json'"})
		return nil, false
	}
	return decodeBodyOptional(w, r)
}

func decodeBodyOptional(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if r.Body == nil {
		return map[string]any{}, true
	}
	defer r.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if errors.Is(err, http.ErrBodyNotAllowed) {
			return map[string]any{}, true
		}
		if strings.Contains(err.Error(), "EOF") {
			return map[string]any{}, true
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Error parsing request body: " + err.Error()})
		return nil, false
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, true
}

func albumKey(item map[string]any) string {
	artist := stringify(item["albumartist"])
	if artist == "" {
		artist = stringify(item["artist"])
	}
	album := stringify(item["album"])
	date := stringify(item["date"])
	if artist == "" || album == "" || date == "" {
		return ""
	}
	return artist + "|||" + album + "|||" + date
}

func findByID(items []map[string]any, id string) map[string]any {
	for _, item := range items {
		if stringify(item["id"]) == id {
			return item
		}
	}
	return nil
}

func findManyByID(items []map[string]any, ids []string) []map[string]any {
	index := map[string]map[string]any{}
	for _, item := range items {
		index[stringify(item["id"])] = item
	}
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		if item, ok := index[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringify(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizePlaylistMode(mode string) string {
	switch mode {
	case "insert", "replace":
		return mode
	default:
		return "add"
	}
}

func validRating(value string) bool {
	if value == "Delete" || value == "---" {
		return true
	}
	for i := 1; i <= 10; i++ {
		if value == strconv.Itoa(i) {
			return true
		}
	}
	return false
}

func valueOrNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *app) writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	a.logger.Printf("%s %s -> %d: %s", r.Method, r.URL.Path, status, message)
	writeJSON(w, status, map[string]string{"error": message})
}

func stringify(value any) string {
	return shared.Stringify(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseNumberTag(value any) int {
	s := stringify(value)
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	for i, r := range s {
		if r < '0' || r > '9' {
			s = s[:i]
			break
		}
	}
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func getenvDefault(key, fallback string) string {
	return shared.Getenv(key, fallback)
}

func getenvIntDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func splitAndTrim(value, sep string) []string {
	parts := strings.Split(value, sep)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func intFromAny(value any, fallback int) int {
	return shared.IntFromAny(value, fallback)
}

func boolFromAny(value any, fallback bool) bool {
	return shared.BoolFromAny(value, fallback)
}
