# Clerk

Clerk is a little application to quickly add albums or tracks to your mpd playback queue.
It consists of:
- **`clerk_core`**: a core module to handle all functionality.
- **`clerk-rofi`**: an example UI utilizing rofi
- **`clerk-service`**: a service that serves a REST API plus an example webpage.
- **`clerk-musiclist`**: a little script that creates a searchable webpage, which uses clerk's cache files to generate album lists with album ratings.

## Installation

### Arch Linux:
- use the provided PKGBUILD

### Others:
Note for packagers: clerk-rofi and clerk-service expect the clerk_core.py file to be in the same directory.

## Configuration
The core module and clerk-rofi tool expect configuration files at ~/.config/clerk/clerk-core.conf and ~/.config/clerk/clerk/clerk-rofi.conf.
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


# REST API of clerk-service

---

## **Static File Serving**

These endpoints serve the web interface for Clerk.

* **GET `/`**

    * **Description:** Serves the main `index.html` file for the web interface.

    * **Response:** `text/html` content of `index.html`.

    * **Status Codes:** `200 OK`

* **GET `/<path:filename>`**

    * **Description:** Serves any other static assets (CSS, JavaScript, images) located in the configured `PUBLIC_DIR`.

    * **Response:** The requested file.

    * **Status Codes:** `200 OK`, `404 Not Found`

---

## **API Endpoints (v1)**

### **Albums**

* **GET `/api/v1/albums`**

    * **Description:** Retrieves a list of all albums in the Clerk cache, including their associated ratings.

    * **Response Body (JSON):**

        ```json
        [
            {
                "albumartist": "Artist Name",
                "album": "Album Title",
                "date": "YYYY",
                "id": "unique_id",
                "rating": "X"
            }
        ]
        ```

        The `rating` field will be a string from "1" to "10", or `null` if the album is unrated.

    * **Status Codes:** `200 OK`

* **GET `/api/v1/albums/<album_id>/rating`**

    * **Description:** Retrieves the rating for a specific album by its unique `album_id`.

    * **Parameters:**

        * `album_id` (path): The unique ID of the album as found in `/api/v1/albums`.

    * **Response Body (JSON):**

        ```json
        {
            "album_id": "unique_id",
            "rating": "X"
        }
        ```

        The `rating` field will be a string from "1" to "10", or `null` if the album is unrated.

    * **Status Codes:** `200 OK`, `404 Not Found` (if album not found)

