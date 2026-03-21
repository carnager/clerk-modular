# Clerk

Clerk is an API-first MPD queue and rating tool.

## Repository Layout

- `clerkd/`: daemon source and systemd user unit
- `cmd/clerk-rofi/`: rofi client
- `cmd/clerk-musiclist/`: cache-to-HTML exporter

## Configuration

- `~/.config/clerk/clerkd.conf`: daemon configuration
- `~/.config/clerk/clerk-rofi.conf`: rofi client configuration

`clerkd` will still read `~/.config/clerk/clerk-core.conf` if `clerkd.conf` does not exist yet.

## Development

Build all binaries into `bin/`:

```bash
./build
```

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

## Daemon

`clerkd` is API-only.

Important environment variables:

- `CLERK_WEB_HOST`
- `CLERK_WEB_PORT`
- `MPD_HOST`

The packaged systemd user unit is [clerkd.service](/home/carnager/clerk-modular/clerkd/clerkd.service).

## Rofi Client

`clerk-rofi` is API-only. The default `api_base_url` is `http://localhost:6601/api/v1`.

When the base URL is the implicit local default, `clerk-rofi` can auto-start the daemon through:

- `systemctl --user start clerkd.service`
- `clerkd`

Useful commands:

- `clerk-rofi -a`: add albums
- `clerk-rofi -l`: add latest albums
- `clerk-rofi -t`: add tracks
- `clerk-rofi -A`: random album
- `clerk-rofi -T`: random tracks
- `clerk-rofi -c`: rate current track or album
- `clerk-rofi -u`: rebuild caches
- `clerk-rofi -x`: regenerate the client config

## Music List Exporter

`clerk-musiclist` reads Clerk cache files, writes a static HTML page, and uploads it with `scp`.

Important environment variables:

- `CLERK_SYNC_HOST`
- `CLERK_SYNC_PATH`

## API Base URL

Default local API base URL:

```text
http://localhost:6601/api/v1
```

## Packaging

The provided `PKGBUILD` builds and packages:

- `clerkd`
- `clerk-rofi`
- `clerk-musiclist`
