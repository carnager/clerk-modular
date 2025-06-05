#!/usr/bin/env python3

import os
import sys
import msgpack
import random
import subprocess
from mpd import MPDClient, MPDError
import toml

# --- Global Cache Data ---
album_ratings = {}

# --- XDG Paths ---
xdg_data = os.environ.get('XDG_DATA_HOME', os.path.join(os.environ.get('HOME', ''), ".local", "share"))
xdg_config = os.environ.get('XDG_CONFIG_HOME', os.path.join(os.environ.get('HOME', ''), ".config"))
clerk_data_dir = os.path.join(xdg_data, 'clerk')
clerk_config_path = os.path.join(xdg_config, 'clerk', 'clerk-core.conf')

# --- Cache File Paths ---
RATINGS_CACHE_FILE = os.path.join(clerk_data_dir, "ratings.cache")
ALBUM_CACHE_FILE = os.path.join(clerk_data_dir, "album.cache")
TRACKS_CACHE_FILE = os.path.join(clerk_data_dir, "tracks.cache")
LATEST_CACHE_FILE = os.path.join(clerk_data_dir, "latest.cache")

# Ensure directories exist
os.makedirs(clerk_data_dir, exist_ok=True)
os.makedirs(os.path.join(xdg_config, "clerk"), exist_ok=True)

# --- Core Config Loader ---
def create_core_config():
    default_config = """\
[general]
mpd_host = "localhost"
number_of_tracks = 20
cache_batch_size = 10000
random_artist = "albumartist"
sync_online_list = true
sync_command = ["/path/to/your/modified/musiclist.py"]
"""
    with open(clerk_config_path, 'w') as f:
        f.write(default_config)
    print(f"Core Info: Default core config created at {clerk_config_path}.")

def load_core_config():
    if not os.path.exists(clerk_config_path):
        print(f"Core Info: Config file not found. Creating default at {clerk_config_path}...")
        create_core_config()
    try:
        config = toml.load(clerk_config_path)
        return config
    except Exception as e:
        print(f"Core Error: Failed to load config: {e}. Exiting.", file=sys.stderr)
        sys.exit(1)

core_config = load_core_config()

# --- Helper Functions ---
def add_album_to_playlist(mpd_client, album_data_list, mode="add"):
    """
    Add/insert/replace album(s) in MPD playlist.
    album_data_list: list of album dictionaries.
    mode: "add", "insert", or "replace".
    """
    if mode == "replace":
        mpd_client.clear()
    pos = None
    if mode == "insert":
        try:
            pos = int(mpd_client.currentsong().get('pos')) + 1
        except:
            pos = None
    for album in album_data_list:
        tracks = mpd_client.find('albumartist', album['albumartist'], 'album', album['album'], 'date', album['date'])
        for track in tracks:
            try:
                if pos is not None:
                    mpd_client.addid(track['file'], pos)
                    pos += 1
                else:
                    mpd_client.addid(track['file'])
            except Exception as e:
                print(f"Core Error: Could not add track {track.get('file')}: {e}")
    if mode in ["replace", "insert"]:
        mpd_client.play()

def add_track_to_playlist(mpd_client, track_data_list, mode="add"):
    """
    Add/insert/replace individual tracks in MPD playlist.
    track_data_list: list of track dictionaries.
    mode: "add", "insert", or "replace".
    """
    if mode == "replace":
        mpd_client.clear()
    pos = None
    if mode == "insert":
        try:
            pos = int(mpd_client.currentsong().get('pos')) + 1
        except (TypeError, ValueError): # Handle cases where currentsong() or 'pos' is missing/invalid
            pos = None
    for track in track_data_list:
        # The key change is here: use track['file'] to add the specific track
        track_file = track.get('file')
        if not track_file:
            print(f"Core Warning: Skipping track due to missing 'file' key: {track}")
            continue
        try:
            if pos is not None:
                mpd_client.addid(track_file, pos)
                pos += 1
            else:
                mpd_client.addid(track_file)
        except Exception as e:
            print(f"Core Error: Could not add track {track_file}: {e}")
    if mode in ["replace", "insert"]:
        mpd_client.play()

def get_album_key(album_data):
    try:
        artist = album_data.get('albumartist', album_data.get('artist'))
        album = album_data.get('album')
        date = album_data.get('date')
        artist_str = str(artist[0]) if isinstance(artist, list) else str(artist)
        album_str = str(album[0]) if isinstance(album, list) else str(album)
        date_str = str(date[0]) if isinstance(date, list) else str(date)
        if not artist_str or not album_str or not date_str:
            return None
        return f"{artist_str}|||{album_str}|||{date_str}"
    except Exception:
        return None

