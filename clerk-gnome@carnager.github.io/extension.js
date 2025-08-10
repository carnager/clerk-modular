/*
 * clerk-gnome extension.js
 *
 * Single-file GNOME Shell extension for Clerk music library integration.
 * Features built-in hotkeys and configuration.
 *
 * Author: Rasmus Steinke
 */

import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import St from 'gi://St';
import Clutter from 'gi://Clutter';
import Soup from 'gi://Soup';
import Shell from 'gi://Shell';
import Meta from 'gi://Meta';

import {Extension, gettext as _} from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';

// --- CONFIGURATION ---
const CONFIG = {
    // Clerk API configuration - CHANGE THIS to your clerk API URL
    api_base_url: 'http://gemenon:6601/api/v1',
    
    // UI configuration
    menu_prompt: 'Clerk',
    columns: {
        artist_width: 30,
        albumartist_width: 30,
        date_width: 6,
        album_width: 40,
        id_width: 5,
        title_width: 40,
        track_width: 5
    }
};

class ClerkUI {
    constructor(extension) {
        this._extension = extension;
        this._httpSession = new Soup.Session();
        
        // Create UI identical to dmenu-gnome
        this.actor = new St.BoxLayout({
            style_class: 'dmenu-container',
            vertical: true,
            x_align: Clutter.ActorAlign.FILL,
            y_align: Clutter.ActorAlign.START,
            reactive: true,
        });

        this.top_bar = new St.BoxLayout({
            style_class: 'dmenu-top-bar',
            vertical: false,
        });

        this.prompt_label = new St.Label({
            style_class: 'dmenu-prompt',
            y_align: Clutter.ActorAlign.CENTER,
        });
        
        this.entry = new St.Entry({
            style_class: 'dmenu-entry',
            can_focus: true,
            x_expand: true,
        });

        this.results_container = new St.ScrollView({
            style_class: 'dmenu-results-container',
            hscrollbar_policy: St.PolicyType.NEVER,
            vscrollbar_policy: St.PolicyType.AUTOMATIC,
            x_expand: true,
            y_expand: true,
        });
        this.results_box = new St.BoxLayout({ style_class: 'dmenu-results-box', vertical: true });
        
        this.results_container.set_child(this.results_box);
        
        this.top_bar.add_child(this.prompt_label);
        this.top_bar.add_child(this.entry);
        this.actor.add_child(this.top_bar);
        this.actor.add_child(this.results_container);

        // Event handling
        this.entry.get_clutter_text().connect('text-changed', this._onTextChanged.bind(this));
        this.entry.get_clutter_text().connect('activate', this._onActivate.bind(this));
        this.actor.connect('key-press-event', this._onKeyPress.bind(this));

        // State
        this._items = [];
        this._visible_items = [];
        this._selected_index = 0;
        this._selected_items = new Set();
        this._filter_timeout = null;
        this._scroll_start_index = 0;
        this._items_per_page = 20;
        this._current_mode = null;
        this._current_data = null;
        this._inActionMode = false;
        this._inRatingMode = false;
        this._actionCallback = null;
        this._ratingCallback = null;
    }

