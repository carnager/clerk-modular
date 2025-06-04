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

Usage
clerk-rofi -h
clerk-service (and then navigate to http://localhost:5000 or configured address)
