package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultLocalAPIBaseURL = "http://localhost:6601/api/v1"

type config struct {
	General struct {
		MenuPrompt           string   `toml:"menu_prompt"`
		MenuTool             []string `toml:"menu_tool"`
		APIBaseURL           string   `toml:"api_base_url"`
		AutoStartLocalDaemon bool     `toml:"auto_start_local_daemon"`
		LocalServiceUnit     string   `toml:"local_service_unit"`
		LocalServiceCommand  []string `toml:"local_service_command"`
		StartupTimeout       float64  `toml:"startup_timeout_seconds"`
	} `toml:"general"`
	Columns struct {
		ArtistWidth      int `toml:"artist_width"`
		AlbumArtistWidth int `toml:"albumartist_width"`
		DateWidth        int `toml:"date_width"`
		AlbumWidth       int `toml:"album_width"`
		IDWidth          int `toml:"id_width"`
		TitleWidth       int `toml:"title_width"`
		TrackWidth       int `toml:"track_width"`
	} `toml:"columns"`
}

type album struct {
	ID          string `json:"id"`
	AlbumArtist string `json:"albumartist"`
	Album       string `json:"album"`
	Date        string `json:"date"`
	Rating      any    `json:"rating"`
}

type track struct {
	ID     string `json:"id"`
	Track  any    `json:"track"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Date   string `json:"date"`
	Rating any    `json:"rating"`
}

type currentAlbum struct {
	Rating      any    `json:"rating"`
	AlbumArtist string `json:"albumartist"`
	Album       string `json:"album"`
}

type currentTrack struct {
	Rating any    `json:"rating"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
}

type apiClient struct {
	baseURL              string
	autoStartLocalDaemon bool
	localServiceUnit     string
	localServiceCommand  []string
	startupTimeout       time.Duration
	httpClient           *http.Client
	autostartAttempted   bool
}

