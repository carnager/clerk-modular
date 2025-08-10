#!/usr/bin/env python3
"""
clerk-gnome.py - Command line interface for clerk-gnome extension

Usage examples:
    clerk-gnome.py -a      # Show albums
    clerk-gnome.py -l      # Show latest albums  
    clerk-gnome.py -t      # Show tracks
    clerk-gnome.py -c      # Current track actions
    clerk-gnome.py -A      # Random album
    clerk-gnome.py -T      # Random tracks
    clerk-gnome.py -u      # Cache update
"""

import sys
import subprocess

def call_dbus_method(method):
    """Call a DBus method on the clerk-gnome extension"""
    try:
        result = subprocess.run([
            'gdbus', 'call',
            '--session',
            '--dest', 'org.gnome.Shell.Extensions.ClerkGnome',
            '--object-path', '/org/gnome/Shell/Extensions/ClerkGnome',
            '--method', f'org.gnome.Shell.Extensions.ClerkGnome.{method}'
        ], capture_output=True, text=True, timeout=30)
        
        if result.returncode != 0:
            print(f"Error calling {method}: {result.stderr}", file=sys.stderr)
            return False
        return True
        
    except subprocess.TimeoutExpired:
        print(f"Timeout calling {method}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"Failed to call {method}: {e}", file=sys.stderr)
        return False

def main():
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(1)
    
    cmd = sys.argv[1]
    
    method_map = {
        '-a': 'ShowAlbums',
        '-l': 'ShowLatestAlbums', 
        '-t': 'ShowTracks',
        '-c': 'ShowCurrentTrack',
        '-A': 'RandomAlbum',
        '-T': 'RandomTracks',
        '-u': 'CacheUpdate'
    }
    
    if cmd not in method_map:
        print(f"Unknown command: {cmd}")
        print(__doc__)
        sys.exit(1)
    
    success = call_dbus_method(method_map[cmd])
    sys.exit(0 if success else 1)

if __name__ == '__main__':
    main()