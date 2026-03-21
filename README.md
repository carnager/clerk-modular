# Clerk

Clerk is an API-first MPD queue and rating tool.

## Repository Layout

- `clerkd/`: daemon source and systemd user unit
- `cmd/clerk-rofi/`: rofi client
- `cmd/clerk-musiclist/`: cache-to-HTML exporter

## Configuration

- `~/.config/clerk/clerkd.toml`: daemon configuration
- `~/.config/clerk/clerk-rofi.toml`: rofi client configuration

Both files are created automatically with defaults if they do not exist.

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

It does not trigger publishing or export jobs. `clerk-musiclist` is a separate command.

Important environment variables:

- `CLERKD_HOST`
- `CLERKD_PORT`
- `CLERKD_MPD_ADDRESS`

The packaged systemd user unit is [clerkd.service](/home/carnager/clerk-modular/clerkd/clerkd.service).

## Rofi Client

`clerk-rofi` is API-only. The default `api.base_url` is `http://localhost:6601/api/v1`.

Running `clerk-rofi -x` rewrites `~/.config/clerk/clerk-rofi.toml` with the default config.

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
