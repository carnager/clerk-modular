# Clerk

Clerk is a little application to quickly add albums or tracks to your mpd playback queue.
It consists of:
- **`clerk_core`**: a core module to handle all functionality.
- **`clerk-rofi`**: an example UI utilizing rofi
- **`clerk-service`**: a service that serves a REST API plus an example webpage.
- **`clerk-musiclist`**: a little script that creates a searchable webpage, which uses clerk's cache files to generate album lists with album ratings.

## Installation

to be considered...

## Configuration
The core module and clerk-rofi tool expect configuration files at ~/.config/clerk/clerk-core.conf and ~/.config/clerk/clerk/clerk-rofi.conf.
A default configuration will be generated if it doesn't exist.

## Usage

### clerk-rofi

- **`clerk-rofi -h`**: Show help
- **`clerk-rofi -a, -l`**: Add/Insert/Replace album(s)
- **`clerk-rofi -t`**: Add/Insert/Replace track(s)
- **`clerk-rofi -A`**: Play random album
- **`clerk-rofi -T`**: Play random tracks
- **`clerk-rofi -u`**: Update clerk caches
- **`clerk-rofi -c`**: Change rating of currently running album
- **`clerk-rofi -x`**: Regenerate default config

### clerk-service

Run clerk-service

#### Environment variables:
- CLERK_WEB_HOST
- CLERK_WEB_PORT
- CLERK_WEB_PUBLIC_DIR

### clerk-musiclist

Run clerk-musiclist

#### Environment variables:
- CLERK_SYNC_HOST
- CLERK_SYNC_PATH
