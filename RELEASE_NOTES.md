## Highlights

- Added local Unix socket transport for `clerkd`
- Introduced `api.base_url = "local"` for local clients
- Kept remote HTTP API usage fully supported
- Fixed album/track rating edge cases
- Reworked track ratings to live in `tracks.cache`
- Rewrote the README around the current project state

## Added

- Unix socket listener in `clerkd`
- `CLERKD_SOCKET_PATH` support
- Shared local transport helpers in `internal/shared`
- Tests for:
  - local API resolution in `clerk-rofi`
  - track cache rating updates in `clerkd`

## Changed

- `clerk-rofi` now defaults to `api.base_url = "local"`
- `clerk-musiclist` now defaults to `api.base_url = "local"`
- `clerk-musiclist` reads from the Clerk API instead of assuming shared local cache files
- `clerkd` now respects `cache.batch_size` during cache rebuilds
- Track ratings are cached in `tracks.cache` during rebuild and updated on write
- README rewritten to document the current architecture, config, and usage

## Fixed

- Latest-album rating now resolves against the correct cache/list mode
- Track ratings now show up correctly in track listings
- `clerk-rofi` local autodetection now works for loopback URLs beyond one exact hardcoded value
- Rofi selection mapping no longer depends on brittle whitespace parsing

## Verification

- `go test ./...`
- `go build -o /tmp/clerkd-test ./clerkd`
- `go build -o /tmp/clerk-rofi-test ./cmd/clerk-rofi`
- `go build -o /tmp/clerk-musiclist-test ./cmd/clerk-musiclist`
