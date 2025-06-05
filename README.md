# Clerk

Clerk is a little application to quickly add albums or tracks to your mpd playback queue.
It consists of:
- **`clerk_core`**: a core module to handle all functionality.
- **`clerk-rofi`**: an example UI utilizing rofi using clerk_core directly
- **`clerk-service`**: a service that serves a REST API plus an example webpage.
- **`clerk-api-rofi`**: an example UI utilizing rofi using the REST API of clerk-service
- **`clerk-musiclist`**: a little script that creates a searchable webpage, which uses clerk's cache files to generate album lists with album ratings.

## Installation

### Arch Linux:
- use the provided PKGBUILD

### Others:
Note for packagers: clerk-rofi and clerk-service expect the clerk_core.py file to be in the same directory or in site-packages/ folder of your python distribution

## Configuration Files
- **`clerk_core`**: ~/.config/clerk/clerk-core.conf
- **`clerk-rofi`**: ~/.config/clerk/clerk/clerk-rofi.conf
- **`clerk-api-rofi`**: ~/.config/clerk/clerk-api-rofi.conf

A default configuration will be generated if it doesn't exist.

## Usage

### clerk_core

is used as a module by the other scripts

#### Environment variables:
- MPD_HOST

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

## Rating Limitations

Originally I wanted to use MPD stickers for album ratings. While mpd supports an album type for stickers, it does not allow to combine these with a date type, which results in album ratings being applied for all albums with the same name.
Since caches already use msgpack, I opted for the same format for ratings, to keep the code clean. This has one obvious drawback:
Using clerk on multiple machines to rate albums will give you multiple, out of sync rating caches.

This is why the clerk-api-rofi script exists. If you use the web service, you can use this to interact with it. 


# Clerk Web API

This project provides a REST API to interact with MPD (Music Player Daemon) via a Flask backend. It supports album and track rating, playlist manipulation, and random playback features.

---

## Base URL

```
http://<host>:<port>/api/v1
```

Default: `http://localhost:5000/api/v1`

---

## Endpoints

### Albums

#### List All Albums

```
GET /albums
```

Returns cached list of albums with ratings.

#### List Latest Albums

```
GET /latest_albums
```

Returns most recently added albums with ratings.

#### Get Album Rating

```
GET /albums/<album_id>/rating
```

Returns rating of a specific album.

#### Set Album Rating

```
POST /albums/<album_id>/rating
```

**Request Body:**

```json
{ "rating": "1" }
```

Valid values: "1" to "10", "Delete", or "---".

### Tracks

#### List All Tracks

```
GET /tracks
```

Returns all track entries.

#### Set Track Rating

```
POST /tracks/<track_id>/rating
```

**Request Body:**

```json
{ "rating": "5" }
```

### Playlist Control

#### Add Album to Playlist

```
POST /playlist/add/album/<album_id>
```

**Request Body (optional):**

```json
{ "mode": "add" }
```

Modes: `add`, `insert`, `replace`

#### Add Track to Playlist

```
POST /playlist/add/track/<track_id>
```

**Request Body (optional):**

```json
{ "mode": "add" }
```

### Random Playback

#### Play Random Album

```
POST /playback/random/album
```

Plays a random album.

#### Play Random Tracks

```
POST /playback/random/tracks
```

Plays a random selection of tracks.

### Cache

#### Update Cache

```
POST /cache/update
```

Triggers cache rebuild from MPD.

### Current Playback Info

#### Get Current Album Rating

```
GET /current_album/rating
```

Returns rating for the album currently playing.

#### Set Current Album Rating

```
POST /current_album/rating
```

**Headers:**

```
Content-Type: application/json
```

**Request Body:**

```json
{ "rating": "7" }
```

#### Get Current Track Rating

```
GET /current_track/rating
```

Returns MPD sticker rating for currently playing track.

#### Set Current Track Rating

```
POST /current_track/rating
```

**Request Body:**

```json
{ "rating": "8" }
```

---

## Error Codes

* `400` - Invalid input
* `404` - Not found (album, track, etc.)
* `415` - Unsupported media type (missing/invalid JSON)
* `422` - Unprocessable entity (missing required fields)
* `500` - Internal server error

---

## Notes

* MPD must be running and reachable.
* Ratings for albums are stored via internal caching.
* Track ratings are managed using MPD stickers.
* JSON request bodies must include `Content-Type: application/json`.
