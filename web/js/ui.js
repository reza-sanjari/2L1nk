
function openSidebar() {
    document.querySelector('nav').classList.add('open');
    document.getElementById('nav-overlay').classList.add('active');
    document.body.style.overflow = 'hidden';
    const icon = document.querySelector('.nav-toggle i');
    if (icon) { icon.classList.remove('fa-bars'); icon.classList.add('fa-times'); }
}

function closeSidebar() {
    document.querySelector('nav').classList.remove('open');
    document.getElementById('nav-overlay').classList.remove('active');
    document.body.style.overflow = '';
    const icon = document.querySelector('.nav-toggle i');
    if (icon) { icon.classList.remove('fa-times'); icon.classList.add('fa-bars'); }
}

async function whoAmI() {
    const username = sessionStorage.getItem('username');
    const sessionId = sessionStorage.getItem('sessionId');

    if (!username || !sessionId) {
        window.location.href = 'Login';
        return null;
    }

    try {
        const res = await authFetch('GET', '/api/users/me');
        if (res.status === 401 || res.status === 403) {
            sessionStorage.clear();
            window.location.href = 'Login';
            return null;
        }
        if (res.ok) {
            const data = await res.json();
            if (data.publicFingerPrint) {
                sessionStorage.setItem('my_fingerprint', data.publicFingerPrint);
            }
            if (data.mode !== undefined) {
                sessionStorage.setItem('my_mode', data.mode);
            }
        }
    } catch (e) {
        console.warn('whoAmI: Server nicht erreichbar', e);
    }

    const usernamefield = document.getElementById('username');
    usernamefield.textContent = username;
    const usernameshortfield = document.getElementById('usernameshort');
    usernameshortfield.textContent = username.substring(0, 2).toUpperCase();

    if (Number(sessionStorage.getItem('my_mode') ?? 1) === 0) {
        const btn = document.querySelector('.user-popup-btn--danger');
        if (btn) btn.style.display = 'none';
    }

    return username;
}

function logout() {
    sessionStorage.clear();
    window.location.href = 'Login';
}

function toggleUserPopup() {
    const popup = document.getElementById('user-popup');
    popup.classList.toggle('open');
}

document.addEventListener('click', (e) => {
    const popup = document.getElementById('user-popup');
    if (!popup) return;
    if (!popup.contains(e.target) && e.target.id !== 'avatar' && !document.getElementById('avatar').contains(e.target)) {
        popup.classList.remove('open');
    }
});

async function purgeAllMessages() {
    if (!confirm('Really delete all your messages?')) return;
    const res = await authFetch('DELETE', '/api/users/me/messages');
    if (res.ok) {
        document.getElementById('user-popup').classList.remove('open');
        const isEphemeral = Number(sessionStorage.getItem('my_mode') ?? 1) === 0;
        if (isEphemeral) {
            const myFP = sessionStorage.getItem('my_fingerprint');
            const chatEl = document.getElementById('chat');
            if (chatEl && myFP) {
                chatEl.querySelectorAll(`[data-sender-fp="${CSS.escape(myFP)}"]`).forEach(el => el.remove());
            }
            for (const [roomId, msgs] of roomMessageCache.entries()) {
                roomMessageCache.set(roomId, msgs.filter(m => !m.isMine));
            }
        }
    } else {
        alert('Error deleting messages');
    }
}

// Nav panels
const _panelOrigins = new Map();

function _isMobile() { return window.innerWidth <= 768; }

function _movePanelToBody(panel) {
    if (!_panelOrigins.has(panel.id)) {
        _panelOrigins.set(panel.id, panel.parentElement);
    }
    const root = document.getElementById('mobile-panel-root');
    if (panel.parentElement !== root) root.appendChild(panel);
}

function _movePanelBack(panel) {
    const origin = _panelOrigins.get(panel.id);
    if (origin && panel.parentElement !== origin) origin.appendChild(panel);
}

function _openOverlay() {
    const ov = document.getElementById('nav-overlay');
    ov.classList.add('active');
    document.body.style.overflow = 'hidden';
}

function _closeOverlay() {
    const ov = document.getElementById('nav-overlay');
    ov.classList.remove('active');
    document.body.style.overflow = '';
}

function closeMobilePanel() {
    document.querySelectorAll('#mobile-panel-root .nav-panel').forEach(p => {
        p.classList.remove('open');
        _movePanelBack(p);
    });
    _closeOverlay();
}