    // HTTP request helper
    async _makeApiRequest(endpoint, method = 'GET', jsonData = null) {
        return new Promise((resolve, reject) => {
            const url = CONFIG.api_base_url + '/' + endpoint.replace(/^\//, '');
            const message = Soup.Message.new(method, url);
            
            if (method === 'POST' && jsonData) {
                const body = JSON.stringify(jsonData);
                message.set_request_body_from_bytes('application/json', new GLib.Bytes(body));
            }
            
            this._httpSession.send_and_read_async(message, GLib.PRIORITY_DEFAULT, null, (session, result) => {
                try {
                    const response = session.send_and_read_finish(result);
                    const data = new TextDecoder().decode(response.get_data());
                    const json = JSON.parse(data);
                    resolve(json);
                } catch (error) {
                    reject(error);
                }
            });
        });
    }

    async showAlbums(mode) {
        this._current_mode = mode;
        this._inActionMode = false;
        this._inRatingMode = false;
        const prompt = mode === 'latest_albums' ? 'Latest Albums' : 'Albums';
        
        this.prompt_label.set_text(prompt + ' ');
        this.entry.set_text('');
        this._selected_index = 0;
        this._selected_items.clear();
        
        this._showUI();
        
        try {
            const albums = await this._makeApiRequest(mode);
            this._current_data = albums;
            this._items = albums.map(album => this._formatAlbumLine(album));
            this._updateResults();
        } catch (error) {
            this._showError(`Failed to fetch ${mode}: ${error.message}`);
        }
    }

    async showTracks() {
        this._current_mode = 'tracks';
        this._inActionMode = false;
        this._inRatingMode = false;
        
        this.prompt_label.set_text('Tracks ');
        this.entry.set_text('');
        this._selected_index = 0;
        this._selected_items.clear();
        
        this._showUI();
        
        try {
            const tracks = await this._makeApiRequest('tracks');
            this._current_data = tracks;
            this._items = tracks.map(track => this._formatTrackLine(track));
            this._updateResults();
        } catch (error) {
            this._showError(`Failed to fetch tracks: ${error.message}`);
        }
    }

    _formatAlbumLine(album) {
        const rating = album.rating !== null ? album.rating : '-';
        const cols = CONFIG.columns;
        
        const albumartist = String(album.albumartist || '').padEnd(cols.albumartist_width);
        const date = String(album.date || '0000').padEnd(cols.date_width);
        const albumName = String(album.album || '').padEnd(cols.album_width);
        const id = String(album.id || '').padEnd(cols.id_width);
        
        return `${albumartist} ${date} ${albumName} ${id} r=${rating}`;
    }

    _formatTrackLine(track) {
        const rating = '-';
        const cols = CONFIG.columns;
        
        const safeString = (value, width) => {
            const str = String(value || '');
            return str.padEnd ? str.padEnd(width) : str;
        };
        
        const trackNum = Array.isArray(track.track) ? (track.track[0] || '') : (track.track || '');
        const trackNumStr = safeString(trackNum, cols.track_width);
        const title = safeString(track.title, cols.title_width);
        const artist = safeString(track.artist, cols.artist_width);
        const album = safeString(track.album, cols.album_width);
        const date = safeString(track.date || '0000', cols.date_width);
        const id = safeString(track.id, cols.id_width);
        
        return `${trackNumStr} ${title} ${artist} ${album} ${date} ${id} r=${rating}`;
    }

    _showUI() {
        const monitor = Main.layoutManager.primaryMonitor;
        this.actor.set_position(monitor.x, monitor.y);
        this.actor.set_size(monitor.width, monitor.height);
        
        Main.uiGroup.add_child(this.actor);
        global.stage.set_key_focus(this.entry);
        this.entry.grab_key_focus();
    }

    _showError(message) {
        this._items = [message];
        this._updateResults();
    }

    hide() {
        if (this._filter_timeout) {
            GLib.source_remove(this._filter_timeout);
            this._filter_timeout = null;
        }
        if (this.actor.get_parent()) {
            Main.uiGroup.remove_child(this.actor);
        }
    }

    _onTextChanged() {
        if (this._filter_timeout) {
            GLib.source_remove(this._filter_timeout);
        }
        
        this._filter_timeout = GLib.timeout_add(GLib.PRIORITY_DEFAULT, 300, () => {
            this._selected_index = 0;
            this._updateResults();
            this._filter_timeout = null;
            return GLib.SOURCE_REMOVE;
        });
    }
    
    _onKeyPress(actor, event) {
        const symbol = event.get_key_symbol();
        
        if (symbol === Clutter.KEY_Escape) {
            this.hide();
            return Clutter.EVENT_STOP;
        } else if (symbol === Clutter.KEY_Up) {
            if (this._visible_items.length > 0) {
                this._selected_index = Math.max(0, this._selected_index - 1);
                this._updateScrollWindow();
                this._renderVisibleItems();
            }
            return Clutter.EVENT_STOP;
        } else if (symbol === Clutter.KEY_Down) {
            if (this._visible_items.length > 0) {
                this._selected_index = Math.min(this._visible_items.length - 1, this._selected_index + 1);
                this._updateScrollWindow();
                this._renderVisibleItems();
            }
            return Clutter.EVENT_STOP;
        } else if (symbol === Clutter.KEY_Return || symbol === Clutter.KEY_KP_Enter) {
            this._onActivate();
            return Clutter.EVENT_STOP;
        }
        return Clutter.EVENT_PROPAGATE;
    }

    async _onActivate() {
        if (this._visible_items.length > 0 && this._selected_index < this._visible_items.length) {
            const selectedItem = this._visible_items[this._selected_index];
            
            if (this._inActionMode) {
                this._inActionMode = false;
                await this._actionCallback(selectedItem);
                this.hide();
            } else if (this._inRatingMode) {
                this._inRatingMode = false;
                await this._ratingCallback(selectedItem);
                this.hide();
            } else {
                this._handleSelection(selectedItem);
            }
        }
    }

    _handleSelection(selectedLine) {
        const parts = selectedLine.split(/\s+/);
        if (parts.length >= 2) {
            const id = parts[parts.length - 2];
            
            if (this._current_mode === 'albums' || this._current_mode === 'latest_albums') {
                this._showAlbumActions(id, selectedLine);
            } else if (this._current_mode === 'tracks') {
                this._showTrackActions(id, selectedLine);
            }
        }
    }

    _showAlbumActions(albumId, selectedLine) {
        const actions = ['Add', 'Insert', 'Replace', 'Rate'];
        this._showActionMenu(actions, async (action) => {
            if (action === 'Rate') {
                this._showRatingMenu(albumId, selectedLine, 'album');
            } else {
                await this._performAlbumAction(albumId, action.toLowerCase());
            }
        });
    }

    _showTrackActions(trackId, selectedLine) {
        const actions = ['Add', 'Insert', 'Replace', 'Rate Track (MPD Sticker)'];
        this._showActionMenu(actions, async (action) => {
            if (action === 'Rate Track (MPD Sticker)') {
                this._showRatingMenu(trackId, selectedLine, 'track');
            } else {
                await this._performTrackAction(trackId, action.toLowerCase());
            }
        });
    }

    _showActionMenu(actions, callback) {
        this._items = actions;
        this._visible_items = actions;
        this._selected_index = 0;
        this.prompt_label.set_text('Action: ');
        this.entry.set_text('');
        
        this._actionCallback = callback;
        this._inActionMode = true;
        
        this._renderVisibleItems();
    }

    _showRatingMenu(itemId, selectedLine, type) {
        const ratings = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '---', 'Delete'];
        this._items = ratings;
        this._visible_items = ratings;
        this._selected_index = 0;
        this.prompt_label.set_text(`Rate ${type}: `);
        this.entry.set_text('');
        
        this._ratingCallback = async (rating) => {
            await this._performRating(itemId, rating, type);
        };
        this._inRatingMode = true;
        
        this._renderVisibleItems();
    }