func main() {
	cfg, cfgPath, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	var (
		optAlbums    = flag.Bool("a", false, "")
		optLatest    = flag.Bool("l", false, "")
		optTracks    = flag.Bool("t", false, "")
		optRandomA   = flag.Bool("A", false, "")
		optRandomT   = flag.Bool("T", false, "")
		optCurrent   = flag.Bool("c", false, "")
		optUpdate    = flag.Bool("u", false, "")
		optRegen     = flag.Bool("x", false, "")
		optHelp      = flag.Bool("h", false, "")
		apiBaseURL   = flag.String("api-base-url", "", "")
		noAutostart  = flag.Bool("no-auto-start-local-daemon", false, "")
	)
	flag.Parse()

	effectiveURL, implicitLocal := resolveAPIBaseURL(cfg, *apiBaseURL)
	if *optHelp || !(*optAlbums || *optLatest || *optTracks || *optRandomA || *optRandomT || *optCurrent || *optUpdate || *optRegen) {
		fmt.Print(helpText(effectiveURL, implicitLocal))
		return
	}
	if *optRegen {
		fmt.Printf("To regenerate %s, delete the file manually.\n", cfgPath)
		return
	}

	client := newAPIClient(cfg, effectiveURL, implicitLocal && !*noAutostart)
	if err := client.ensureAvailable(); err != nil {
		fatal(err)
	}

	switch {
	case *optAlbums:
		if err := addAlbumUI(cfg, client, "album"); err != nil {
			fatal(err)
		}
	case *optLatest:
		if err := addAlbumUI(cfg, client, "latest"); err != nil {
			fatal(err)
		}
	case *optTracks:
		if err := addTrackUI(cfg, client); err != nil {
			fatal(err)
		}
	case *optRandomA:
		if err := client.post("playback/random/album", nil, nil); err != nil {
			fatal(err)
		}
	case *optRandomT:
		if err := client.post("playback/random/tracks", nil, nil); err != nil {
			fatal(err)
		}
	case *optCurrent:
		if err := currentTrackUI(cfg, client); err != nil {
			fatal(err)
		}
	case *optUpdate:
		if err := client.post("cache/update", nil, nil); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func loadConfig() (config, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, "", err
	}
	xdgConfig := getenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	confDir := filepath.Join(xdgConfig, "clerk")
	confPath := filepath.Join(confDir, "clerk-rofi.conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return config{}, "", err
	}
	if _, err := os.Stat(confPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(confPath, []byte(defaultConfigText()), 0o644); err != nil {
			return config{}, "", err
		}
	}

	var raw map[string]any
	if _, err := toml.DecodeFile(confPath, &raw); err != nil {
		return config{}, "", err
	}
	var cfg config
	general, _ := raw["general"].(map[string]any)
	columns, _ := raw["columns"].(map[string]any)
	cfg.General.MenuPrompt = stringify(general["menu_prompt"])
	cfg.General.MenuTool = stringSlice(general["menu_tool"])
	cfg.General.APIBaseURL = stringify(general["api_base_url"])
	cfg.General.AutoStartLocalDaemon = boolFromAny(general["auto_start_local_daemon"], true)
	cfg.General.LocalServiceUnit = stringify(general["local_service_unit"])
	cfg.General.LocalServiceCommand = stringSlice(general["local_service_command"])
	cfg.General.StartupTimeout = floatFromAny(general["startup_timeout_seconds"], 5.0)
	cfg.Columns.ArtistWidth = intFromAny(columns["artist_width"], 30)
	cfg.Columns.AlbumArtistWidth = intFromAny(columns["albumartist_width"], 30)
	cfg.Columns.DateWidth = intFromAny(columns["date_width"], 6)
	cfg.Columns.AlbumWidth = intFromAny(columns["album_width"], 40)
	cfg.Columns.IDWidth = intFromAny(columns["id_width"], 5)
	cfg.Columns.TitleWidth = intFromAny(columns["title_width"], 40)
	cfg.Columns.TrackWidth = intFromAny(columns["track_width"], 5)
	applyDefaults(&cfg)
	return cfg, confPath, nil
}

func defaultConfigText() string {
	return `[general]
menu_prompt = "Clerk"
menu_tool = ["rofi", "-dmenu", "-p", "PLACEHOLDER"]
api_base_url = "http://localhost:6601/api/v1"
auto_start_local_daemon = true
local_service_unit = "clerkd.service"
local_service_command = []
startup_timeout_seconds = 5.0

[columns]
artist_width = 30
albumartist_width = 30
date_width = 6
album_width = 40
id_width = 5
title_width = 40
track_width = 5
`
}

func applyDefaults(cfg *config) {
	if cfg.General.MenuPrompt == "" {
		cfg.General.MenuPrompt = "Clerk"
	}
	if len(cfg.General.MenuTool) == 0 {
		cfg.General.MenuTool = []string{"rofi", "-dmenu", "-p", "PLACEHOLDER"}
	}
	if cfg.General.APIBaseURL == "" {
		cfg.General.APIBaseURL = defaultLocalAPIBaseURL
	}
	if cfg.General.LocalServiceUnit == "" {
		cfg.General.LocalServiceUnit = "clerkd.service"
	}
	if cfg.General.StartupTimeout <= 0 {
		cfg.General.StartupTimeout = 5
	}
	if len(cfg.General.LocalServiceCommand) == 0 {
		cfg.General.LocalServiceCommand = []string{"clerkd"}
	}
	if cfg.Columns.ArtistWidth == 0 {
		cfg.Columns.ArtistWidth = 30
	}
	if cfg.Columns.AlbumArtistWidth == 0 {
		cfg.Columns.AlbumArtistWidth = 30
	}
	if cfg.Columns.DateWidth == 0 {
		cfg.Columns.DateWidth = 6
	}
	if cfg.Columns.AlbumWidth == 0 {
		cfg.Columns.AlbumWidth = 40
	}
	if cfg.Columns.IDWidth == 0 {
		cfg.Columns.IDWidth = 5
	}
	if cfg.Columns.TitleWidth == 0 {
		cfg.Columns.TitleWidth = 40
	}
	if cfg.Columns.TrackWidth == 0 {
		cfg.Columns.TrackWidth = 5
	}
}

func resolveAPIBaseURL(cfg config, override string) (string, bool) {
	base := strings.TrimSpace(override)
	if base == "" {
		base = strings.TrimSpace(cfg.General.APIBaseURL)
	}
	if base == "" {
		base = defaultLocalAPIBaseURL
	}
	base = strings.TrimRight(base, "/")
	return base, base == defaultLocalAPIBaseURL
}

func helpText(apiBaseURL string, autoStart bool) string {
	auto := "no"
	if autoStart {
		auto = "yes"
	}
	return fmt.Sprintf(`Usage: clerk-rofi [option] [--api-base-url URL] [--no-auto-start-local-daemon]
 -a  Add Albums
 -l  Add Latest Albums
 -t  Add Tracks
 -A  Random Album
 -T  Random Tracks
 -c  Rate Current Track/Album
 -u  Rebuild Caches
 -x  Regenerate UI Config
 -h  Show This Help

Defaults:
 api_base_url = %s
 auto_start_local_daemon = %s
`, apiBaseURL, auto)
}

func newAPIClient(cfg config, baseURL string, autoStart bool) *apiClient {
	return &apiClient{
		baseURL:              baseURL,
		autoStartLocalDaemon: autoStart && cfg.General.AutoStartLocalDaemon,
		localServiceUnit:     cfg.General.LocalServiceUnit,
		localServiceCommand:  cfg.General.LocalServiceCommand,
		startupTimeout:       time.Duration(cfg.General.StartupTimeout * float64(time.Second)),
		httpClient:           &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *apiClient) ensureAvailable() error {
	if !c.autoStartLocalDaemon {
		return nil
	}
	if c.healthcheck() {
		return nil
	}
	if err := c.ensureLocalService(); err != nil {
		return fmt.Errorf("local Clerk daemon is not reachable at %s and could not be started: %w", c.baseURL, err)
	}
	return nil
}

func (c *apiClient) healthcheck() bool {
	req, _ := http.NewRequest(http.MethodGet, c.baseURL+"/health", nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (c *apiClient) ensureLocalService() error {
	if c.autostartAttempted {
		return errors.New("daemon start already attempted")
	}
	c.autostartAttempted = true
	if err := c.startWithSystemd(); err != nil {
		if err := c.startWithCommand(); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(c.startupTimeout)
	for time.Now().Before(deadline) {
		if c.healthcheck() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("startup timed out")
}

func (c *apiClient) startWithSystemd() error {
	if c.localServiceUnit == "" {
		return errors.New("no systemd unit configured")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return err
	}
	cmd := exec.Command("systemctl", "--user", "start", c.localServiceUnit)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func (c *apiClient) startWithCommand() error {
	if len(c.localServiceCommand) == 0 {
		return errors.New("no local service command configured")
	}
	cmd := exec.Command(c.localServiceCommand[0], c.localServiceCommand[1:]...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	return cmd.Start()
}

func (c *apiClient) get(path string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, c.baseURL+"/"+strings.TrimLeft(path, "/"), nil)
	return c.do(req, out, true)
}

func (c *apiClient) post(path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(http.MethodPost, c.baseURL+"/"+strings.TrimLeft(path, "/"), reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, out, true)
}

func (c *apiClient) do(req *http.Request, out any, retryOnConnectError bool) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if retryOnConnectError && c.autoStartLocalDaemon {
			if startErr := c.ensureLocalService(); startErr == nil {
				clone := req.Clone(req.Context())
				if req.Body != nil && req.GetBody != nil {
					body, _ := req.GetBody()
					clone.Body = body
				}
				return c.do(clone, out, false)
			}
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d for %s - %s", resp.StatusCode, req.URL.String(), strings.TrimSpace(string(body)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func addAlbumUI(cfg config, client *apiClient, mode string) error {
	var albums []album
	endpoint := "albums"
	if mode == "latest" {
		endpoint = "latest_albums"
	}
	if err := client.get(endpoint, &albums); err != nil {
		return err
	}
	if len(albums) == 0 {
		return errors.New("no albums available")
	}
	lines := make([]string, 0, len(albums))
	for _, album := range albums {
		lines = append(lines, formatAlbumLine(cfg, album))
	}
	selectedIDs, err := runMenu(cfg, lines, true, cfg.General.MenuPrompt)
	if err != nil || len(selectedIDs) == 0 {
		return err
	}
	action, err := runSingleMenu(cfg, []string{"Add", "Insert", "Replace", "Rate"}, cfg.General.MenuPrompt)
	if err != nil || action == "" {
		return err
	}
	selected := filterAlbums(albums, selectedIDs)
	if action == "Rate" {
		for _, album := range selected {
			rating, err := inputRating(cfg, fmt.Sprintf("%s - %s", fallback(album.AlbumArtist, "N/A"), fallback(album.Album, "N/A")))
			if err != nil {
				return err
			}
			if err := client.post("albums/"+album.ID+"/rating", map[string]string{"rating": rating}, nil); err != nil {
				return err
			}
		}
		return nil
	}
	payload := map[string]any{
		"mode":      strings.ToLower(action),
		"list_mode": mode,
	}
	if len(selected) == 1 {
		return client.post("playlist/add/album/"+selected[0].ID, payload, nil)
	}
	ids := make([]string, 0, len(selected))
	for _, album := range selected {
		ids = append(ids, album.ID)
	}
	payload["album_ids"] = ids
	return client.post("playlist/add/albums", payload, nil)
}

func addTrackUI(cfg config, client *apiClient) error {
	var tracks []track
	if err := client.get("tracks", &tracks); err != nil {
		return err
	}
	if len(tracks) == 0 {
		return errors.New("no tracks available")
	}
	lines := make([]string, 0, len(tracks))
	for _, track := range tracks {
		lines = append(lines, formatTrackLine(cfg, track))
	}
	selectedIDs, err := runMenu(cfg, lines, true, cfg.General.MenuPrompt)
	if err != nil || len(selectedIDs) == 0 {
		return err
	}
	action, err := runSingleMenu(cfg, []string{"Add", "Insert", "Replace", "Rate Track (MPD Sticker)"}, cfg.General.MenuPrompt)
	if err != nil || action == "" {
		return err
	}
	selected := filterTracks(tracks, selectedIDs)
	if action == "Rate Track (MPD Sticker)" {
		for _, track := range selected {
			rating, err := inputRating(cfg, fmt.Sprintf("%s - %s", fallback(track.Artist, "Unknown Artist"), fallback(track.Title, "Unknown Title")))
			if err != nil {
				return err
			}
			if err := client.post("tracks/"+track.ID+"/rating", map[string]string{"rating": rating}, nil); err != nil {
				return err
			}
		}
		return nil
	}
	payload := map[string]any{"mode": strings.ToLower(action)}
	if len(selected) == 1 {
		return client.post("playlist/add/track/"+selected[0].ID, payload, nil)
	}
	ids := make([]string, 0, len(selected))
	for _, track := range selected {
		ids = append(ids, track.ID)
	}
	payload["track_ids"] = ids
	return client.post("playlist/add/tracks", payload, nil)
}

func currentTrackUI(cfg config, client *apiClient) error {
	action, err := runSingleMenu(cfg, []string{"Rate Album", "Rate Track (MPD Sticker)"}, cfg.General.MenuPrompt)
	if err != nil || action == "" {
		return err
	}
	switch action {
	case "Rate Album":
		var current currentAlbum
		if err := client.get("current_album/rating", &current); err != nil {
			return err
		}
		rating, err := inputRating(cfg, fmt.Sprintf("%s - %s", fallback(current.AlbumArtist, "Unknown Artist"), fallback(current.Album, "Unknown Album")))
		if err != nil {
			return err
		}
		return client.post("current_album/rating", map[string]string{"rating": rating}, nil)
	case "Rate Track (MPD Sticker)":
		var current currentTrack
		if err := client.get("current_track/rating", &current); err != nil {
			return err
		}
		rating, err := inputRating(cfg, fmt.Sprintf("%s - %s", fallback(current.Artist, "Unknown Artist"), fallback(current.Title, "Unknown Title")))
		if err != nil {
			return err
		}
		return client.post("current_track/rating", map[string]string{"rating": rating}, nil)
	default:
		return nil
	}
}

func runMenu(cfg config, lines []string, trim bool, prompt string) ([]string, error) {
	cmdArgs := make([]string, len(cfg.General.MenuTool))
	copy(cmdArgs, cfg.General.MenuTool)
	for i, arg := range cmdArgs {
		cmdArgs[i] = strings.ReplaceAll(arg, "PLACEHOLDER", prompt)
	}
	if len(cmdArgs) == 0 {
		return nil, errors.New("menu_tool is empty")
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	_, _ = io.WriteString(stdin, strings.Join(lines, "\n"))
	_ = stdin.Close()
	err = cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	outLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(outLines) == 1 && outLines[0] == "" {
		return nil, nil
	}
	if trim {
		ids := make([]string, 0, len(outLines))
		for _, line := range outLines {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ids = append(ids, parts[len(parts)-2])
			}
		}
		return ids, nil
	}
	return outLines[:1], nil
}

func runSingleMenu(cfg config, lines []string, prompt string) (string, error) {
	selected, err := runMenu(cfg, lines, false, prompt)
	if err != nil || len(selected) == 0 {
		return "", err
	}
	return strings.TrimSpace(selected[0]), nil
}

func inputRating(cfg config, prompt string) (string, error) {
	result, err := runMenu(cfg, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "---", "Delete"}, false, prompt+" "+cfg.General.MenuPrompt)
	if err != nil || len(result) == 0 || strings.TrimSpace(result[0]) == "" {
		return "---", err
	}
	return strings.TrimSpace(result[0]), nil
}

func formatAlbumLine(cfg config, a album) string {
	return fmt.Sprintf("%-*s %-*s %-*s %-*s r=%s",
		cfg.Columns.AlbumArtistWidth, a.AlbumArtist,
		cfg.Columns.DateWidth, fallback(a.Date, "0000"),
		cfg.Columns.AlbumWidth, a.Album,
		cfg.Columns.IDWidth, a.ID,
		ratingString(a.Rating),
	)
}

func formatTrackLine(cfg config, t track) string {
	return fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s r=%s",
		cfg.Columns.TrackWidth, trackNumberString(t.Track),
		cfg.Columns.TitleWidth, t.Title,
		cfg.Columns.ArtistWidth, t.Artist,
		cfg.Columns.AlbumWidth, t.Album,
		cfg.Columns.DateWidth, fallback(t.Date, "0000"),
		cfg.Columns.IDWidth, t.ID,
		ratingString(t.Rating),
	)
}

func filterAlbums(items []album, ids []string) []album {
	index := make(map[string]album, len(items))
	for _, item := range items {
		index[item.ID] = item
	}
	out := make([]album, 0, len(ids))
	for _, id := range ids {
		if item, ok := index[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func filterTracks(items []track, ids []string) []track {
	index := make(map[string]track, len(items))
	for _, item := range items {
		index[item.ID] = item
	}
	out := make([]track, 0, len(ids))
	for _, id := range ids {
		if item, ok := index[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func ratingString(value any) string {
	switch v := value.(type) {
	case nil:
		return "-"
	case string:
		if v == "" {
			return "-"
		}
		return v
	case float64:
		return strconv.Itoa(int(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func trackNumberString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		if len(v) == 0 {
			return ""
		}
		return stringify(v[0])
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if float64(int(v)) == v {
			return strconv.Itoa(int(v))
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringify(item))
		}
		return out
	default:
		return nil
	}
}

func intFromAny(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return fallback
}

func floatFromAny(value any, fallback float64) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return n
		}
	}
	return fallback
}

func boolFromAny(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func fallback(value, alt string) string {
	if strings.TrimSpace(value) == "" {
		return alt
	}
	return value
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
