# Clerk

Clerk is an API-first MPD queue and rating tool.

It consists of:

- `clerkd`: the daemon that talks to MPD, builds caches, stores album ratings, and exposes the HTTP API
- `clerk-rofi`: an interactive rofi client for browsing, queueing, and rating music
- `clerk-musiclist`: a static HTML exporter that reads album data from the Clerk API and uploads the result with `scp`

## Repository Layout

- `clerkd/`: daemon source and systemd user unit
- `cmd/clerk-rofi/`: rofi client
- `cmd/clerk-musiclist/`: music list exporter

## Architecture

`clerkd` is the center of the system.

It connects to MPD, builds local msgpack caches, and serves the Clerk API over:

- TCP, by default on `0.0.0.0:6601`
- a Unix socket, by default at `$XDG_RUNTIME_DIR/clerk/clerkd.sock`

The two supported client modes are:

- local: use `api.base_url = "local"` to talk to the daemon over the Unix socket
- remote: use a normal HTTP URL such as `http://host:6601/api/v1`

## Configuration

Clerk creates default config files automatically when they do not exist.

Config files:

- `~/.config/clerk/clerkd.toml`
- `~/.config/clerk/clerk-rofi.toml`
- `~/.config/clerk/clerk-musiclist.toml`

Data files written by the daemon:

- `~/.local/share/clerk/album.cache`
- `~/.local/share/clerk/tracks.cache`
- `~/.local/share/clerk/latest.cache`
- `~/.local/share/clerk/ratings.cache`

XDG base directory variables are respected:

- `XDG_CONFIG_HOME`
- `XDG_DATA_HOME`
- `XDG_RUNTIME_DIR`

### `clerkd`

Important environment variables:

- `CLERKD_HOST`
- `CLERKD_PORT`
- `CLERKD_SOCKET_PATH`
- `CLERKD_MPD_HOST`
- `CLERKD_MPD_PORT`
- `CLERKD_MPD_ADDRESS`

Default daemon config:

```toml
[server]
host = "0.0.0.0"
port = 6601

[mpd]
host = "localhost"
port = 6600

[random]
tracks = 20
artist_tag = "albumartist"

[cache]
batch_size = 10000
```

Notes:

- `server.socket_path` is optional; when omitted Clerk uses the default runtime socket path
- `mpd.host` and `mpd.port` are the primary MPD connection settings
- `mpd.address` and `CLERKD_MPD_ADDRESS` are still accepted as compatibility fallbacks
- `cache.batch_size` controls batched MPD library fetches during cache rebuilds
- album ratings are stored by Clerk in `ratings.cache`
- track ratings are stored in MPD stickers and mirrored into `tracks.cache`

### `clerk-rofi`

Default client config:

```toml
[api]
base_url = "local"

[autostart]
enabled = true
systemd_unit = "clerkd.service"
command = ["clerkd"]
timeout_seconds = 5.0

[ui]
prompt = "Clerk"
menu = ["rofi", "-dmenu", "-p", "PLACEHOLDER"]

[columns]
artist = 30
album_artist = 30
date = 6
album = 40
id = 5
title = 40
track = 5
```

Notes:

- `api.base_url = "local"` uses the Unix socket transport
- a remote daemon uses a normal HTTP API URL instead
- when the target is local, `clerk-rofi` can auto-start `clerkd`

### `clerk-musiclist`

Default exporter config:

```toml
[api]
base_url = "local"

[upload]
host = "proteus"
path = "/srv/http/list"

[output]
temp_file = "/tmp/musiclist_albums_only.html"
```

Notes:

- `clerk-musiclist` reads album data from the Clerk API, not from local cache files
- `api.base_url = "local"` uses the Unix socket transport
- remote exports use a normal HTTP API URL instead

## API

Default local API value:

```text
local
```

Equivalent remote-style local HTTP URL:

```text
http://localhost:6601/api/v1
```

The daemon exposes endpoints under `/api/v1`, including:

- `GET /health`
- `GET /albums`
- `GET /latest_albums`
- `GET /tracks`
- album rating endpoints
- track rating endpoints
- playlist add/insert/replace endpoints
- random playback endpoints
- `POST /cache/update`

## Building

Build all binaries into `bin/`:

```bash
./build
```

Direct builds:

```bash
go build ./clerkd
go build ./cmd/clerk-rofi
go build ./cmd/clerk-musiclist
```

## Running

Run the daemon from the repository root:

```bash
go run ./clerkd
```

Run the rofi client:

```bash
go run ./cmd/clerk-rofi -a
```

Run the music list exporter:

```bash
go run ./cmd/clerk-musiclist
```

## `clerk-rofi` Commands

Useful options:

- `-a`: add albums
- `-l`: add latest albums
- `-t`: add tracks
- `-A`: random album
- `-T`: random tracks
- `-c`: rate current album or current track
- `-u`: rebuild caches
- `-x`: regenerate the default config
- `-h`: show help

You can also override the API target at runtime:

```bash
clerk-rofi --api-base-url local -a
clerk-rofi --api-base-url http://musicbox:6601/api/v1 -a
```

## Local Service

The packaged systemd user unit is [clerkd.service](/home/carnager/clerk-modular/clerkd/clerkd.service).

When installed as a user service:

```bash
systemctl --user enable --now clerkd.service
```

## Packaging

The provided `PKGBUILD` builds and packages:

- `clerkd`
- `clerk-rofi`
- `clerk-musiclist`
