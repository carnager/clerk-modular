# Clerk

Clerk is an API-first MPD queue and rating tool.

It consists of:

- `clerkd`: the daemon that talks to MPD, builds caches, stores album ratings, and exposes the HTTP API
- `clerk-rofi`: an interactive API client for browsing, queueing, and rating music through rofi
- `clerk-musiclist`: an API client that renders a static HTML music list and uploads it with `scp`

## Repository Layout

- `clerkd/`: daemon source and systemd user unit
- `cmd/clerk-rofi/`: rofi client
- `cmd/clerk-musiclist/`: music list exporter

## Architecture

`clerkd` is the center of the system.

It connects to MPD, builds local msgpack caches, and serves the Clerk API over:

- one or more configured bind addresses
- the default config listens on both `0.0.0.0:6601` and `$XDG_RUNTIME_DIR/clerk/clerkd.sock`

The two supported client modes are:

- local: use `api.address = "local"` or a Unix socket path
- remote: use `api.address = "host:port"`

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
- `~/.local/share/clerk/cache.state`

XDG base directory variables are respected:

- `XDG_CONFIG_HOME`
- `XDG_DATA_HOME`
- `XDG_RUNTIME_DIR`

### `clerkd`

Important environment variables:

- `CLERKD_BIND_TO_ADDRESS`
- `CLERKD_MPD_ADDRESS`

Default daemon config:

```toml
[server]
bind_to_address = ["0.0.0.0:6601", "/run/user/1000/clerk/clerkd.sock"]

[mpd]
address = "localhost:6600"

[random]
tracks = 20
artist_tag = "albumartist"

[cache]
batch_size = 10000
```

Notes:

- `server.bind_to_address` is the primary listener config
- TCP listeners use `host:port` entries
- Unix socket listeners use filesystem paths
- `mpd.address` accepts either `host:port` or a Unix socket path
- `cache.batch_size` controls batched MPD library fetches during cache rebuilds
- `clerkd` watches MPD idle `database` events and rebuilds its caches automatically after MPD finishes a database update
- album ratings are stored by Clerk in `ratings.cache`
- track ratings are stored in MPD stickers and mirrored into `tracks.cache`
- `cache.state` tracks the current cache version and update timestamp for clients

### `clerk-rofi`

Default client config:

```toml
[api]
address = "local"

[autostart]
enabled = true
systemd_unit = "clerkd.service"
command = ["clerkd"]
timeout_seconds = 5.0

[ui]
menu = ["rofi", "-dmenu", "-p", "Clerk"]

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

- `api.address` accepts:
  - `local` for the default Clerk Unix socket
  - a Unix socket path
  - `host:port` for HTTP API access
- `ui.menu` is the full menu command and is executed as configured
- when the target is local, `clerk-rofi` can auto-start `clerkd`

### `clerk-musiclist`

Default exporter config:

```toml
[api]
address = "local"

[upload]
host = ""
path = ""

[output]
temp_file = "/tmp/musiclist_albums_only.html"
```

Notes:

- `api.address` accepts:
  - `local` for the default Clerk Unix socket
  - a Unix socket path
  - `host:port` for HTTP API access
- `upload.host` and `upload.path` must be set explicitly before publishing

## API

Default local API address:

```text
local
```

Remote example:

```text
musicbox:6601
```

The daemon exposes endpoints under `/api/v1`, including:

- `GET /health`
- `GET /albums`
- `GET /latest_albums`
- `GET /tracks`
- `GET /cache/status`
- album rating endpoints
- track rating endpoints
- playlist add/insert/replace endpoints
- random playback endpoints
- `POST /cache/update`

Client refresh contract:

- `GET /cache/status` returns the current cache `version` and `updated_at`
- `GET /albums`, `GET /latest_albums`, and `GET /tracks` also return:
  - `X-Clerk-Cache-Version`
  - `X-Clerk-Cache-Updated-At`
  - `ETag`
- clients should compare cache `version` values and re-fetch list data when the version changes

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

Nix flake builds:

```bash
nix build .#clerkd
nix build .#clerk-rofi
nix build .#clerk-musiclist
nix build
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
clerk-rofi --api-address local -a
clerk-rofi --api-address musicbox:6601 -a
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