def load_ratings_cache():
    global album_ratings
    try:
        if os.path.exists(RATINGS_CACHE_FILE):
            with open(RATINGS_CACHE_FILE, "rb") as f:
                data = f.read()
                album_ratings = msgpack.unpackb(data) if data else {}
        else:
            album_ratings = {}
    except Exception as e:
        print(f"Core Error: Failed to load ratings cache: {e}")
        album_ratings = {}

def save_ratings_cache():
    try:
        with open(RATINGS_CACHE_FILE, "wb") as f:
            f.write(msgpack.packb(album_ratings))
            f.flush()
            os.fsync(f.fileno())
    except Exception as e:
        print(f"Core Error: Failed to save ratings cache: {e}")
        raise

def update_album_rating(album_data, rating_value):
    """
    Update a single album’s rating (local cache & MPD sticker).
    If something changed and sync_online_list is true, run sync_command.
    """
    key = get_album_key(album_data)
    if not key:
        print(f"Core Error: Cannot generate album key from {album_data}")
        return False

    changed = False
    # LOCAL CACHE UPDATE
    if rating_value == "Delete":
        if key in album_ratings:
            del album_ratings[key]
            changed = True
    elif rating_value == "---":
        return False
    elif rating_value in [str(i) for i in range(1, 11)]:
        if album_ratings.get(key) != rating_value:
            album_ratings[key] = rating_value
            changed = True
    else:
        print(f"Core Warning: Invalid rating value '{rating_value}'")
        return False

    # SAVE IF CHANGED
    if changed:
        try:
            save_ratings_cache()
        except Exception as e:
            print(f"Core Error: Failed to save ratings cache: {e}")
            raise

        # SYNC ONLINE LIST IF ENABLED
        if core_config['general'].get('sync_online_list', False):
            cmd = core_config['general'].get('sync_command', [])
            try:
                subprocess.run(cmd, check=True)
            except Exception as e:
                print(f"Core Error: Sync command failed: {e}", file=sys.stderr)

    return changed