    async _performAlbumAction(albumId, mode) {
        try {
            let listMode = this._current_mode;
            if (listMode === 'latest_albums') listMode = 'latest';
            if (listMode === 'albums') listMode = 'album';
            
            const response = await this._makeApiRequest(`playlist/add/album/${albumId}`, 'POST', {mode: mode, list_mode: listMode});
            const albumName = this._getAlbumNameById(albumId) || `ID ${albumId}`;
            this._showNotification(`Album "${albumName}" ${mode}ed to playlist`);
        } catch (error) {
            this._showNotification(`Failed to ${mode} album: ${error.message}`);
        }
    }

    async _performTrackAction(trackId, mode) {
        try {
            const response = await this._makeApiRequest(`playlist/add/track/${trackId}`, 'POST', {mode: mode});
            this._showNotification(`Track ${trackId} ${mode}ed to playlist`);
        } catch (error) {
            this._showNotification(`Failed to ${mode} track: ${error.message}`);
        }
    }

    async _performRating(itemId, rating, type) {
        try {
            let endpoint = type === 'album' ? `albums/${itemId}/rating` : `tracks/${itemId}/rating`;
            const response = await this._makeApiRequest(endpoint, 'POST', {rating: rating});
            
            if (response && response.changed) {
                this._showNotification(`Rated ${type} ${itemId} as ${rating}`);
            } else {
                this._showNotification(`${type} ${itemId} rating already ${rating} or no change needed`);
            }
        } catch (error) {
            this._showNotification(`Failed to rate ${type}: ${error.message}`);
        }
    }

    async performRandomAlbum() {
        try {
            await this._makeApiRequest('playback/random/album', 'POST');
            this._showNotification('Random album started');
        } catch (error) {
            this._showNotification(`Failed to start random album: ${error.message}`);
        }
    }