* **POST `/api/v1/albums/<album_id>/rating`**

    * **Description:** Sets or updates the rating for a specific album by its unique `album_id`.

    * **Parameters:**

        * `album_id` (path): The unique ID of the album.

    * **Request Body (JSON):**

        ```json
        {
            "rating": "X"
        }
        ```

        The `rating` value can be a string from "1" through "10", "---" to unset the rating, or "Delete" to remove the rating entirely.

    * **Response Body (JSON):**

        ```json
        {
            "changed": true
        }
        ```

        changed` will be `true` if the rating was successfully updated or deleted, `false` otherwise.

    * **Status Codes:** `200 OK`, `400 Bad Request` (for invalid rating value), `404 Not Found` (if album not found)

### **Tracks**

* **GET `/api/v1/tracks`**

    * **Description:** Retrieves a list of all individual tracks in the Clerk cache. These objects include `file` paths essential for MPD operations.

    * **Response Body (JSON):**

        ```json
        [
            {
                "track": "1",
                "title": "Song Title",
                "artist": "Artist Name",
                "album": "Album Title",
                "date": "YYYY",
                "file": "/path/to/music/file.mp3",
                "id": "unique_id"
            }
        ]
        ```

        The `file` field contains the full path to the audio file and is important for MPD interactions.

    * **Status Codes:** `200 OK`

### **Playlist Management**

* **POST `/api/v1/playlist/add/album/<album_id>`**

    * **Description:** Adds a full album to the MPD playlist.

    * **Parameters:**

        * `album_id` (path): The unique ID of the album to add.

    * **Request Body (JSON, optional):**

        ```json
        {
            "mode": "add"
        }
        ```

        The `mode` field is optional. It can be `"add"` (default, appends to playlist), `"insert"` (inserts at the current playback position), or `"replace"` (clears the playlist and adds the album).

    * **Response Body (JSON):**

        ```json
        {
            "message": "Album added to playlist successfully."
        }
        ```

    * **Status Codes:** `200 OK`, `400 Bad Request` (for invalid mode), `404 Not Found` (if album not found), `500 Internal Server Error` (if MPD operation fails)

* **POST `/api/v1/playlist/add/track/<track_id>`**

    * **Description:** Adds a single track to the MPD playlist.

    * **Parameters:**

        * `track_id` (path): The unique ID of the track to add.

    * **Request Body (JSON, optional):**

        ```json
        {
            "mode": "add"
        }
        ```

        The `mode` field is optional. It can be `"add"` (default, appends to playlist), `"insert"` (inserts at the current playback position), or `"replace"` (clears the playlist and adds the track).

    * **Response Body (JSON):**

        ```json
        {
            "message": "Track added to playlist successfully in 'mode' mode."
        }
        ```

    * **Status Codes:** `200 OK`, `400 Bad Request` (for invalid mode), `404 Not Found` (if track not found), `500 Internal Server Error` (if MPD operation fails)

### **Playback Control**

* **POST `/api/v1/playback/random/album`**

    * **Description:** Clears the current playlist and adds a single random album, then starts playback.

    * **Response Body (JSON):**

        ```json
        {
            "message": "Playing: Artist - Album (YYYY)"
        }
        ```

        Note: The exact message format depends on `clerk_core` output, but it typically returns information about the played item.

    * **Status Codes:** `200 OK`, `500 Internal Server Error` (if MPD operation fails)

* **POST `/api/v1/playback/random/tracks`**

    * **Description:** Clears the current playlist and adds a configured number of random tracks, then starts playback.

    * **Response Body (JSON):**

        ```json
        {
            "message": "Playing X random tracks"
        }
        ```

    * **Status Codes:** `200 OK`, `500 Internal Server Error` (if MPD operation fails)

### **Cache Management**

* **POST `/api/v1/cache/update`**

    * **Description:** Forces a complete rebuild of the Clerk internal caches (albums and tracks). This can be useful after adding new music to your MPD library.

    * **Response Body (JSON):**

        ```json
        {
            "message": "Cache updated"
        }
        ```

        or

        ```json
        {
            "error": "Cache update failed"
        }
        ```

    * **Status Codes:** `200 OK` (on success), `500 Internal Server Error` (on failure)

### **Current Playback Information**

* **GET `/current_album/rating`**

    * **Description:** Retrieves the local rating for the album of the currently playing song.

    * **Response Body (JSON):**

        ```json
        {
            "rating": "X"
        }
        ```

        The `rating` field will be a string from "1" to "10", or `null` if unrated.

    * **Status Codes:** `200 OK`, `404 Not Found` (if no song is playing), `500 Internal Server Error`

* **POST `/current_album/rating`**

    * **Description:** Sets or updates the local rating for the album of the currently playing song.

    * **Request Body (JSON):**

        ```json
        {
            "rating": "X"
        }
        ```

        The `rating` value can be a string from "1" through "10", "---" to unset the rating, or "Delete" to remove the rating entirely.

    * **Response Body (JSON):**

        ```json
        {
            "changed": true
        }
        ```

        changed` will be `true` if the rating was updated or deleted, `false` otherwise.

    * **Status Codes:** `200 OK`, `400 Bad Request` (for invalid rating value or request body), `404 Not Found` (if no song is playing), `415 Unsupported Media Type` (if Content-Type is not `application/json`), `500 Internal Server Error` (if MPD or core operation fails)
