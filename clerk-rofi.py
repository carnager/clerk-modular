#!/usr/bin/env python3

import os
import sys
import subprocess
import toml
from mpd import MPDClient
import clerk_core as core

# ── Load Rofi UI Config ─────────────────────────────────────────────────────────
xdg_config  = os.environ.get('XDG_CONFIG_HOME',
                            os.path.join(os.environ.get('HOME',''),'.config'))
conf_path   = os.path.join(xdg_config, 'clerk', 'clerk-rofi.conf')
cfg         = toml.load(conf_path)

menu_prompt = cfg['general']['menu_prompt']
menu_tool   = [w.replace('PLACEHOLDER', menu_prompt)
               for w in cfg['general']['menu_tool']]

# Column widths
artist_w      = int(cfg['columns']['artist_width'])
albumartist_w = int(cfg['columns']['albumartist_width'])
date_w        = int(cfg['columns']['date_width'])
album_w       = int(cfg['columns']['album_width'])
id_w          = int(cfg['columns']['id_width'])
title_w       = int(cfg['columns']['title_width'])
track_w       = int(cfg['columns']['track_width'])

# ── MPD Connection ──────────────────────────────────────────────────────────────
m = MPDClient()
mpd_host = os.environ.get('MPD_HOST',
                          core.core_config['general']['mpd_host'])
try:
    m.connect(mpd_host, 6600)
    m.ping()
except Exception as e:
    print(f"Fatal: cannot connect to MPD at {mpd_host}:6600 – {e}",
          file=sys.stderr)
    sys.exit(1)

# ── Rofi Menu Helper ────────────────────────────────────────────────────────────
def _menu(lines, trim='no'):
    """
    Show lines in Rofi. If trim=='yes', return list of IDs (second-to-last token).
    Otherwise, return the single selected line.
    """
    p = subprocess.Popen(menu_tool,
                         stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE,
                         stderr=subprocess.DEVNULL)
    inp = "\n".join(lines).encode('utf-8')
    out,_ = p.communicate(inp)
    if p.returncode != 0:
        return [] if trim=='yes' else ''
    sel = out.decode('utf-8', errors='replace').splitlines()
    if trim=='yes':
        ids = []
        for line in sel:
            parts = line.split()
            if len(parts) >= 2:
                # second-to-last part is the ID (last is 'r=<rating>')
                ids.append(parts[-2])
        return ids
    return sel[0].strip() if sel else ''

# ── Formatting Helpers ─────────────────────────────────────────────────────────
def format_album_line(a):
    key    = core.get_album_key(a)
    rating = core.album_ratings.get(key, '-')
    base   = (f"{a['albumartist']:<{albumartist_w}} "
              f"{a['date']:<{date_w}} "
              f"{a['album']:<{album_w}} "
              f"{a['id']:<{id_w}}")
    return f"{base} r={rating}"

def format_track_line(t):
    track = t.get('track', '')
    if isinstance(track, list):
        track = track[0] if track else ''
    key = core.get_album_key({
        'albumartist': t['artist'],
        'album':       t['album'],
        'date':        t.get('date','0000'),
    })
    rating = core.album_ratings.get(key, '-')
    base   = (f"{track:<{track_w}} "
              f"{t['title']:<{title_w}} "
              f"{t['artist']:<{artist_w}} "
              f"{t['album']:<{album_w}} "
              f"{t.get('date','0000'):<{date_w}} "
              f"{t['id']:<{id_w}}")
    return f"{base} r={rating}"

# ── UI Actions ─────────────────────────────────────────────────────────────────
def add_album(mode):
    core.load_ratings_cache()
    albums = core.get_albums(mode)
    if not albums:
        print("No albums available.", file=sys.stderr)
        return

    lines = [format_album_line(a) for a in albums]
    selected_ids = _menu(lines, trim='yes')
    if not selected_ids:
        return

    action = _menu(['Add','Insert','Replace','Rate'], trim='no')
    if action not in ('Add','Insert','Replace','Rate'):
        return

    if action == 'Rate':
        for a in albums:
            if a['id'] in selected_ids:
                prompt = f"{a['albumartist']} - {a['album']}"
                new_rating = core.input_rating(prompt,
                                               menu_tool,
                                               menu_prompt)
                core.update_album_rating(a, new_rating)
        return  # done rating

    # Add/Insert/Replace
    picked = [a for a in albums if a['id'] in selected_ids]
    core.add_album_to_playlist(m,
                               picked,
                               mode=action.lower())

def add_track():
    core.load_ratings_cache()
    tracks = core.get_tracks()
    if not tracks:
        print("No tracks available.", file=sys.stderr)
        return

    lines = [format_track_line(t) for t in tracks]
    selected_ids = _menu(lines, trim='yes')
    if not selected_ids:
        return

    action = _menu(['Add','Insert','Replace','Rate Track (MPD Sticker)'],
                   trim='no')
    if action not in ('Add','Insert','Replace','Rate Track (MPD Sticker)'):
        return

    if action == 'Rate Track (MPD Sticker)':
        for t in tracks:
            if t['id'] in selected_ids:
                prompt = f"{t['artist']} - {t['title']}"
                val = core.input_rating(prompt,
                                        menu_tool,
                                        menu_prompt)
                if val == 'Delete':
                    m.sticker_delete('song', t['file'], 'rating')
                else:
                    m.sticker_set('song', t['file'], 'rating', val)
        return

    picked = [t for t in tracks if t['id'] in selected_ids]
    core.add_track_to_playlist(m,
                               picked,
                               mode=action.lower())

def random_album_ui():
    core.random_album(m)

def random_tracks_ui():
    core.random_tracks(m)

def current_track():
    song = m.currentsong()
    if not song:
        return
    action = _menu(['Rate Album (Local)','Rate Track (MPD Sticker)'],
                   trim='no')
    if action.startswith('Rate Album'):
        core.load_ratings_cache()
        ad = {
            'albumartist': song.get('albumartist', song.get('artist')),
            'album':       song.get('album'),
            'date':        song.get('date','0000')
        }
        rating = core.input_rating(f"{ad['albumartist']} - {ad['album']}",
                                   menu_tool,
                                   menu_prompt)
        core.update_album_rating(ad, rating)
    elif action.startswith('Rate Track'):
        val  = core.input_rating(f"{song.get('artist')} - {song.get('title')}",
                                 menu_tool,
                                 menu_prompt)
        path = song.get('file')
        if val == 'Delete':
            m.sticker_delete('song', path, 'rating')
        else:
            m.sticker_set('song', path, 'rating', val)

# ── Help Text and Main ─────────────────────────────────────────────────────────
ra = core.core_config['general']['random_artist']
nt = core.core_config['general']['number_of_tracks']
help_txt = f"""
Usage: clerk [option]
 -a  Add Albums
 -l  Add Latest Albums
 -t  Add Tracks
 -A  Random Album
 -T  Random Tracks (by '{ra}', count {nt})
 -c  Rate Current Track/Album
 -u  Rebuild Caches
 -x  Regenerate UI Config
 -h  Show This Help
"""

if __name__ == "__main__":
    if len(sys.argv) > 1:
        cmd = sys.argv[1]
        match cmd:
            case '-a': add_album('album')
            case '-l': add_album('latest')
            case '-t': add_track()
            case '-A': random_album_ui()
            case '-T': random_tracks_ui()
            case '-c': current_track()
            case '-u': core.create_cache(m)
            case '-x':
                print("Regenerate UI config manually.")
            case '-h'|_: print(help_txt)
    else:
        print(help_txt)

    m.close()
    m.disconnect()
    sys.exit(0)