def update_track_rating(mpd_client, track_data, rating_value):
    """
    Update a single track's rating via MPD sticker.
    track_data: dictionary containing track information, *must* include 'file' key.
    rating_value: string "1" to "10", "---", or "Delete".
    """
    track_file = track_data.get('file')
    if not track_file:
        print(f"Core Error: Cannot rate track, missing 'file' key in track_data: {track_data}", file=sys.stderr)
        return False

    try:
        if rating_value == "Delete":
            mpd_client.sticker_delete('song', track_file, 'rating')
            print(f"Core Info: Deleted sticker for track '{track_data.get('title', 'N/A')}' (file: {track_file})")
            return True
        elif rating_value == "---":
            # No action needed if explicitly set to "---", which means unset/no change
            print(f"Core Info: Rating for track '{track_data.get('title', 'N/A')}' (file: {track_file}) set to '---' (no change/unset).")
            return False # Indicate no change to sticker
        elif rating_value in [str(i) for i in range(1, 11)]:
            mpd_client.sticker_set('song', track_file, 'rating', rating_value)
            print(f"Core Info: Set sticker for track '{track_data.get('title', 'N/A')}' (file: {track_file}) to {rating_value}")
            return True
        else:
            print(f"Core Warning: Invalid rating value '{rating_value}' for track rating.", file=sys.stderr)
            return False
    except MPDError as e:
        print(f"Core Error: MPD error updating sticker for track '{track_data.get('title', 'N/A')}' (file: {track_file}): {e}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"Core Error: Unexpected error updating sticker for track '{track_data.get('title', 'N/A')}' (file: {track_file}): {e}", file=sys.stderr)
        return False


def input_rating(prompt_text, menu_tool, menu_prompt):
    options = ['1','2','3','4','5','6','7','8','9','10','---','Delete']
    full_prompt = f"{prompt_text} {menu_prompt}"
    custom_menu = [w.replace(menu_prompt, full_prompt) for w in menu_tool]
    try:
        menu = subprocess.Popen(custom_menu, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        input_data = "\n".join(options).encode('utf-8')
        stdout, _ = menu.communicate(input=input_data)
        if menu.returncode != 0:
            return "---"
        result = stdout.decode('utf-8', errors='replace').strip()
        return result if result else "---"
    except Exception as e:
        print(f"Core Error: input_rating failed: {e}")
        return "---"

def read_album_cache(mode):
    path = ALBUM_CACHE_FILE if mode == "album" else LATEST_CACHE_FILE if mode == "latest" else None
    if not path:
        print(f"Core Error: Invalid mode '{mode}' for read_album_cache.", file=sys.stderr)
        return []
    try:
        if os.path.exists(path):
            with open(path, "rb") as f:
                data = f.read()
                return msgpack.unpackb(data) if data else []
        else:
            return []
    except Exception as e:
        print(f"Core Error: Failed to read album cache: {e}")
        return []

def read_tracks_cache():
    try:
        if os.path.exists(TRACKS_CACHE_FILE):
            with open(TRACKS_CACHE_FILE, "rb") as f:
                data = f.read()
                return msgpack.unpackb(data) if data else []
        else:
            return []
    except Exception as e:
        print(f"Core Error: Failed to read tracks cache: {e}")
        return []

def get_albums(mode="album", rating_filter=None):
    """Returns list of albums, optionally filtered by rating (1-10)."""
    load_ratings_cache()
    albums = read_album_cache(mode)
    if rating_filter:
        return [a for a in albums if album_ratings.get(get_album_key(a)) == str(rating_filter)]
    return albums

def get_tracks(rating_filter=None):
    """Returns list of tracks, optionally filtered by album rating (1-10)."""
    load_ratings_cache()
    tracks = read_tracks_cache()
    if rating_filter:
        filtered = []
        for t in tracks:
            key = get_album_key({
                'albumartist': t.get('artist'),
                'album': t.get('album'),
                'date': t.get('date', '0000')
            })
            if album_ratings.get(key) == str(rating_filter):
                filtered.append(t)
        return filtered
    return tracks

def create_cache(mpd_client):
    """
    Creates the album, tracks, and latest caches by fetching data from MPD
    in batches using the 'search' command with the 'window' parameter.
    """
    print("Core Info: Creating cache using batched search with window parameter...")
    
    all_songs_collected = [] # NEW: To collect all songs before sorting/processing

    batch_size = core_config['general'].get('cache_batch_size', 10000)

    try:
        stats = mpd_client.stats()
        total_songs = int(stats.get('songs', 0))
        
        if total_songs == 0:
            print("Core Warning: No songs found in MPD database. Cache will be empty.")
            # Save empty caches
            with open(ALBUM_CACHE_FILE, "wb") as f: f.write(msgpack.packb([]))
            with open(TRACKS_CACHE_FILE, "wb") as f: f.write(msgpack.packb([]))
            with open(LATEST_CACHE_FILE, "wb") as f: f.write(msgpack.packb([]))
            print("Core Info: Cache created (empty due to no songs).")
            return True

        offset = 0
        while offset < total_songs:
            try:
                start_index = offset + 1 # MPD window is 1-indexed for start
                end_index = offset + batch_size # MPD window end is exclusive, so it's start + count

                window_str = f"{start_index}:{end_index}"

                batch_songs = mpd_client.search('filename', '', 'window', window_str)
                
                if not batch_songs:
                    break # No more songs in this batch, or end of database

                all_songs_collected.extend(batch_songs) # Collect all songs
                
                offset += len(batch_songs)
                print(f"Core Info: Fetched {offset}/{total_songs} songs...")

            except MPDError as mpd_e:
                print(f"Core Error: MPD error during batch search (offset {offset}): {mpd_e}", file=sys.stderr)
                break
            except Exception as e:
                print(f"Core Error: Unexpected error processing batch (offset {offset}): {e}", file=sys.stderr)
                break

        # --- Process all collected songs after fetching is complete ---
        albums = []
        tracks = []
        latest_unique_albums = []
        seen_albums_for_main_list = set()
        seen_albums_for_latest_list = set()
        track_id_counter = 0

        # Sort all songs by last-modified time in descending order to get the "latest" first
        # Handle cases where 'last-modified' might be missing (e.g., set to a default old date)
        all_songs_collected.sort(key=lambda s: s.get('last-modified', '1970-01-01T00:00:00Z'), reverse=True)


        for song in all_songs_collected:
            album_artist_for_key = song.get('albumartist')
            if not album_artist_for_key:
                album_artist_for_key = song.get('artist')

            album_name = song.get('album')
            album_date = song.get('date', '0000')
            track_file = song.get('file')

            # Skip if essential metadata or file path is missing
            if not album_artist_for_key or not album_name or not album_date or not track_file:
                continue

            album_key_tuple = (album_artist_for_key, album_name, album_date)
            
            # --- Populate the 'albums' list (all unique albums) ---
            if album_key_tuple not in seen_albums_for_main_list:
                seen_albums_for_main_list.add(album_key_tuple)
                albums.append({
                    'albumartist': album_artist_for_key,
                    'album': album_name,
                    'date': album_date,
                    'id': str(len(albums)) # Unique ID for main albums list
                })
            
            # --- Populate the 'tracks' list (all individual songs) ---
            tracks.append({
                'track': song.get('track', ''),
                'title': song.get('title', ''),
                'artist': song.get('artist'), # Keep original artist tag for tracks
                'album': album_name,
                'date': album_date,
                'file': track_file,
                'id': str(track_id_counter) # Unique ID for each track
            })
            track_id_counter += 1

            # --- Populate the 'latest_unique_albums' list ---
            # Ensure uniqueness for 'latest' list while preserving order of first encounter (from sorted list)
            if album_key_tuple not in seen_albums_for_latest_list:
                seen_albums_for_latest_list.add(album_key_tuple)
                latest_unique_albums.append({
                    'albumartist': album_artist_for_key,
                    'album': album_name,
                    'date': album_date,
                    'id': str(len(latest_unique_albums)) # Unique ID for items in this list
                })
        
        # 'latest_unique_albums' is now already sorted by last-modified (desc) due to previous sort of all_songs_collected

        # Write data to cache files
        with open(ALBUM_CACHE_FILE, "wb") as f:
            f.write(msgpack.packb(albums))
        with open(TRACKS_CACHE_FILE, "wb") as f:
            f.write(msgpack.packb(tracks))
        with open(LATEST_CACHE_FILE, "wb") as f:
            f.write(msgpack.packb(latest_unique_albums))
        print(f"Core Info: Cache created successfully. Total tracks processed: {len(tracks)}.")
        print(f"Core Info: Total unique albums in main cache: {len(albums)}.")
        print(f"Core Info: Total unique albums in latest cache: {len(latest_unique_albums)}.")
        return True
    except MPDError as mpd_e:
        print(f"Core Error: MPD error getting status or listing artists: {mpd_e}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"Core Error: Cache creation failed: {e}", file=sys.stderr)
        return False

def check_update(mpd_client):
    files = [ALBUM_CACHE_FILE, TRACKS_CACHE_FILE, LATEST_CACHE_FILE, RATINGS_CACHE_FILE]
    if not all(os.path.exists(f) for f in files):
        print("Core Info: Cache missing. Creating...")
        if not create_cache(mpd_client):
            print("Core Error: Cache creation failed.")
            return False
    if os.path.exists(RATINGS_CACHE_FILE) and not album_ratings:
        load_ratings_cache()
    return True

# --- Random Logic ---
def random_album(mpd_client):
    try:
        tag = core_config['general']['random_artist']
        values = mpd_client.list(tag)
        values = [v if isinstance(v, str) else v.get(tag) for v in values]
        values = [v for v in values if v]
        if not values:
            print(f"Core Warning: No values found for tag '{tag}'")
            return
        random_value = random.choice(values)
        all_tracks = mpd_client.find(tag, random_value)
        albums = set((t.get('album'), t.get('date', '0000')) for t in all_tracks if t.get('album'))
        if not albums:
            print(f"Core Warning: No albums found for '{random_value}'")
            return
        album, date = random.choice(list(albums))
        tracks = mpd_client.find(tag, random_value, 'album', album, 'date', date)
        if not tracks:
            print(f"Core Warning: No tracks found for album '{album}'")
            return
        mpd_client.clear()
        mpd_client.findadd(tag, random_value, 'album', album, 'date', date)
        mpd_client.play()
        print(f"Playing: {random_value} - {album} ({date})")
    except Exception as e:
        print(f"Core Error: Random album error: {e}")

def random_tracks(mpd_client):
    try:
        tag = core_config['general']['random_artist']
        count = int(core_config['general']['number_of_tracks'])
        values = mpd_client.list(tag)
        values = [v if isinstance(v, str) else v.get(tag) for v in values]
        values = [v for v in values if v]
        if not values:
            print(f"Core Warning: No values for tag '{tag}'")
            return
        chosen = random.sample(values, min(count, len(values)))
        mpd_client.clear()
        for v in chosen:
            tracks = mpd_client.find(tag, v)
            if tracks:
                track = random.choice(tracks)
                mpd_client.addid(track['file'])
        mpd_client.play()
        print(f"Playing {len(chosen)} random tracks")
    except Exception as e:
        print(f"Core Error: Random tracks error: {e}")

# --- Self-Test ---
if __name__ == "__main__":
    print("--- clerk_core.py Self-Test ---")
    mpd = MPDClient()
    host = os.environ.get('MPD_HOST', core_config['general']['mpd_host'])
    try:
        mpd.connect(host, 6600)
        check_update(mpd)
        random_album(mpd)
        random_tracks(mpd)
        mpd.close()
        mpd.disconnect()
    except Exception as e:
        print(f"Core Test Error: {e}")

