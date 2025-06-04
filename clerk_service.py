#!/usr/bin/env python3
import os
import sys
import logging
from flask import Flask, request, jsonify, send_from_directory
from mpd import MPDClient, MPDError, ConnectionError as MPDConnErr
import clerk_core as core
from toml import load as toml_load

# ── Configuration ──────────────────────────────────────────────────────────────
PUBLIC_DIR = "/usr/share/clerk-web"
conf_path = os.path.join(
    os.environ.get('XDG_CONFIG_HOME', os.path.join(os.environ.get('HOME'),'.config')),
    'clerk', 'clerk-core.conf'
)
if not os.path.exists(conf_path):
    core.create_core_config()
core_config = toml_load(conf_path)
mpd_host   = os.environ.get('MPD_HOST', core_config['general']['mpd_host'])

# ── Logging Setup ──────────────────────────────────────────────────────────────
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("clerk-web")

# ── MPD Client Setup ───────────────────────────────────────────────────────────
mpd = MPDClient()

def connect_mpd():
    """Ensure mpd client stays connected, reconnecting if ping fails."""
    try:
        mpd.ping()
    except (MPDError, MPDConnErr):
        # Safe disconnect
        try: mpd.close()
        except: pass
        try: mpd.disconnect()
        except: pass
        # Reconnect
        mpd.connect(mpd_host, 6600)
        mpd.ping()

# Initial connect
try:
    mpd.connect(mpd_host, 6600)
    mpd.ping()
except Exception as e:
    logger.critical(f"Cannot connect to MPD at {mpd_host}:6600: {e}")
    sys.exit(1)

# ── Ensure caches exist and load ratings ────────────────────────────────────────
if not core.check_update(mpd):
    logger.critical("Core cache initialization failed")
    sys.exit(1)
core.load_ratings_cache()

# ── Flask App Initialization ──────────────────────────────────────────────────
app = Flask(
    __name__,
    static_folder=PUBLIC_DIR,
    static_url_path=''
)

# ── Static File Routes ─────────────────────────────────────────────────────────
@app.route('/')
def index():
    return send_from_directory(PUBLIC_DIR, 'index.html')

@app.route('/<path:filename>')
def public_asset(filename):
    return send_from_directory(PUBLIC_DIR, filename)

# ── API Endpoints ──────────────────────────────────────────────────────────────

@app.route('/api/v1/albums', methods=['GET'])
def list_albums():
    albums = core.read_album_cache("album")
    core.load_ratings_cache()
    out = []
    for a in albums:
        entry = a.copy()
        entry['rating'] = core.album_ratings.get(core.get_album_key(a))
        out.append(entry)
    return jsonify(out), 200

@app.route('/api/v1/tracks', methods=['GET'])
def list_tracks():
    tracks = core.read_tracks_cache()
    return jsonify(tracks), 200

@app.route('/api/v1/albums/<album_id>/rating', methods=['GET'])
def get_album_rating(album_id):
    albums = core.read_album_cache("album")
    a = next((x for x in albums if x['id']==album_id), None)
    if not a:
        return jsonify({"error":"Album not found"}), 404
    core.load_ratings_cache()
    return jsonify({
        "album_id": album_id,
        "rating": core.album_ratings.get(core.get_album_key(a))
    }), 200

@app.route('/api/v1/albums/<album_id>/rating', methods=['POST'])
def set_album_rating(album_id):
    data = request.get_json() or {}
    val  = str(data.get('rating','---'))
    if val not in [str(i) for i in range(1,11)] + ['Delete','---']:
        return jsonify({"error":"Invalid rating"}), 400
    albums = core.read_album_cache("album")
    a = next((x for x in albums if x['id']==album_id), None)
    if not a:
        return jsonify({"error":"Album not found"}), 404
    changed = core.update_album_rating(a, val)
    return jsonify({"changed": changed}), 200

@app.route('/api/v1/playlist/add/album/<album_id>', methods=['POST'])
def add_album_to_playlist(album_id):
    data = request.get_json() or {}
    mode = data.get('mode','add')
    if mode not in ['add','insert','replace']:
        return jsonify({"error":"Invalid mode"}), 400
    albums = core.read_album_cache("album")
    a = next((x for x in albums if x['id']==album_id), None)
    if not a:
        return jsonify({"error":"Album not found"}), 404
    # Ensure MPD is connected
    connect_mpd()
    
    try:
        core.add_album_to_playlist(mpd, [a], mode) # Call the core function
        # If no exception, it was successful. Return a clear success message.
        return jsonify({"message": "Album added to playlist successfully."}), 200
    except Exception as e:
        # Catch any potential errors during MPD interaction or in the core function
        return jsonify({"error": f"Failed to add album to playlist: {str(e)}"}), 500


@app.route('/api/v1/playlist/add/track/<track_id>', methods=['POST'])
def add_track_to_playlist(track_id):
    tracks = core.read_tracks_cache()
    t = next((x for x in tracks if x['id']==track_id), None)
    if not t:
        return jsonify({"error":"Track not found"}), 404
    connect_mpd()
    core.add_track_to_playlist(mpd, [t], mode='add')
    return jsonify({"message":"Track added"}), 200

@app.route('/api/v1/playback/random/album', methods=['POST'])
def play_random_album():
    connect_mpd()
    result = core.random_album(mpd)
    if isinstance(result, tuple):
        body, status = result
        return jsonify(body), status
    return jsonify(result), 200

@app.route('/api/v1/playback/random/tracks', methods=['POST'])
def play_random_tracks():
    connect_mpd()
    result = core.random_tracks(mpd)
    if isinstance(result, tuple):
        body, status = result
        return jsonify(body), status
    return jsonify(result), 200

@app.route('/api/v1/cache/update', methods=['POST'])
def cache_update():
    connect_mpd()
    ok = core.create_cache(mpd)
    return (jsonify({"message":"Cache updated"}), 200) if ok \
           else (jsonify({"error":"Cache update failed"}), 500)

@app.route('/current_album/rating', methods=['GET'])
def get_current_album_rating():
    try:
        connect_mpd()
        song = mpd.currentsong()
        if not song:
            return jsonify({"error":"No song playing"}), 404
        ad = {
            'albumartist': song.get('albumartist', song.get('artist')),
            'album':       song.get('album'),
            'date':        song.get('date','0000')
        }
        core.load_ratings_cache()
        return jsonify({"rating": core.album_ratings.get(core.get_album_key(ad))}), 200
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/current_album/rating', methods=['POST'])
def set_current_album_rating():
    data = request.get_json() or {}
    val  = str(data.get('rating','---'))
    if val not in [str(i) for i in range(1,11)] + ['Delete','---']:
        return jsonify({"error":"Invalid rating"}), 400
    try:
        connect_mpd()
        song = mpd.currentsong()
        if not song:
            return jsonify({"error":"No song playing"}), 404
        ad = {
            'albumartist': song.get('albumartist', song.get('artist')),
            'album':       song.get('album'),
            'date':        song.get('date','0000')
        }
        changed = core.update_album_rating(ad, val)
        return jsonify({"changed": changed}), 200
    except Exception as e:
        return jsonify({"error": str(e)}), 500

# ── Run Application ────────────────────────────────────────────────────────────
if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)