function toggleNavPanel(id) {
    const panel = document.getElementById(id);
    const isOpen = panel.classList.contains('open');

    document.querySelectorAll('.nav-panel').forEach(p => {
        p.classList.remove('open');
        if (_isMobile()) _movePanelBack(p);
    });
    _closeOverlay();

    if (!isOpen) {
        if (_isMobile()) _movePanelToBody(panel);
        panel.classList.add('open');
        if (id === 'settings-panel') populateSettingsPanel();
        if (id === 'notif-panel') populateNotifPanel();
        if (_isMobile()) _openOverlay();
    }
}

function closeNavPanel(id) {
    const panel = document.getElementById(id);
    panel.classList.remove('open');
    if (_isMobile()) {
        _movePanelBack(panel);
        _closeOverlay();
    }
}

document.addEventListener('click', (e) => {
    if (!e.target.closest('.nav-panel-wrapper') && !e.target.closest('#mobile-panel-root')) {
        document.querySelectorAll('.nav-panel').forEach(p => {
            p.classList.remove('open');
            if (_isMobile()) _movePanelBack(p);
        });
        _closeOverlay();
    }
});

function populateSettingsPanel() {
    const s = Settings.load();

    const swatchContainer = document.getElementById('settings-swatches');
    if (swatchContainer && !swatchContainer.dataset.built) {
        swatchContainer.dataset.built = '1';
        Settings.PRESETS.forEach(p => {
            const sw = document.createElement('div');
            sw.className = 'settings-swatch';
            sw.style.background = p.color;
            sw.title = p.name;
            sw.dataset.color = p.color;
            sw.onclick = () => {
                const cur = Settings.load();
                cur.accentColor = p.color;
                cur.accentRgb = p.rgb;
                cur.accentDark = p.dark;
                Settings.save(cur);
                Settings.apply(cur);
                syncSettingsUI(cur);
            };
            swatchContainer.appendChild(sw);
        });
        const customWrap = document.createElement('div');
        customWrap.className = 'settings-swatch-custom';
        customWrap.title = 'Custom color';
        customWrap.style.position = 'relative';
        customWrap.innerHTML = '<i class="fas fa-plus" style="pointer-events:none;font-size:0.65rem"></i>';
        const colorInput = document.createElement('input');
        colorInput.type = 'color';
        colorInput.style.cssText = 'position:absolute;width:200%;height:200%;opacity:0;cursor:pointer;top:-50%;left:-50%;';
        colorInput.oninput = (e) => {
            const hex = e.target.value;
            const r = parseInt(hex.slice(1, 3), 16), g = parseInt(hex.slice(3, 5), 16), b = parseInt(hex.slice(5, 7), 16);
            const cur = Settings.load();
            cur.accentColor = hex;
            cur.accentRgb = `${r}, ${g}, ${b}`;
            cur.accentDark = `#${Math.round(r * 0.4).toString(16).padStart(2, '0')}${Math.round(g * 0.4).toString(16).padStart(2, '0')}${Math.round(b * 0.4).toString(16).padStart(2, '0')}`;
            Settings.save(cur);
            Settings.apply(cur);
            syncSettingsUI(cur);
        };
        customWrap.appendChild(colorInput);
        swatchContainer.appendChild(customWrap);
    }

    const fp = sessionStorage.getItem('my_fingerprint') ?? '–';
    const fpEl = document.getElementById('settings-fp');
    if (fpEl) { fpEl.textContent = fp.length > 16 ? fp.slice(0, 8) + '…' + fp.slice(-8) : fp; fpEl.title = fp; }
    const modeEl = document.getElementById('settings-mode');
    if (modeEl) modeEl.textContent = sessionStorage.getItem('my_mode') ?? '–';

    syncSettingsUI(s);
}

let _searchTimer = null;

function globalSearch(event) {
    clearTimeout(_searchTimer);
    const query = event.target.value.trim();
    const resultsEl = document.getElementById('global-search-results');
    if (!resultsEl) return;

    if (!query) {
        resultsEl.innerHTML = '';
        return;
    }

    resultsEl.innerHTML = '<div class="nav-panel-empty search-loading"><i class="fas fa-circle-notch fa-spin"></i> Searching…</div>';
    _searchTimer = setTimeout(() => _runMessageSearch(query, resultsEl), 350);
}