    async performRandomTracks() {
        try {
            await this._makeApiRequest('playback/random/tracks', 'POST');
            this._showNotification('Random tracks started');
        } catch (error) {
            this._showNotification(`Failed to start random tracks: ${error.message}`);
        }
    }

    async performCacheUpdate() {
        try {
            await this._makeApiRequest('cache/update', 'POST');
            this._showNotification('Cache update completed');
        } catch (error) {
            this._showNotification(`Cache update failed: ${error.message}`);
        }
    }

    _getAlbumNameById(albumId) {
        if (this._current_data) {
            const album = this._current_data.find(item => item.id == albumId);
            return album ? album.album : null;
        }
        return null;
    }

    _showNotification(message) {
        Main.notify('Clerk GNOME', message);
    }

    _updateResults() {
        const filter = this.entry.get_text().trim().toLowerCase();
        this._visible_items = [];
        
        const filterTokens = filter.split(/\s+/).filter(token => token.length > 0);
        
        for (const item of this._items) {
            const itemLower = item.toLowerCase();
            
            if (filterTokens.length === 0) {
                this._visible_items.push(item);
                continue;
            }
            
            const allTokensMatch = filterTokens.every(token => itemLower.includes(token));
            if (allTokensMatch) {
                this._visible_items.push(item);
            }
        }

        this._updateScrollWindow();
        this._renderVisibleItems();
    }
    
    _updateScrollWindow() {
        if (this._visible_items.length === 0) {
            this._scroll_start_index = 0;
            return;
        }
        
        const buffer = Math.floor(this._items_per_page / 4);
        
        if (this._selected_index < this._scroll_start_index + buffer) {
            this._scroll_start_index = Math.max(0, this._selected_index - buffer);
        } else if (this._selected_index >= this._scroll_start_index + this._items_per_page - buffer) {
            this._scroll_start_index = Math.min(
                this._visible_items.length - this._items_per_page,
                this._selected_index - this._items_per_page + buffer + 1
            );
        }
        
        this._scroll_start_index = Math.max(0, this._scroll_start_index);
    }
    
    _renderVisibleItems() {
        this.results_box.remove_all_children();
        
        const end_index = Math.min(
            this._scroll_start_index + this._items_per_page,
            this._visible_items.length
        );
        
        for (let i = this._scroll_start_index; i < end_index; i++) {
            const item = this._visible_items[i];
            const itemContainer = new St.BoxLayout({ style_class: 'dmenu-result-item', vertical: false });
            
            const label = new St.Label({ text: item, x_align: Clutter.ActorAlign.FILL });
            
            itemContainer.add_child(label);
            this.results_box.add_child(itemContainer);
            
            if (i === this._selected_index) {
                itemContainer.add_style_class_name('selected');
            }
        }
    }
}

export default class ClerkExtension extends Extension {
    enable() {
        this._ui = new ClerkUI(this);
        this._setupKeybindings();
    }

    disable() {
        this._removeKeybindings();
        if (this._ui) {
            this._ui.hide();
            this._ui = null;
        }
    }

    _setupKeybindings() {
        this._keybindings = [];
        
        const addKeybinding = (name, callback) => {
            this._keybindings.push(name);
            Main.wm.addKeybinding(
                name,
                this.getSettings(),
                Meta.KeyBindingFlags.NONE,
                Shell.ActionMode.NORMAL,
                callback
            );
        };

        // Setup all hotkeys
        addKeybinding('show-albums', () => {
            this._ui.showAlbums('albums');
        });

        addKeybinding('show-latest-albums', () => {
            this._ui.showAlbums('latest_albums');
        });

        addKeybinding('show-tracks', () => {
            this._ui.showTracks();
        });

        addKeybinding('random-album', () => {
            this._ui.performRandomAlbum();
        });

        addKeybinding('random-tracks', () => {
            this._ui.performRandomTracks();
        });

        addKeybinding('cache-update', () => {
            this._ui.performCacheUpdate();
        });
    }

    _removeKeybindings() {
        if (this._keybindings) {
            this._keybindings.forEach(name => {
                Main.wm.removeKeybinding(name);
            });
            this._keybindings = [];
        }
    }
}