async function _runMessageSearch(query, resultsEl) {
    const q = query.toLowerCase();
    const results = [];

    roomList.forEach(room => {
        if (room.name?.toLowerCase().includes(q)) {
            results.push({ type: 'room', room });
        }
    });

    await Promise.all(roomList.map(async room => {
        try {
            const slotsRes = await authFetch('GET', `/api/rooms/${room.room_id}/key-slots`);
            if (slotsRes.ok) {
                const slotsData = await slotsRes.json();
                for (const slot of (slotsData.key_slots ?? [])) {
                    try {
                        const keyData = JSON.parse(atob(slot.encrypted_key));
                        roomKeys.set(`${slot.room_id}:${slot.epoch}`, AppCrypto.decryptRoomKey(keyData));
                    } catch { }
                }
            }

            const res = await authFetch('GET', `/api/rooms/${room.room_id}/messages`);
            if (!res.ok) return;
            const data = await res.json();
            for (const msg of (data.messages ?? [])) {
                if (!msg.ciphertext) continue;
                const text = decryptText(msg.ciphertext, room.room_id, msg.epoch);
                if (!text) continue;
                if (text.toLowerCase().includes(q)) {
                    const start = Math.max(0, text.toLowerCase().indexOf(q) - 30);
                    const excerpt = (start > 0 ? '…' : '') + text.slice(start, start + 80) + (text.length > start + 80 ? '…' : '');
                    results.push({ type: 'message', room, msgId: msg.id, excerpt });
                }
            }
        } catch { }
    }));

    if (results.length === 0) {
        resultsEl.innerHTML = '<div class="nav-panel-empty">No results</div>';
        return;
    }

    resultsEl.innerHTML = '';

    results.forEach(r => {
        const item = document.createElement('div');
        item.className = 'nav-panel-result-item';

        if (r.type === 'room') {
            item.innerHTML = `<i class="fas fa-comments"></i><span>${escapeHtml(r.room.name)}</span>`;
            item.onclick = () => {
                closeNavPanel('search-panel');
                document.getElementById('global-search-input').value = '';
                resultsEl.innerHTML = '';
                clickroom(r.room);
            };
        } else {
            item.classList.add('search-msg-result');
            item.innerHTML = `
                <i class="fas fa-comment-dots"></i>
                <div class="search-msg-content">
                    <div class="search-msg-room">${escapeHtml(r.room.name)}</div>
                    <div class="search-msg-excerpt">${escapeHtml(r.excerpt)}</div>
                </div>`;
            item.onclick = () => {
                closeNavPanel('search-panel');
                document.getElementById('global-search-input').value = '';
                resultsEl.innerHTML = '';
                pendingScrollMsgId = r.msgId;
                clickroom(r.room);
            };
        }

        resultsEl.appendChild(item);
    });
}

function updateUnreadUI(roomId) {
    const count = unreadCounts.get(roomId) ?? 0;

    const badge = document.querySelector(`.chat-item[data-room-id="${roomId}"] .unread-badge`);
    if (badge) {
        badge.textContent = count > 99 ? '99+' : count;
        badge.style.display = count > 0 ? 'flex' : 'none';
    }

    const total = [...unreadCounts.values()].reduce((a, b) => a + b, 0);
    const bellBadge = document.getElementById('bell-badge');
    if (bellBadge) {
        bellBadge.textContent = total > 99 ? '99+' : total;
        bellBadge.style.display = total > 0 ? 'flex' : 'none';
    }

    if (document.getElementById('notif-panel')?.classList.contains('open')) {
        populateNotifPanel();
    }
}

function populateNotifPanel() {
    const list = document.getElementById('notif-list');
    if (!list) return;

    const rooms = roomList.filter(r => (unreadCounts.get(r.room_id) ?? 0) > 0);
    if (rooms.length === 0) {
        list.innerHTML = '<div class="nav-panel-empty">No new notifications</div>';
        return;
    }

    list.innerHTML = '';
    rooms.forEach(room => {
        const count = unreadCounts.get(room.room_id);
        const item = document.createElement('div');
        item.className = 'nav-panel-result-item';
        item.innerHTML = `<i class="fas fa-comments"></i><span style="flex:1">${room.name}</span><span class="notif-count">${count > 99 ? '99+' : count}</span>`;
        item.onclick = () => {
            closeNavPanel('notif-panel');
            clickroom(room);
        };
        list.appendChild(item);
    });
}

window.addEventListener('DOMContentLoaded', async () => {
    Settings.apply(Settings.load());
    await whoAmI();
    loadIceConfig();
    connectLocalChat();
    fetchRooms();
});

window.addEventListener('resize', () => {
    if (window.innerWidth > 768) {
        document.querySelector('nav').classList.remove('open');
        document.getElementById('nav-overlay').classList.remove('active');
        document.body.style.overflow = '';
    }
});
