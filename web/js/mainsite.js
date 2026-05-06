
let socket;
let roomList = [];

function escapeHtml(str) {
    const d = document.createElement("div");
    d.textContent = str;
    return d.innerHTML;
}
let gefilterteListe = [];
let currentRoomId = null;
let currentRoomEpoch = 0;
const pendingSent = new Set(); // eigene gesendete Nachrichten (roomId:ciphertext) — verhindert Echo-Duplikate
const roomKeys = new Map();   // `${roomId}:${epoch}` → Uint8Array (Room-Key)
// fingerprint → base64 Ed25519 public key. Used to verify per-message
// signatures (V-02). Populated from every server response that lists members:
// GET /rooms (fetchRooms), room_updated WS events, room_key_rotation payloads,
// and room-detail fetches inside loadChat.
const memberEd25519Keys = new Map();
const unreadCounts = new Map(); // roomId → number (ungelesene Nachrichten)
// In-memory message cache for ephemeral rooms (never stored in DB).
// Structure: roomId → [{isMine, senderName, senderMode, text, time}]
const roomMessageCache = new Map();
// Messages that arrived before the room key was available — retried on room_key_slot.
// Structure: roomId → [{ciphertext, epoch, senderName, senderMode, time}]
const pendingEncryptedMessages = new Map();
let pendingScrollMsgId = null;  // nach loadChat zu dieser Nachrichten-ID scrollen

// ============================================================
// SETTINGS MODULE
// ============================================================
const Settings = (() => {
    const KEY = '2l1nk_settings';
    const DEFAULTS = {
        accentColor: '#bc13fe', accentRgb: '188, 19, 254', accentDark: '#4b0082',
        bgFrom: '#1a0525', bgTo: '#310a5d', shapeColor: '#4b0082',
        bgStyle: 'shapes', glow: true, bubbleSquare: false,
        fontSize: 'normal', timestamps: false, compact: false,
        notifSound: true, notifDesktop: false,
    };
    const PRESETS = [
        { name: 'Lila', color: '#bc13fe', rgb: '188, 19, 254', dark: '#4b0082' },
        { name: 'Blau', color: '#1d9bf0', rgb: '29, 155, 240', dark: '#0a3d6b' },
        { name: 'Grün', color: '#00c853', rgb: '0, 200, 83', dark: '#005723' },
        { name: 'Rot', color: '#f44336', rgb: '244, 67, 54', dark: '#7f0000' },
        { name: 'Orange', color: '#ff6d00', rgb: '255, 109, 0', dark: '#7f3400' },
        { name: 'Cyan', color: '#00bcd4', rgb: '0, 188, 212', dark: '#006064' },
    ];

    function load() {
        try {
            const s = localStorage.getItem(KEY);
            return s ? { ...DEFAULTS, ...JSON.parse(s) } : { ...DEFAULTS };
        } catch { return { ...DEFAULTS }; }
    }

    function save(s) { localStorage.setItem(KEY, JSON.stringify(s)); }

    function apply(s) {
        const r = document.documentElement;
        r.style.setProperty('--accent', s.accentColor);
        r.style.setProperty('--accent-rgb', s.accentRgb);
        r.style.setProperty('--accent-dark', s.accentDark);
        r.style.setProperty('--bg-from', s.bgFrom);
        r.style.setProperty('--bg-to', s.bgTo);
        r.style.setProperty('--shape-color', s.shapeColor);
        const b = document.body;
        b.classList.remove('bg-shapes', 'bg-grid', 'bg-gradient', 'bg-none');
        b.classList.add(`bg-${s.bgStyle}`);
        b.classList.toggle('no-glow', !s.glow);
        b.classList.toggle('bubble-square', s.bubbleSquare);
        b.classList.remove('font-sm', 'font-lg');
        if (s.fontSize !== 'normal') b.classList.add(`font-${s.fontSize}`);
        b.classList.toggle('show-timestamps', s.timestamps);
        b.classList.toggle('compact', s.compact);
    }

    return { load, save, apply, PRESETS, DEFAULTS };
})();

function resetSettings() {
    Settings.save({ ...Settings.DEFAULTS });
    Settings.apply(Settings.DEFAULTS);
    syncSettingsUI(Settings.DEFAULTS);
}

function setSetting(key, value) {
    const s = Settings.load();
    if (key === 'notifDesktop' && value) {
        Notification.requestPermission().then(perm => {
            const tog = document.getElementById('tog-desktop');
            if (perm !== 'granted') { if (tog) tog.checked = false; return; }
            s[key] = true;
            Settings.save(s);
            Settings.apply(s);
            syncSettingsUI(s);
        });
        return;
    }
    s[key] = value;
    Settings.save(s);
    Settings.apply(s);
    syncSettingsUI(s);
}

function syncSettingsUI(s) {
    // Swatches
    document.querySelectorAll('.settings-swatch').forEach(el => {
        el.classList.toggle('active', el.dataset.color === s.accentColor);
    });
    // Segmented controls
    [['seg-bg', 'bgStyle', v => v], ['seg-bubble', 'bubbleSquare', v => v ? 'square' : 'round'], ['seg-fontsize', 'fontSize', v => v]].forEach(([id, key, mapFn]) => {
        const seg = document.getElementById(id);
        if (!seg) return;
        const cur = mapFn(s[key]);
        seg.querySelectorAll('button').forEach(btn => btn.classList.toggle('active', btn.dataset.val === String(cur)));
    });
    // Color pickers
    [['bg-from-picker', 'bgFrom'], ['bg-to-picker', 'bgTo'], ['shape-color-picker', 'shapeColor']].forEach(([id, key]) => {
        const el = document.getElementById(id);
        if (el) el.value = s[key];
    });
    // Toggles
    [['tog-glow', 'glow'], ['tog-timestamps', 'timestamps'], ['tog-compact', 'compact'], ['tog-sound', 'notifSound'], ['tog-desktop', 'notifDesktop']].forEach(([id, key]) => {
        const el = document.getElementById(id);
        if (el) el.checked = !!s[key];
    });
}

function playNotifSound() {
    try {
        const ctx = new (window.AudioContext || window.webkitAudioContext)();
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.frequency.setValueAtTime(880, ctx.currentTime);
        osc.frequency.exponentialRampToValueAtTime(660, ctx.currentTime + 0.15);
        gain.gain.setValueAtTime(0.25, ctx.currentTime);
        gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.35);
        osc.start(ctx.currentTime);
        osc.stop(ctx.currentTime + 0.35);
    } catch { }
}

function showDesktopNotif(roomName, text) {
    if (Notification.permission === 'granted') {
        try { new Notification(`2L1nk — ${roomName}`, { body: text || 'Neue Nachricht' }); } catch { }
    }
}

function copySettingsValue(sessionKey, btnId) {
    const val = sessionStorage.getItem(sessionKey) ?? '';
    if (!val) return;
    navigator.clipboard.writeText(val).then(() => {
        const btn = document.getElementById(btnId);
        if (!btn) return;
        btn.classList.add('copied');
        const orig = btn.innerHTML;
        btn.innerHTML = '<i class="fas fa-check"></i> Kopiert';
        setTimeout(() => { btn.innerHTML = orig; btn.classList.remove('copied'); }, 1800);
    }).catch(() => { });
}

function formatTime(unixSeconds) {
    const d = new Date(unixSeconds * 1000);
    const h = d.getHours().toString().padStart(2, '0');
    const m = d.getMinutes().toString().padStart(2, '0');
    return `${h}:${m}`;
}

// ============================================================
// VOICE CHAT STATE
// ============================================================
let voiceRoomId = null;
let localStream = null;
const peerConnections = new Map(); // FP → RTCPeerConnection
const peerStates = new Map();      // FP → { makingOffer, ignoreOffer, polite, roomId, iceRestartTimer }
const pendingIceCandidates = new Map(); // FP → RTCIceCandidate[] (queued before remoteDescription or PC creation)
let isMuted = false;
const voiceParticipants = new Set(); // FP von Usern aktuell in Voice
const mutedUsers = new Set();        // FP von Usern die aktuell gemutet sind

const DEFAULT_ICE_CONFIG = {
    iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        { urls: 'stun:stun1.l.google.com:19302' },
    ]
};
let ICE_CONFIG = DEFAULT_ICE_CONFIG;

async function loadIceConfig() {
    try {
        const res = await fetch('/api/ice-config', { cache: 'no-store' });
        if (!res.ok) throw new Error('ice-config status ' + res.status);
        const cfg = await res.json();
        if (cfg && Array.isArray(cfg.iceServers) && cfg.iceServers.length > 0) {
            ICE_CONFIG = cfg;
        }
    } catch (e) {
        console.warn('ice-config fetch failed, using default STUN', e);
    }
}

// ============================================================
// CRYPTO MODULE — via tweetnacl
// Ed25519 (Signing) + X25519/box (ECDH) + secretbox (AES-äquivalent)
// ============================================================
const AppCrypto = (() => {

    function bufToB64(buf) {
        return btoa(String.fromCharCode(...new Uint8Array(buf)));
    }

    function b64ToBuf(b64) {
        return Uint8Array.from(atob(b64), c => c.charCodeAt(0));
    }

    function strToBytes(str) {
        return new TextEncoder().encode(str);
    }

    function bytesToStr(bytes) {
        return new TextDecoder().decode(bytes);
    }

    // --- Identität generieren & speichern ---

    function generateIdentity() {
        const signingKP = nacl.sign.keyPair();   // Ed25519
        const dhKP = nacl.box.keyPair();    // X25519

        sessionStorage.setItem('ed25519_secret', bufToB64(signingKP.secretKey));
        sessionStorage.setItem('ed25519_public', bufToB64(signingKP.publicKey));
        sessionStorage.setItem('x25519_secret', bufToB64(dhKP.secretKey));
        sessionStorage.setItem('x25519_public', bufToB64(dhKP.publicKey));

        return {
            signingPublicKey: bufToB64(signingKP.publicKey),
            dhPublicKey: bufToB64(dhKP.publicKey)
        };
    }

    function loadIdentity() {
        const edSec = sessionStorage.getItem('ed25519_secret');
        const xSec = sessionStorage.getItem('x25519_secret');
        if (!edSec || !xSec) return null;
        return {
            signingSecretKey: b64ToBuf(edSec),
            dhSecretKey: b64ToBuf(xSec)
        };
    }

    // --- Ed25519: Signieren & Verifizieren ---

    function sign(data) {
        const id = loadIdentity();
        if (!id) throw new Error('Keine Identität geladen');
        const sig = nacl.sign.detached(strToBytes(data), id.signingSecretKey);
        return bufToB64(sig);
    }

    function verify(data, signatureB64, publicKeyB64) {
        return nacl.sign.detached.verify(
            strToBytes(data),
            b64ToBuf(signatureB64),
            b64ToBuf(publicKeyB64)
        );
    }

    // --- X25519 box: Room-Key Verschlüsselung ---

    // Verschlüsselt einen Room-Key für einen Empfänger.
    // Gibt { ephemeralPub, nonce, ciphertext } (alles Base64) zurück.
    function encryptRoomKey(roomKey, recipientDHPublicB64) {
        const ephemeral = nacl.box.keyPair();
        const nonce = nacl.randomBytes(nacl.box.nonceLength);
        const key = roomKey instanceof Uint8Array ? roomKey : new Uint8Array(roomKey);
        const encrypted = nacl.box(key, nonce, b64ToBuf(recipientDHPublicB64), ephemeral.secretKey);
        return {
            ephemeralPub: bufToB64(ephemeral.publicKey),
            nonce: bufToB64(nonce),
            ciphertext: bufToB64(encrypted)
        };
    }

    // Entschlüsselt einen Room-Key mit dem eigenen X25519-Private-Key.
    function decryptRoomKey({ ephemeralPub, nonce, ciphertext }) {
        const id = loadIdentity();
        if (!id) throw new Error('Keine Identität geladen');
        const decrypted = nacl.box.open(
            b64ToBuf(ciphertext), b64ToBuf(nonce),
            b64ToBuf(ephemeralPub), id.dhSecretKey
        );
        if (!decrypted) throw new Error('Entschlüsselung fehlgeschlagen');
        return decrypted;
    }

    // --- secretbox: Nachrichten verschlüsseln & entschlüsseln ---

    function encryptMessage(plaintext, roomKey) {
        const nonce = nacl.randomBytes(nacl.secretbox.nonceLength);
        const key = roomKey instanceof Uint8Array ? roomKey : new Uint8Array(roomKey);
        const encrypted = nacl.secretbox(strToBytes(plaintext), nonce, key);
        return { nonce: bufToB64(nonce), ciphertext: bufToB64(encrypted) };
    }

    function decryptMessage({ nonce, ciphertext }, roomKey) {
        const key = roomKey instanceof Uint8Array ? roomKey : new Uint8Array(roomKey);
        const decrypted = nacl.secretbox.open(b64ToBuf(ciphertext), b64ToBuf(nonce), key);
        if (!decrypted) throw new Error('Entschlüsselung fehlgeschlagen');
        return bytesToStr(decrypted);
    }

    function generateRoomKey() {
        return nacl.randomBytes(32);
    }

    return {
        generateIdentity,
        loadIdentity,
        sign,
        verify,
        encryptRoomKey,
        decryptRoomKey,
        encryptMessage,
        decryptMessage,
        generateRoomKey,
        bufToB64,
        b64ToBuf
    };
})();
if (!AppCrypto.loadIdentity()) AppCrypto.generateIdentity();
// Gibt den entschlüsselten Text zurück, oder null wenn kein Key vorhanden / Fehler / leer.
function decryptText(ciphertextStr, roomId, epoch) {
    try {
        const roomKey = roomKeys.get(`${roomId}:${epoch}`);
        if (!roomKey) return null;
        const { nonce, ciphertext } = JSON.parse(ciphertextStr);
        const text = AppCrypto.decryptMessage({ nonce, ciphertext }, roomKey);
        return text || null;
    } catch {
        return null;
    }
}

// Generiert einen neuen Room-Key, speichert ihn lokal und POSTet die
// verschlüsselten Key-Slots für alle Mitglieder an den Server.
async function submitRoomKey(roomId, epoch, members) {
    const roomKey = AppCrypto.generateRoomKey();
    roomKeys.set(`${roomId}:${epoch}`, roomKey);

    const keys = members.map(m => ({
        recipient_fp: m.fingerprint,
        encrypted_key: btoa(JSON.stringify(AppCrypto.encryptRoomKey(roomKey, m.x25519_public_key)))
    }));

    try {
        const res = await authFetch('POST', `/api/rooms/${roomId}/epoch-keys`, { epoch, keys });
        if (!res.ok) console.error('submitRoomKey fehlgeschlagen:', await res.text());
    } catch (e) {
        console.error('submitRoomKey Netzwerkfehler:', e);
    }
}

let wsReconnectAttempts = 0;
let wsReconnectTimeout = null;

function scheduleWsReconnect() {
    if (wsReconnectTimeout) return;
    const delay = Math.min(1000 * Math.pow(2, wsReconnectAttempts), 30000);
    wsReconnectAttempts++;
    wsReconnectTimeout = setTimeout(() => {
        wsReconnectTimeout = null;
        connectLocalChat();
    }, delay);
}

function connectLocalChat() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(`${wsProtocol}//${window.location.host}/api/ws`);

    socket.onopen = () => {
        wsReconnectAttempts = 0;
        const sessionId = sessionStorage.getItem('sessionId');
        const timestamp = Math.floor(Date.now() / 1000);
        const canonical = `WS\n${sessionId}\n${timestamp}`;
        const signature = AppCrypto.sign(canonical);
        const authPayload = {
            "type": "auth",
            "payload": {
                "Chat-Session-ID": sessionId,
                "Chat-Timestamp": timestamp,
                "Chat-Signature": signature
            }
        };
        socket.send(JSON.stringify(authPayload));
        // Re-fetch rooms after a short delay so the hub has time to process
        // RegisterUser + AddToRoom before the HTTP request arrives.
        // Without the delay, fetchRooms arrives before the hub registers the user
        // in room.Users, so host is null and the manage-members button is missing.
        if (sessionId) setTimeout(fetchRooms, 1200);
    };

    socket.onmessage = (event) => {
        const envelope = JSON.parse(event.data);

        if (envelope.type === "join_room" || envelope.type === "leave_room") {
            fetchRooms();
            return;
        }

        if (envelope.type === "room_updated") {
            const p = envelope.payload;
            // Refresh Ed25519 pk cache from the updated member list so newly-
            // added members can be verified (V-02).
            if (p.host) cacheMemberEd25519Keys([p.host]);
            cacheMemberEd25519Keys(p.users ?? []);
            // Update the cached room entry
            const idx = roomList.findIndex(r => r.room_id === p.room_id);
            if (idx !== -1) {
                const oldRoom = roomList[idx];
                const merged = { ...oldRoom, ...p };
                // If the old host was a persistent user, keep them as host in the UI.
                // Persistent users may reconnect — only ephemeral hosts should trigger
                // an immediate ownership handover.
                const oldHostPersistent = oldRoom.host && oldRoom.host.mode !== 0;
                const hostChanged = p.host && oldRoom.host && p.host.fingerprint !== oldRoom.host.fingerprint;
                if (oldHostPersistent && hostChanged) {
                    merged.host = oldRoom.host;
                }
                roomList[idx] = merged;
            }
            renderFunc(roomList);
            // If this is the open room, refresh the member list too
            if (p.room_id === currentRoomId && Array.isArray(p.users)) {
                renderChatUserList(p.users);
            }
            return;
        }

        if (envelope.type === "room_key_rotation") {
            const p = envelope.payload;
            // Always update roomList so clickroom picks up the correct epoch.
            const rotIdx = roomList.findIndex(r => r.room_id === p.room_id);
            if (rotIdx !== -1) roomList[rotIdx] = { ...roomList[rotIdx], epoch: p.epoch };
            if (p.room_id === currentRoomId) currentRoomEpoch = p.epoch;
            // Cache Ed25519 pubkeys for every member listed in the rotation so
            // live messages from anyone in this epoch can be verified (V-02).
            cacheMemberEd25519Keys(p.members ?? []);
            const myFP = sessionStorage.getItem('my_fingerprint');
            if (p.key_creator_fp === myFP) submitRoomKey(p.room_id, p.epoch, p.members);
            return;
        }

        if (envelope.type === "room_key_slot") {
            const p = envelope.payload;
            try {
                const keyData = JSON.parse(atob(p.encrypted_key));
                const roomKey = AppCrypto.decryptRoomKey(keyData);
                roomKeys.set(`${p.room_id}:${p.epoch}`, roomKey);
                // If this is the open room, make sure currentRoomEpoch is in sync.
                if (p.room_id === currentRoomId) currentRoomEpoch = p.epoch;
                // Flush any messages that arrived before the key slot and
                // couldn't be decrypted at the time.
                const pending = pendingEncryptedMessages.get(p.room_id) ?? [];
                if (pending.length > 0) {
                    pendingEncryptedMessages.delete(p.room_id);
                    pending.forEach(m => {
                        const t = decryptText(m.ciphertext, p.room_id, m.epoch);
                        if (!t) return;
                        if (!roomMessageCache.has(p.room_id)) roomMessageCache.set(p.room_id, []);
                        roomMessageCache.get(p.room_id).push({
                            isMine: false, senderFP: m.senderFP,
                            senderName: m.senderName,
                            senderMode: m.senderMode, text: t,
                            time: m.time
                        });
                    });
                }
            } catch (e) {
                console.error('Fehler beim Entschlüsseln des Room-Keys:', e);
            }
            return;
        }

        if (envelope.type === "epoch_mismatch") {
            const p = envelope.payload;
            if (p && p.room_id === currentRoomId) {
                currentRoomEpoch = p.current_epoch;
            }
            return;
        }

        if (envelope.type === "signal") {
            handleSignalMessage(envelope.payload);
            return;
        }

        if (envelope.type === "voice_joined") {
            handleVoiceJoined(envelope.payload);
            return;
        }

        if (envelope.type === "voice_left") {
            handleVoiceLeft(envelope.payload);
            return;
        }

        if (envelope.type === "messages_purged") {
            const p = envelope.payload;
            // Cache-Einträge des Senders aus allen Räumen entfernen
            for (const [roomId, msgs] of roomMessageCache.entries()) {
                roomMessageCache.set(roomId, msgs.filter(m => m.isMine ? true : m.senderFP !== p.sender_fp));
            }
            // Falls der betroffene Raum gerade offen ist, DOM-Bubbles des Senders entfernen
            if (p.room_id === currentRoomId) {
                const chatEl = document.getElementById('chat');
                if (chatEl) {
                    chatEl.querySelectorAll(`[data-sender-fp="${CSS.escape(p.sender_fp)}"]`).forEach(el => el.remove());
                }
            }
            return;
        }

        if (envelope.type === "message") {
            const payload = envelope.payload;

            // eigene Echo-Nachricht ignorieren. sender_fp now lives in the
            // payload (server-constructed outbound, V-02) — the old
            // envelope.sender_fp path is gone for live chat messages.
            const myFP = sessionStorage.getItem('my_fingerprint');
            const sentKey = `${payload.room_id}:${payload.ciphertext}`;
            if (payload.sender_fp === myFP || pendingSent.has(sentKey)) {
                pendingSent.delete(sentKey);
                return;
            }

            // V-02: verify the per-message signature BEFORE any side-effects
            // (unread counters, desktop notifications, cache writes, DOM).
            // Dropping an unverified message means no UI is ever influenced by
            // a forged or tampered message. verifyIncomingMessageSignature
            // handles its own logging.
            (async () => {
                if (!await verifyIncomingMessageSignature(payload)) return;
                renderVerifiedIncomingMessage(payload);
            })();
        }
    };

function renderVerifiedIncomingMessage(payload) {
    // Raum nicht offen → unread zählen, Sound/Desktop, in Cache speichern
    if (payload.room_id !== currentRoomId) {
        unreadCounts.set(payload.room_id, (unreadCounts.get(payload.room_id) ?? 0) + 1);
        updateUnreadUI(payload.room_id);
        const cfg = Settings.load();
        if (cfg.notifSound) playNotifSound();
        if (cfg.notifDesktop) {
            const room = roomList.find(r => r.room_id === payload.room_id);
            showDesktopNotif(room?.name ?? 'Neue Nachricht', null);
        }
        // Cache so it appears when the user opens the room.
        const cachedText = decryptText(payload.ciphertext, payload.room_id, payload.epoch);
        const msgTime = Math.floor(Date.now() / 1000);
        if (cachedText) {
            if (!roomMessageCache.has(payload.room_id)) roomMessageCache.set(payload.room_id, []);
            roomMessageCache.get(payload.room_id).push({
                isMine: false,
                senderFP: payload.sender_fp,
                senderName: payload.sender_name ?? null,
                senderMode: payload.sender_mode ?? 1,
                text: cachedText,
                time: msgTime
            });
        } else {
            // Key not available yet — queue for retry when room_key_slot arrives.
            if (!pendingEncryptedMessages.has(payload.room_id)) pendingEncryptedMessages.set(payload.room_id, []);
            pendingEncryptedMessages.get(payload.room_id).push({
                ciphertext: payload.ciphertext,
                epoch: payload.epoch,
                senderFP: payload.sender_fp,
                senderName: payload.sender_name ?? null,
                senderMode: payload.sender_mode ?? 1,
                time: msgTime
            });
        }
        return;
    }
    const text = decryptText(payload.ciphertext, payload.room_id, payload.epoch);
    if (!text) return;
    const chatEl = document.getElementById('chat');
    if (!chatEl) return;
    const wrapper = document.createElement('div');
    wrapper.className = 'msg-wrapper';
    wrapper.dataset.senderFp = payload.sender_fp;
    if (payload.sender_name) {
        const label = document.createElement('div');
        label.className = 'msg-label';
        if (payload.sender_mode === 0) {
            const badge = document.createElement('span');
            badge.textContent = '👻';
            badge.title = 'Temporärer Nutzer';
            badge.style.cssText = 'margin-right:4px;font-size:0.85em;opacity:0.8;';
            label.appendChild(badge);
        }
        label.appendChild(document.createTextNode(payload.sender_name));
        wrapper.appendChild(label);
    }
    const div = document.createElement('div');
    div.className = 'bubble received';
    div.innerText = text;
    const timeEl = document.createElement('span');
    timeEl.className = 'msg-time';
    timeEl.textContent = formatTime(Math.floor(Date.now() / 1000));
    div.appendChild(timeEl);
    wrapper.appendChild(div);
    chatEl.appendChild(wrapper);
    chatEl.scrollTop = chatEl.scrollHeight;

    // Cache so re-opening the room doesn't wipe ephemeral messages.
    if (!roomMessageCache.has(payload.room_id)) roomMessageCache.set(payload.room_id, []);
    roomMessageCache.get(payload.room_id).push({
        isMine: false,
        senderFP: payload.sender_fp,
        senderName: payload.sender_name ?? null,
        senderMode: payload.sender_mode ?? 1,
        text,
        time: Math.floor(Date.now() / 1000)
    });
    // Remove "no messages" placeholder if still visible.
    chatEl.querySelector('.chat-empty')?.remove();
}

    socket.onerror = (err) => console.error("WebSocket Fehler:", err);
    socket.onclose = () => {
        console.warn("WebSocket geschlossen, reconnect in Kürze...");
        if (sessionStorage.getItem('sessionId')) scheduleWsReconnect();
    };
}
async function fetchRooms() {

    const timestamp = Math.floor(Date.now() / 1000);
    const nonce = crypto.randomUUID();
    const path = '/api/users/me/rooms';

    const emptyBodyHash = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
    const canonical = `GET\n${path}\n${timestamp}\n${emptyBodyHash}\n${nonce}`;
    const signature = AppCrypto.sign(canonical);

    try {
        const response = await fetch(`/api/users/me/rooms`, {
            method: 'GET',
            headers: {
                'Chat-Session-ID': sessionStorage.getItem('sessionId'),
                'Chat-Timestamp': timestamp,
                'Chat-Nonce': nonce,
                'Chat-Signature': signature
            }
        });

        if (!response.ok) {
            throw new Error(`HTTP-Fehler: ${response.status}`);
        }

        roomList = (await response.json()).rooms ?? [];

        // Cache known host fingerprints so offline rooms still show the edit button.
        roomList.forEach(r => {
            if (r.host?.fingerprint) {
                localStorage.setItem(`2l1nk_host_${r.room_id}`, r.host.fingerprint);
            }
            // Seed Ed25519 pk cache from both host and member entries so any
            // incoming message from any member can be verified without waiting
            // for a key rotation event.
            if (r.host) cacheMemberEd25519Keys([r.host]);
            cacheMemberEd25519Keys(r.users ?? []);
        });

        // For rooms the server returned without a host (hub race or room offline),
        // fall back to the cached host fingerprint. Mode is always 1 (persistent)
        // because ephemeral hosts never survive a reload anyway.
        roomList = roomList.map(r => {
            if (!r.host) {
                const cached = localStorage.getItem(`2l1nk_host_${r.room_id}`);
                if (cached) return { ...r, host: { fingerprint: cached, mode: 1 } };
            }
            return r;
        });

        renderFunc(roomList);
        const cr = roomList.find(r => r.room_id === currentRoomId);
        if (cr) renderChatUserList(cr.users);

        // Timing race: if any room still has no host after the cache fill,
        // the hub hasn't settled yet. Retry up to 3 times, 700 ms apart.
        const myFP = sessionStorage.getItem('my_fingerprint');
        if (roomList.some(r => !r.host) && myFP) {
            fetchRooms._retries = (fetchRooms._retries ?? 0);
            if (fetchRooms._retries < 3 && !fetchRooms._retryPending) {
                fetchRooms._retries++;
                fetchRooms._retryPending = true;
                setTimeout(() => { fetchRooms._retryPending = false; fetchRooms(); }, 700);
            }
        } else {
            fetchRooms._retries = 0;
        }
    } catch (error) {

        console.error("Fehler beim Abrufen der Räume:", error);

    }

}
function searchfunction(event) {
    const searchTerm = event.target.value.toLowerCase();

    const gefilterteListe = roomList.filter(room => {
        return room.room_id.toLowerCase().includes(searchTerm) ||
            room.name.toLowerCase().includes(searchTerm);
    });
    renderFunc(gefilterteListe);
}
function toggleChatUserList() {
    const panel = document.getElementById('chat-user-panel');
    if (!panel) return;
    const isOpening = !panel.classList.contains('open');
    if (isOpening) {
        const voicePanel = document.getElementById('voice-panel');
        if (voicePanel) voicePanel.classList.remove('open');
        // Re-fetch online status every time the panel opens so dots are always current.
        const room = roomList.find(r => r.room_id === currentRoomId);
        if (room) renderChatUserList(room.users);
    }
    panel.classList.toggle('open');
}

async function renderChatUserList(users) {
    const list = document.getElementById('chat-user-list');
    if (!list) return;
    list.innerHTML = '';

    // Fetch online status from the server.
    let onlineFPs = new Set();
    try {
        const r = await authFetch('GET', '/api/users');
        if (r?.ok) {
            const all = await r.json();
            if (Array.isArray(all)) {
                onlineFPs = new Set(all.filter(u => u.online).map(u => u.fingerprint));
            }
        }
    } catch {}

    (users ?? []).forEach(u => {
        const isOnline = onlineFPs.has(u.fingerprint);
        const div = document.createElement('div');
        div.className = 'chat-user-entry';
        const dotStyle = isOnline
            ? 'background:#4ade80;box-shadow:0 0 5px rgba(74,222,128,0.5)'
            : 'background:rgba(255,255,255,0.2);box-shadow:none';
        div.innerHTML = `<span class="chat-user-dot" style="${dotStyle}"></span><span>${escapeHtml(u.username)}</span>`;
        list.appendChild(div);
    });
}

function clickroom(room) {
    currentRoomId = room.room_id;
    currentRoomEpoch = room.epoch ?? 0;
    unreadCounts.set(room.room_id, 0);
    updateUnreadUI(room.room_id);
    // Auf Mobile: Sidebar schließen wenn ein Raum geöffnet wird
    if (window.innerWidth <= 768) closeSidebar();

    const maininfo = document.querySelector('.maininfo');
    maininfo.style.display = 'none';
    const main = document.getElementById('main');
    main.style.display = 'flex';
    main.innerHTML = `
                <div class="chat-header">
                    <div class="chat-header-left">
                        <span class="chat-header-name">${escapeHtml(room.name)}</span>
                        <button class="leave-room-btn" onclick="leaveRoom('${room.room_id}')" title="Chat verlassen"><i class="fas fa-sign-out-alt"></i></button>
                    </div>
                    <div class="chat-header-actions">
                        <div class="voice-controls">
                            <div id="voice-avatars" class="voice-avatars-row"></div>
                            <button id="voice-join-btn" class="voice-btn" onclick="toggleVoice('${room.room_id}')">🎙️ Voice</button>
                            <button id="voice-mute-btn" class="voice-btn" onclick="toggleMute()" style="display:none">🎤 Stumm</button>
                        </div>
                        <div class="chat-user-panel-wrapper" style="position:relative">
                            <button class="chat-user-btn" onclick="toggleVoicePanel()" title="Voice-Teilnehmer">
                                <i class="fas fa-headphones"></i>
                            </button>
                            <div class="voice-panel" id="voice-panel">
                                <div class="voice-panel-title">IN VOICE</div>
                                <div id="voice-participants-list"></div>
                            </div>
                        </div>
                        <div class="chat-user-panel-wrapper">
                            <button class="chat-user-btn" onclick="toggleChatUserList()" title="Mitglieder">
                                <i class="fas fa-users"></i>
                            </button>
                            <div class="chat-user-panel" id="chat-user-panel">
                                <div class="chat-user-panel-title">MITGLIEDER</div>
                                <div id="chat-user-list"></div>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="chat-messages" id="chat">
                </div>
                <div class="chat-input">
                    <div class="input-bar">
                        <input type="text" id="schreibnachricht" placeholder="Nachricht schreiben...">
                        <button class="send-btn" onclick="sendMessage('${room.room_id}')" title="Senden">
                            <i class="fas fa-paper-plane"></i>
                        </button>
                    </div>
                </div>
            `;
    document.getElementById('schreibnachricht').addEventListener('keydown', (e) => {
        if (e.key === 'Enter') sendMessage(room.room_id);
    });
    renderChatUserList(room.users);
    loadChat(room);
    updateVoiceUI();
}

function toggleVoicePanel() {
    const panel = document.getElementById('voice-panel');
    if (!panel) return;
    const isOpening = !panel.classList.contains('open');
    if (isOpening) {
        const memberPanel = document.getElementById('chat-user-panel');
        if (memberPanel) memberPanel.classList.remove('open');
    }
    panel.classList.toggle('open');
}
async function loadChat(room) {
    const chatEl = document.getElementById('chat');
    const myFP = sessionStorage.getItem('my_fingerprint');

    const fpToName = {};
    if (room.host) fpToName[room.host.fingerprint] = room.host.username;
    (room.users ?? []).forEach(u => { fpToName[u.fingerprint] = u.username; });
    // V-02: make sure we can verify signatures on historical messages from
    // members who may currently be offline. The room object is the authoritative
    // member list for this chat session.
    if (room.host) cacheMemberEd25519Keys([room.host]);
    cacheMemberEd25519Keys(room.users ?? []);

    // Key Slots laden damit gespeicherte Nachrichten entschlüsselt werden können
    try {
        const slotsRes = await authFetch('GET', `/api/rooms/${room.room_id}/key-slots`);
        if (slotsRes.ok) {
            const slotsData = await slotsRes.json();
            for (const slot of (slotsData.key_slots ?? [])) {
                try {
                    const keyData = JSON.parse(atob(slot.encrypted_key));
                    const roomKey = AppCrypto.decryptRoomKey(keyData);
                    roomKeys.set(`${slot.room_id}:${slot.epoch}`, roomKey);
                } catch (e) {
                    console.error('Fehler beim Laden des Key Slots:', e);
                }
            }
        }
    } catch (e) {
        console.error('Fehler beim Laden der Key Slots:', e);
    }

    try {
        const response = await authFetch('GET', `/api/rooms/${room.room_id}/messages`);
        if (!response.ok) throw new Error(`HTTP-Fehler: ${response.status}`);

        const data = await response.json();
        const messages = (data.messages ?? []).filter(m => m.ciphertext && m.ciphertext.trim() !== '').reverse();

        chatEl.innerHTML = '';

        if (messages.length === 0) {
            // For ephemeral rooms messages are never in DB — fall back to in-memory cache.
            const cached = roomMessageCache.get(room.room_id) ?? [];
            if (cached.length === 0) {
                const empty = document.createElement('div');
                empty.className = 'chat-empty';
                empty.textContent = 'Noch keine Nachrichten.';
                chatEl.appendChild(empty);
                return;
            }
            cached.forEach(msg => {
                if (msg.isMine) {
                    const div = document.createElement('div');
                    div.className = 'bubble sent';
                    div.dataset.senderFp = myFP;
                    div.innerText = msg.text;
                    const timeEl = document.createElement('span');
                    timeEl.className = 'msg-time';
                    timeEl.textContent = formatTime(msg.time);
                    div.appendChild(timeEl);
                    chatEl.appendChild(div);
                } else {
                    const wrapper = document.createElement('div');
                    wrapper.className = 'msg-wrapper';
                    if (msg.senderFP) wrapper.dataset.senderFp = msg.senderFP;
                    if (msg.senderName) {
                        const label = document.createElement('div');
                        label.className = 'msg-label';
                        if (msg.senderMode === 0) {
                            const badge = document.createElement('span');
                            badge.textContent = '👻';
                            badge.title = 'Temporärer Nutzer';
                            badge.style.cssText = 'margin-right:4px;font-size:0.85em;opacity:0.8;';
                            label.appendChild(badge);
                        }
                        label.appendChild(document.createTextNode(msg.senderName));
                        wrapper.appendChild(label);
                    }
                    const div = document.createElement('div');
                    div.className = 'bubble received';
                    div.innerText = msg.text;
                    const timeEl = document.createElement('span');
                    timeEl.className = 'msg-time';
                    timeEl.textContent = formatTime(msg.time);
                    div.appendChild(timeEl);
                    wrapper.appendChild(div);
                    chatEl.appendChild(wrapper);
                }
            });
            chatEl.scrollTop = chatEl.scrollHeight;
            return;
        }

        // V-02: verify signatures on every stored message. Build a canonical
        // that matches what the client signed at send-time; if verify fails the
        // row is either tampered or pre-dates the signing rollout (the user
        // has agreed to reset the DB for this fix, so either case is a bug).
        const verifiedMsgs = [];
        for (const msg of messages) {
            const ok = await verifyIncomingMessageSignature({
                room_id:   room.room_id,
                epoch:     msg.epoch,
                ciphertext: msg.ciphertext,
                sender_fp: msg.sender_fp,
                timestamp: String(msg.sig_timestamp ?? ''),
                nonce:     msg.sig_nonce,
                signature: msg.signature,
            });
            if (ok) verifiedMsgs.push(msg);
        }

        verifiedMsgs.forEach(msg => {
            const isMine = msg.sender_fp === myFP;
            const text = decryptText(msg.ciphertext, room.room_id, msg.epoch);
            if (!text) return;
            const timeEl = document.createElement('span');
            timeEl.className = 'msg-time';
            timeEl.textContent = msg.created_at ? formatTime(msg.created_at) : '';
            if (isMine) {
                const div = document.createElement('div');
                div.className = 'bubble sent';
                div.dataset.msgId = msg.id;
                div.dataset.senderFp = myFP;
                div.innerText = text;
                div.appendChild(timeEl);
                chatEl.appendChild(div);
            } else {
                const wrapper = document.createElement('div');
                wrapper.className = 'msg-wrapper';
                wrapper.dataset.msgId = msg.id;
                wrapper.dataset.senderFp = msg.sender_fp;
                const label = document.createElement('div');
                label.className = 'msg-label';
                if (msg.is_ephemeral) {
                    const badge = document.createElement('span');
                    badge.textContent = '👻';
                    badge.title = 'Temporärer Nutzer';
                    badge.style.cssText = 'margin-right:4px;font-size:0.85em;opacity:0.8;';
                    label.appendChild(badge);
                }
                const senderName = fpToName[msg.sender_fp] ?? (msg.sender_fp.slice(0, 8) + '…');
                label.appendChild(document.createTextNode(senderName));
                wrapper.appendChild(label);
                const div = document.createElement('div');
                div.className = 'bubble received';
                div.innerText = text;
                div.appendChild(timeEl);
                wrapper.appendChild(div);
                chatEl.appendChild(wrapper);
            }
        });

        if (pendingScrollMsgId) {
            const target = chatEl.querySelector(`[data-msg-id="${pendingScrollMsgId}"]`);
            pendingScrollMsgId = null;
            if (target) {
                target.scrollIntoView({ behavior: 'smooth', block: 'center' });
                target.classList.add('msg-highlight');
                setTimeout(() => target.classList.remove('msg-highlight'), 2000);
            } else {
                chatEl.scrollTop = chatEl.scrollHeight;
            }
        } else {
            chatEl.scrollTop = chatEl.scrollHeight;
        }
    } catch (err) {
        console.error("Fehler beim Laden der Nachrichten:", err);
    }
}
function renderFunc(RenderList) {
    const container = document.getElementById('chat-list-container');
    if (!container) return;

    if (RenderList && RenderList.length > 0) {
        container.innerHTML = "";
        const myFP = sessionStorage.getItem('my_fingerprint');

        RenderList.forEach(room => {
            const isHost = room.host?.fingerprint === myFP;
            const div = document.createElement('div');
            div.className = 'chat-item';
            div.setAttribute('data-room-id', room.room_id);

            div.innerHTML = `
                <div class="chat-item-row">
                    <div style="flex:1;min-width:0;cursor:pointer;" class="room-info">
                        <div style="font-weight:bold;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">👤${escapeHtml(room.name)}</div>
                    </div>
                    <span class="unread-badge" style="display:none"></span>
                    ${isHost ? `<span class="room-menu-btn" title="Mitglieder verwalten">llll</span>` : ''}
                </div>`;

            div.querySelector('.room-info').onclick = () => clickroom(room);

            if (isHost) {
                div.querySelector('.room-menu-btn').onclick = (e) => {
                    e.stopPropagation();
                    openRoomMenu(room);
                };
            }

            container.appendChild(div);
        });

        // Badges nach Re-Render wiederherstellen
        unreadCounts.forEach((count, roomId) => {
            if (count > 0) updateUnreadUI(roomId);
        });
    } else {
        container.innerHTML = '<i class="fas fa-users" style="font-size: 2rem; margin-bottom: 10px;"></i><p>Keine aktiven Chats</p>';
    }
}

// ---- Member Modal ----

let activeMemberModal = null;

function closeMemberModal() {
    if (activeMemberModal) { activeMemberModal.remove(); activeMemberModal = null; }
}

async function openRoomMenu(room) {
    closeMemberModal();

    // Overlay
    const overlay = document.createElement('div');
    overlay.className = 'member-modal-overlay';
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeMemberModal(); });

    // Modal
    const modal = document.createElement('div');
    modal.className = 'member-modal';

    // Header
    const header = document.createElement('div');
    header.className = 'member-modal-header';
    const title = document.createElement('h3');
    title.textContent = 'MITGLIEDER VERWALTEN';
    const closeBtn = document.createElement('button');
    closeBtn.className = 'member-modal-close';
    closeBtn.textContent = '✕';
    closeBtn.onclick = closeMemberModal;
    header.appendChild(title);
    header.appendChild(closeBtn);

    // Body
    const body = document.createElement('div');
    body.className = 'member-modal-body';

    // --- linke Spalte: aktuelle Mitglieder ---
    const leftCol = document.createElement('div');
    leftCol.className = 'member-col';
    const leftTitle = document.createElement('div');
    leftTitle.className = 'member-col-title';
    leftTitle.textContent = 'Mitglieder';
    const leftList = document.createElement('div');
    leftList.className = 'member-col-list';
    leftCol.appendChild(leftTitle);
    leftCol.appendChild(leftList);

    // --- rechte Spalte: hinzufügbare User ---
    const rightCol = document.createElement('div');
    rightCol.className = 'member-col';
    const rightTitle = document.createElement('div');
    rightTitle.className = 'member-col-title';
    rightTitle.textContent = 'Hinzufügen';
    const rightList = document.createElement('div');
    rightList.className = 'member-col-list';
    rightCol.appendChild(rightTitle);
    rightCol.appendChild(rightList);

    // Footer: Gruppe löschen
    const footer = document.createElement('div');
    footer.className = 'member-modal-footer';
    const deleteBtn = document.createElement('button');
    deleteBtn.className = 'del-group-btn';
    deleteBtn.textContent = 'Gruppe löschen';
    deleteBtn.onclick = () => deleteGroup(room.room_id);
    footer.appendChild(deleteBtn);

    body.appendChild(leftCol);
    body.appendChild(rightCol);
    modal.appendChild(header);
    modal.appendChild(body);
    modal.appendChild(footer);
    overlay.appendChild(modal);
    document.body.appendChild(overlay);
    activeMemberModal = overlay;

    function makeRow(username, btnClass, btnText, isOnline, onClick) {
        const row = document.createElement('div');
        row.className = 'member-row';
        const nameWrap = document.createElement('span');
        nameWrap.className = 'member-row-name';
        const dot = document.createElement('span');
        const nameText = document.createElement('span');
        nameText.textContent = username;
        nameWrap.appendChild(nameText);
        const btn = document.createElement('button');
        btn.className = btnClass;
        btn.textContent = btnText;
        btn.onclick = onClick;
        row.appendChild(nameWrap);
        row.appendChild(btn);
        return row;
    }

    // Placeholder während Laden
    leftList.innerHTML = '<div class="member-col-empty">Lädt...</div>';
    rightList.innerHTML = '<div class="member-col-empty">Lädt...</div>';

    const myFP = sessionStorage.getItem('my_fingerprint');
    const allResp = await authFetch('GET', '/api/users').catch(() => null);
    const allUsersRaw = allResp?.ok ? (await allResp.json()) : [];
    const allUsers = Array.isArray(allUsersRaw) ? allUsersRaw : [];

    const onlineFPs = new Set(allUsers.filter(u => u.online).map(u => u.fingerprint));
    const memberFPs = new Set((room.users ?? []).map(u => u.fingerprint));
    const removable = (room.users ?? []).filter(u => u.fingerprint !== myFP);
    const addable = allUsers.filter(u => u.online && !memberFPs.has(u.fingerprint) && u.fingerprint !== myFP);

    // Mitglieder-Liste befüllen
    leftList.innerHTML = '';
    if (removable.length === 0) {
        leftList.innerHTML = '<div class="member-col-empty">Keine weiteren Mitglieder</div>';
    } else {
        removable.forEach(u => leftList.appendChild(
            makeRow(u.username, 'rem-btn', '– Entfernen', onlineFPs.has(u.fingerprint), () => removeMember(room.room_id, u.fingerprint))
        ));
    }

    // Hinzufügen-Liste befüllen (nur online)
    rightList.innerHTML = '';
    if (addable.length === 0) {
        rightList.innerHTML = '<div class="member-col-empty">Keine online User verfügbar</div>';
    } else {
        addable.forEach(u => rightList.appendChild(
            makeRow(u.username, 'add-btn', '+ Hinzufügen', true, () => addMember(room.room_id, u))
        ));
    }
}

async function authFetch(method, path, body = null) {
    const bodyString = body ? JSON.stringify(body) : null;
    const timestamp = Math.floor(Date.now() / 1000);
    const nonce = crypto.randomUUID();
    const bodyHash = bodyString
        ? await hashBody(bodyString)
        : 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
    const canonical = `${method}\n${path}\n${timestamp}\n${bodyHash}\n${nonce}`;
    const signature = AppCrypto.sign(canonical);

    const opts = {
        method,
        headers: {
            'Chat-Session-ID': sessionStorage.getItem('sessionId'),
            'Chat-Timestamp': timestamp,
            'Chat-Nonce': nonce,
            'Chat-Signature': signature,
        }
    };
    if (bodyString) {
        opts.body = bodyString;
        opts.headers['Content-Type'] = 'application/json';
    }
    return fetch(`${path}`, opts);
}

async function addMember(roomId, user) {
    const res = await authFetch('POST', `/api/rooms/${roomId}/users/${user.fingerprint}`);
    if (res.ok) {
        await fetchRooms();
        const idx = roomList.findIndex(r => r.room_id === roomId);
        if (idx >= 0 && !roomList[idx].users?.some(u => u.fingerprint === user.fingerprint)) {
            roomList[idx] = { ...roomList[idx], users: [...(roomList[idx].users ?? []), user] };
        }
        const room = roomList.find(r => r.room_id === roomId);
        if (room) openRoomMenu(room);
    } else alert('Fehler beim Hinzufügen');
}

async function removeMember(roomId, fingerprint) {
    const res = await authFetch('DELETE', `/api/rooms/${roomId}/users/${fingerprint}`);
    if (res.ok) {
        await fetchRooms();
        const idx = roomList.findIndex(r => r.room_id === roomId);
        if (idx >= 0) {
            roomList[idx] = { ...roomList[idx], users: (roomList[idx].users ?? []).filter(u => u.fingerprint !== fingerprint) };
        }
        const room = roomList.find(r => r.room_id === roomId);
        if (room) openRoomMenu(room); else closeMemberModal();
    } else alert('Fehler beim Entfernen');
}

async function deleteGroup(roomId) {
    const myFP = sessionStorage.getItem('my_fingerprint');
    if (!confirm('Gruppe wirklich löschen?')) return;
    const res = await authFetch('DELETE', `/api/rooms/${roomId}/users/${myFP}`);
    if (res.ok) {
        closeMemberModal();
        currentRoomId = null;
        currentRoomEpoch = 0;
        const main = document.getElementById('main');
        main.style.display = 'none';
        document.querySelector('.maininfo').style.display = '';
        await fetchRooms();
    } else alert('Fehler beim Löschen der Gruppe');
}

async function leaveRoom(roomId) {
    const myFP = sessionStorage.getItem('my_fingerprint');
    const isEphemeral = Number(sessionStorage.getItem('my_mode') ?? 1) === 0;
    if (!confirm('Chat wirklich verlassen?')) return;
    const res = await authFetch('DELETE', `/api/rooms/${roomId}/users/${myFP}`);
    // Temp users are not in the DB room_members table → API returns 404.
    // Treat that as a successful leave and clean up locally.
    if (res.ok || (isEphemeral && res.status === 404)) {
        currentRoomId = null;
        currentRoomEpoch = 0;
        const main = document.getElementById('main');
        main.style.display = 'none';
        document.querySelector('.maininfo').style.display = '';
        await fetchRooms();
    } else {
        alert('Fehler beim Verlassen des Chats');
    }
}
async function sendMessage(roomID) {
    const plaintext = document.getElementById('schreibnachricht').value.trim();
    if (!plaintext) return;

    if (!socket || socket.readyState !== WebSocket.OPEN) {
        console.error("Socket ist nicht bereit. Verbindung prüfen!");
        return;
    }

    const roomKey = roomKeys.get(`${roomID}:${currentRoomEpoch}`);
    if (!roomKey) {
        console.warn('Kein Room-Key verfügbar – warte auf Key-Rotation.');
        return;
    }

    document.getElementById('schreibnachricht').value = "";

    const encrypted = AppCrypto.encryptMessage(plaintext, roomKey);
    const ciphertext = JSON.stringify(encrypted); // { nonce, ciphertext } als String

    // V-02: sign this message so peers (and the server) can verify that the
    // sender_fp was not spoofed. Canonical and formatting must match exactly
    // what utils.MessageCanonical produces on the backend.
    const myFP = sessionStorage.getItem('my_fingerprint');
    const timestamp = String(Math.floor(Date.now() / 1000));
    const nonce = crypto.randomUUID();
    const ctHashHex = await hashBody(ciphertext);
    const canonical =
        `MSG_V1\n${roomID}\n${currentRoomEpoch}\n${myFP}\n${timestamp}\n${nonce}\n${ctHashHex}`;
    const signature = AppCrypto.sign(canonical);

    pendingSent.add(`${roomID}:${ciphertext}`);
    socket.send(JSON.stringify({
        type: "message",
        payload: {
            room_id: roomID,
            epoch: currentRoomEpoch,
            ciphertext,
            sender_fp: myFP,
            timestamp,
            nonce,
            signature,
        }
    }));
    send(plaintext); // eigene Nachricht lokal als Klartext anzeigen

    // Cache so re-opening the room doesn't wipe ephemeral messages.
    if (!roomMessageCache.has(roomID)) roomMessageCache.set(roomID, []);
    roomMessageCache.get(roomID).push({
        isMine: true,
        senderName: null,
        senderMode: Number(sessionStorage.getItem('my_mode') ?? 1),
        text: plaintext,
        time: Math.floor(Date.now() / 1000)
    });
}

// verifyIncomingMessageSignature rebuilds the canonical an authenticated sender
// would have signed and validates it against the cached Ed25519 pk for their
// fingerprint. Returns true on success. Drops silently with a console.warn on
// any failure so that a forged or tampered message is never rendered.
async function verifyIncomingMessageSignature(payload) {
    if (!payload || !payload.sender_fp || !payload.signature
        || !payload.timestamp || !payload.nonce || !payload.ciphertext) {
        console.warn('message rejected: missing signature fields',
            { sender_fp: payload?.sender_fp, room_id: payload?.room_id });
        return false;
    }
    const pk = memberEd25519Keys.get(payload.sender_fp);
    if (!pk) {
        console.warn('message rejected: no Ed25519 pk cached for sender',
            { sender_fp: payload.sender_fp, room_id: payload.room_id });
        // Refresh member list in the background so the next message works.
        try { fetchRooms(); } catch (_) {}
        return false;
    }
    const ctHashHex = await hashBody(payload.ciphertext);
    const canonical =
        `MSG_V1\n${payload.room_id}\n${payload.epoch}\n${payload.sender_fp}\n${payload.timestamp}\n${payload.nonce}\n${ctHashHex}`;
    const ok = AppCrypto.verify(canonical, payload.signature, pk);
    if (!ok) {
        console.warn('message rejected: signature mismatch',
            { sender_fp: payload.sender_fp, room_id: payload.room_id, nonce: payload.nonce });
    }
    return ok;
}

function send(ciphertext) {
    if (ciphertext === "") return;
    const chatEl = document.getElementById('chat');
    if (!chatEl) return;
    chatEl.querySelector('.chat-empty')?.remove();
    const msg = document.createElement('div');
    msg.className = 'bubble sent';
    msg.dataset.senderFp = sessionStorage.getItem('my_fingerprint');
    msg.innerText = ciphertext;
    const timeEl = document.createElement('span');
    timeEl.className = 'msg-time';
    timeEl.textContent = formatTime(Math.floor(Date.now() / 1000));
    msg.appendChild(timeEl);
    chatEl.appendChild(msg);
    chatEl.scrollTop = chatEl.scrollHeight;
}
async function whoAmI() {
    const username = sessionStorage.getItem('username');
    const sessionId = sessionStorage.getItem('sessionId');

    if (!username || !sessionId) {
        window.location.href = 'Login';
        return null;
    }

    // Server-Session prüfen und Fingerprint laden
    try {
        const res = await authFetch('GET', '/api/users/me');
        if (res.status === 401 || res.status === 403) {
            // Session abgelaufen oder ungültig → neu einloggen
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

    // Hide "delete all messages" for ephemeral users — their messages are never persisted.
    if (Number(sessionStorage.getItem('my_mode') ?? 1) === 0) {
        const btn = document.querySelector('.user-popup-btn--danger');
        if (btn) btn.style.display = 'none';
    }

    return username;
}
async function submitNewChat() {
    const inputField = document.getElementById('groupNameInput');
    const groupName = inputField.value.trim();

    if (!groupName) {
        alert("⚠️ Bitte gib einen Gruppennamen ein!");
        return;
    }
    if (groupName.length > 100) {
        alert("⚠️ Der Gruppenname darf maximal 100 Zeichen lang sein!");
        return;
    }

    // Ruft deine API-Funktion auf
    await newChat(groupName);

    // Aufräumen
    inputField.value = "";
    document.getElementById('newChatModal').close();
}
async function hashBody(bodyString) {
    const msgBuffer = new TextEncoder().encode(bodyString);
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    // Wandelt den Buffer in einen Hex-String um
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
}

// cacheMemberEd25519Keys stores Ed25519 public keys from any member list so
// incoming per-message signatures (V-02) can be verified. Tolerates the three
// key shapes the server sends: RoomMemberInfo ({fingerprint, ed25519_public_key}),
// MemberWithKey ({fingerprint, ed25519_public_key}), and legacy entries missing
// the field. Missing keys are ignored so an older/partial response does not
// clobber a good cached value.
function cacheMemberEd25519Keys(members) {
    if (!Array.isArray(members)) return;
    for (const m of members) {
        if (!m || !m.fingerprint || !m.ed25519_public_key) continue;
        memberEd25519Keys.set(m.fingerprint, m.ed25519_public_key);
    }
}
async function newChat(groupName) {
    const sessionId = sessionStorage.getItem('sessionId');
    // Zeitstempel einmal festlegen und als String speichern
    const timestamp = Math.floor(Date.now() / 1000).toString();
    const path = '/api/rooms';

    // WICHTIG: Sicherstellen, dass die Keys im JSON exakt so heißen, wie das Go-Struct sie erwartet (z.B. "GroupName" statt "groupName"?)
    const bodyObj = { groupName: groupName };
    const bodyString = JSON.stringify(bodyObj);

    try {
        // 1. Body Hash erzeugen
        const bodyHash = await hashBody(bodyString);

        const nonce = crypto.randomUUID();
        const canonical = `POST\n${path}\n${timestamp}\n${bodyHash}\n${nonce}`;
        const signature = AppCrypto.sign(canonical);

        const response = await fetch(`${path}`, {
            method: 'POST',
            headers: {
                'Chat-Session-ID': sessionId,
                'Chat-Timestamp': timestamp,
                'Chat-Nonce': nonce,
                'Chat-Signature': signature,
                'Content-Type': 'application/json',
                'Accept': 'application/json'
            },
            body: bodyString
        });

        if (!response.ok) {
            // Versuche die Fehlermeldung vom Server zu lesen
            const errorText = await response.text();
            console.error("Server Antwort (Error):", errorText);
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        const data = await response.json();
        fetchRooms();

    } catch (err) {
        console.error("Fehler beim Request:", err);
        alert("❌ Fehler: " + err.message);
    }
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
    if (!confirm('Wirklich alle deine Nachrichten löschen?')) return;
    const res = await authFetch('DELETE', '/api/users/me/messages');
    if (res.ok) {
        document.getElementById('user-popup').classList.remove('open');
        // For ephemeral users, messages are never stored in the DB so the server
        // won't broadcast messages_purged (deleted count = 0). Remove own bubbles locally.
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
        alert('Fehler beim Löschen der Nachrichten');
    }
}

// ============================================================
// MOBILE SIDEBAR TOGGLE
// ============================================================

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

// ============================================================
// VOICE CHAT — WebRTC
// ============================================================

function sendSignal(roomId, targetFP, signalData) {
    if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({
            type: 'signal',
            payload: { room_id: roomId, target_fp: targetFP, signal: signalData }
        }));
    }
}

async function joinVoice(roomId) {
    if (voiceRoomId === roomId) return;
    if (voiceRoomId) leaveVoice();

    try {
        localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
    } catch (e) {
        alert('Mikrofon nicht verfügbar: ' + e.message);
        return;
    }

    voiceRoomId = roomId;
    const myFP = sessionStorage.getItem('my_fingerprint');
    voiceParticipants.clear();
    voiceParticipants.add(myFP);

    if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'voice_joined', payload: { room_id: roomId } }));
    }

    updateVoiceUI();
}

function leaveVoice() {
    if (!voiceRoomId) return;
    const roomId = voiceRoomId;

    peerConnections.forEach((_, fp) => pendingIceCandidates.delete(fp));
    peerStates.forEach((s) => { if (s.iceRestartTimer) clearTimeout(s.iceRestartTimer); });
    peerConnections.forEach((pc) => pc.close());
    peerConnections.clear();
    peerStates.clear();

    if (localStream) {
        localStream.getTracks().forEach(t => t.stop());
        localStream = null;
    }

    // Remove all audio elements
    document.querySelectorAll('audio[data-voice]').forEach(a => a.remove());

    voiceParticipants.clear();
    mutedUsers.clear();
    voiceRoomId = null;
    isMuted = false;

    if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'voice_left', payload: { room_id: roomId } }));
    }

    updateVoiceUI();
}

function toggleVoice(roomId) {
    if (voiceRoomId === roomId) leaveVoice(); else joinVoice(roomId);
}

function toggleMute() {
    if (!localStream) return;
    isMuted = !isMuted;
    localStream.getAudioTracks().forEach(t => { t.enabled = !isMuted; });
    const myFP = sessionStorage.getItem('my_fingerprint');
    if (isMuted) mutedUsers.add(myFP); else mutedUsers.delete(myFP);
    // Broadcast mute state to all peers via the existing signal channel (no new Go event needed)
    const muteSignal = JSON.stringify({ type: 'mute_state', muted: isMuted });
    peerConnections.forEach((_, targetFP) => sendSignal(voiceRoomId, targetFP, muteSignal));
    updateVoiceUI();
}

async function flushPendingIceCandidates(fp) {
    const candidates = pendingIceCandidates.get(fp);
    if (!candidates) return;
    pendingIceCandidates.delete(fp);
    const pc = peerConnections.get(fp);
    if (!pc || !pc.remoteDescription) {
        // No PC or description yet — put them back.
        if (candidates.length) pendingIceCandidates.set(fp, candidates);
        return;
    }
    const state = peerStates.get(fp);
    for (const candidate of candidates) {
        try { await pc.addIceCandidate(candidate); }
        catch (e) { if (!state?.ignoreOffer) console.warn('ICE error (flush):', e); }
    }
}

function ensurePeerConnection(remoteFP, roomId) {
    const existing = peerConnections.get(remoteFP);
    if (existing) return existing;

    const pc = new RTCPeerConnection(ICE_CONFIG);
    peerConnections.set(remoteFP, pc);

    const myFP = sessionStorage.getItem('my_fingerprint') || '';
    // Perfect-negotiation "polite" rule: deterministic by fingerprint compare.
    // The peer with the lexicographically larger fingerprint is polite (rolls back on glare).
    const polite = myFP > remoteFP;
    const state = { makingOffer: false, ignoreOffer: false, polite, roomId, iceRestartTimer: null };
    peerStates.set(remoteFP, state);

    if (localStream) {
        localStream.getTracks().forEach(t => pc.addTrack(t, localStream));
    }

    pc.ontrack = (event) => {
        let audio = document.querySelector(`audio[data-voice="${remoteFP}"]`);
        if (!audio) {
            audio = document.createElement('audio');
            audio.dataset.voice = remoteFP;
            audio.autoplay = true;
            document.body.appendChild(audio);
        }
        audio.srcObject = event.streams[0];
        audio.play().catch(e => console.warn('Audio autoplay blocked:', e));
    };

    pc.onicecandidate = (event) => {
        if (event.candidate) {
            sendSignal(roomId, remoteFP, { type: 'ice', candidate: event.candidate });
        }
    };

    pc.onnegotiationneeded = async () => {
        try {
            state.makingOffer = true;
            await pc.setLocalDescription();
            if (pc.localDescription) {
                sendSignal(roomId, remoteFP, { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
            }
        } catch (e) {
            console.error('negotiationneeded error', e);
        } finally {
            state.makingOffer = false;
        }
    };

    pc.oniceconnectionstatechange = () => {
        if (pc.iceConnectionState === 'failed') {
            try { pc.restartIce(); } catch (e) { console.warn('restartIce failed', e); }
        } else if (pc.iceConnectionState === 'disconnected') {
            if (state.iceRestartTimer) clearTimeout(state.iceRestartTimer);
            state.iceRestartTimer = setTimeout(() => {
                if (pc.iceConnectionState === 'disconnected') {
                    try { pc.restartIce(); } catch (e) { console.warn('restartIce failed', e); }
                }
            }, 5000);
        } else if (state.iceRestartTimer) {
            clearTimeout(state.iceRestartTimer);
            state.iceRestartTimer = null;
        }
    };

    pc.onconnectionstatechange = () => {
        if (['failed', 'closed'].includes(pc.connectionState)) {
            const audio = document.querySelector(`audio[data-voice="${remoteFP}"]`);
            if (audio) audio.remove();
            if (peerConnections.get(remoteFP) === pc) {
                peerConnections.delete(remoteFP);
                peerStates.delete(remoteFP);
                pendingIceCandidates.delete(remoteFP);
            }
            voiceParticipants.delete(remoteFP);
            mutedUsers.delete(remoteFP);
            if (remoteFP !== myFP) updateVoiceUI();
        }
    };

    // Drain any ICE candidates that arrived before PC existed (remoteDescription still null — they'll be re-queued).
    flushPendingIceCandidates(remoteFP);
    return pc;
}

async function handleSignalMessage(payload) {
    // payload: { room_id, from_fp, signal }
    if (!voiceRoomId || payload.room_id !== voiceRoomId) return;

    const fromFP = payload.from_fp;
    const sig = payload.signal;

    try {
        if (sig.type === 'offer' || sig.type === 'answer') {
            const pc = ensurePeerConnection(fromFP, voiceRoomId);
            const state = peerStates.get(fromFP);
            const description = { type: sig.type, sdp: sig.sdp };
            const offerCollision = sig.type === 'offer' && (state.makingOffer || pc.signalingState !== 'stable');
            state.ignoreOffer = !state.polite && offerCollision;
            if (state.ignoreOffer) return;
            if (offerCollision) {
                // Polite peer: rollback local offer, then accept remote.
                await Promise.all([
                    pc.setLocalDescription({ type: 'rollback' }).catch(() => {}),
                    pc.setRemoteDescription(description),
                ]);
            } else {
                await pc.setRemoteDescription(description);
            }
            if (sig.type === 'offer') {
                await pc.setLocalDescription();
                if (pc.localDescription) {
                    sendSignal(voiceRoomId, fromFP, { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
                }
            }
            await flushPendingIceCandidates(fromFP);
        } else if (sig.type === 'ice') {
            const pc = peerConnections.get(fromFP);
            if (!pc || !pc.remoteDescription) {
                if (!pendingIceCandidates.has(fromFP)) pendingIceCandidates.set(fromFP, []);
                pendingIceCandidates.get(fromFP).push(sig.candidate);
                return;
            }
            try { await pc.addIceCandidate(sig.candidate); }
            catch (e) {
                const state = peerStates.get(fromFP);
                if (!state?.ignoreOffer) console.warn('ICE error:', e);
            }
        } else if (sig.type === 'mute_state') {
            if (sig.muted) mutedUsers.add(fromFP); else mutedUsers.delete(fromFP);
            if (payload.room_id === currentRoomId) updateVoiceUI();
        }
    } catch (e) {
        console.error('WebRTC signal error:', e);
    }
}

async function handleVoiceJoined(payload) {
    // payload: { room_id, fingerprint, voice_users }
    const myFP = sessionStorage.getItem('my_fingerprint');

    if (payload.voice_users) {
        // Own join confirmation — backend sends us the list of already-connected participants.
        // Add them and initiate a peer connection to each one.
        payload.voice_users.forEach(fp => voiceParticipants.add(fp));
        if (voiceRoomId === payload.room_id) {
            for (const fp of payload.voice_users) {
                await createPeerConnection(fp, voiceRoomId, true);
            }
        }
    } else {
        // Someone else joined — add them and initiate connection if we're already in voice.
        voiceParticipants.add(payload.fingerprint);

    }

    // If we're in voice in this room, ensure a peer connection with every relevant peer.
    // Both sides call ensurePeerConnection; addTrack triggers negotiationneeded and the
    // polite/impolite rule resolves any offer glare.
    if (voiceRoomId === payload.room_id) {
        if (payload.voice_users) {
            // We just joined — open a PC to every existing voice participant.
            payload.voice_users.forEach(fp => {
                if (fp !== myFP) ensurePeerConnection(fp, voiceRoomId);
            });
        } else if (payload.fingerprint !== myFP) {
            ensurePeerConnection(payload.fingerprint, voiceRoomId);
        }
    }

    if (payload.room_id === currentRoomId) updateVoiceUI();
}

function handleVoiceLeft(payload) {
    // payload: { room_id, fingerprint }
    voiceParticipants.delete(payload.fingerprint);
    mutedUsers.delete(payload.fingerprint);

    pendingIceCandidates.delete(payload.fingerprint);
    const state = peerStates.get(payload.fingerprint);
    if (state?.iceRestartTimer) clearTimeout(state.iceRestartTimer);
    peerStates.delete(payload.fingerprint);
    const pc = peerConnections.get(payload.fingerprint);
    if (pc) {
        pc.close();
        peerConnections.delete(payload.fingerprint);
    }

    const audio = document.querySelector(`audio[data-voice="${payload.fingerprint}"]`);
    if (audio) audio.remove();

    if (payload.room_id === currentRoomId) updateVoiceUI();
}

function updateVoiceUI() {
    const isInVoice = voiceRoomId === currentRoomId;

    const joinBtn = document.getElementById('voice-join-btn');
    if (joinBtn) {
        joinBtn.textContent = isInVoice ? '🔴 Verlassen' : '🎙️ Voice';
        joinBtn.className = isInVoice ? 'voice-btn active' : 'voice-btn';
    }

    const muteBtn = document.getElementById('voice-mute-btn');
    if (muteBtn) {
        muteBtn.style.display = isInVoice ? '' : 'none';
        muteBtn.textContent = isMuted ? '🔇 Stumm' : '🎤 Stumm';
        muteBtn.className = isMuted ? 'voice-btn muted' : 'voice-btn';
    }

    const list = document.getElementById('voice-participants-list');
    if (!list) return;
    list.innerHTML = '';

    const myFP = sessionStorage.getItem('my_fingerprint');
    const room = roomList.find(r => r.room_id === currentRoomId);
    const fpToName = {};
    if (room) {
        if (room.host) fpToName[room.host.fingerprint] = room.host.username;
        (room.users ?? []).forEach(u => { fpToName[u.fingerprint] = u.username; });
    }

    // Mini-avatars in the header — always clear first, then repopulate
    const avatarRow = document.getElementById('voice-avatars');
    if (avatarRow) {
        avatarRow.innerHTML = '';
        voiceParticipants.forEach(fp => {
            // Never show own icon when not in voice
            if (!isInVoice && fp === myFP) return;
            const name = fpToName[fp] || fp.slice(0, 8) + '…';
            const initial = name.charAt(0).toUpperCase();
            const isMutedFP = mutedUsers.has(fp);
            const wrapper = document.createElement('span');
            wrapper.className = 'voice-avatar-wrapper';
            wrapper.title = name + (isMutedFP ? ' (stumm)' : '');
            const av = document.createElement('span');
            av.className = 'voice-avatar' + (isMutedFP ? ' voice-avatar-muted' : '');
            av.dataset.fp = fp;
            av.textContent = initial;
            wrapper.appendChild(av);
            if (isMutedFP) {
                const icon = document.createElement('span');
                icon.className = 'voice-mute-icon';
                icon.textContent = '🔇';
                wrapper.appendChild(icon);
            }
            avatarRow.appendChild(wrapper);
        });
    }

    voiceParticipants.forEach(fp => {
        const div = document.createElement('div');
        div.className = 'voice-participant';
        const name = fpToName[fp] || fp.slice(0, 8) + '…';
        const isMe = fp === myFP;
        const isMutedFP = mutedUsers.has(fp);
        const initial = name.charAt(0).toUpperCase();
        const av = document.createElement('span');
        av.className = 'voice-avatar' + (isMutedFP ? ' voice-avatar-muted' : '');
        av.dataset.fp = fp;
        av.textContent = initial;
        div.appendChild(av);
        const label = document.createElement('span');
        label.textContent = `${name}${isMe ? ' (Du)' : ''}${isMutedFP ? ' 🔇' : ''}`;
        div.appendChild(label);
        list.appendChild(div);
    });

    if (voiceParticipants.size === 0) {
        const empty = document.createElement('div');
        empty.style.cssText = 'padding:10px 14px;font-size:0.75rem;color:rgba(255,255,255,0.3)';
        empty.textContent = 'Niemand im Voice';
        list.appendChild(empty);
    }
}

window.addEventListener('DOMContentLoaded', async () => {
    Settings.apply(Settings.load());
    await whoAmI();
    loadIceConfig();
    connectLocalChat();
    fetchRooms();
});

// Beim Wechsel auf Desktop: Sidebar-Zustand zurücksetzen
window.addEventListener('resize', () => {
    if (window.innerWidth > 768) {
        document.querySelector('nav').classList.remove('open');
        document.getElementById('nav-overlay').classList.remove('active');
        document.body.style.overflow = '';
    }
});

// ============================================================
// NAV PANELS — Search / Notifications / Settings
// ============================================================

// Speichert den Original-Parent jedes Panels für Rückmigration
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

    // Alle Panels schließen und ggf. zurückbewegen
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

// Klick außerhalb schließt offene Panels
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

    // Build color swatches once
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
        // Custom color picker
        const customWrap = document.createElement('div');
        customWrap.className = 'settings-swatch-custom';
        customWrap.title = 'Eigene Farbe';
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

    // Account info
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

    resultsEl.innerHTML = '<div class="nav-panel-empty search-loading"><i class="fas fa-circle-notch fa-spin"></i> Suche…</div>';
    _searchTimer = setTimeout(() => _runMessageSearch(query, resultsEl), 350);
}

async function _runMessageSearch(query, resultsEl) {
    const q = query.toLowerCase();
    const results = []; // { room, msgId, excerpt, roomLabel }

    // Räume nach Namen filtern (sofort)
    roomList.forEach(room => {
        if (room.name?.toLowerCase().includes(q)) {
            results.push({ type: 'room', room });
        }
    });

    // Nachrichten in allen Räumen durchsuchen (parallel)
    await Promise.all(roomList.map(async room => {
        try {
            // Key Slots sicherstellen
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
        resultsEl.innerHTML = '<div class="nav-panel-empty">Keine Ergebnisse</div>';
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

    // Badge am Raum-Item in der Sidebar
    const badge = document.querySelector(`.chat-item[data-room-id="${roomId}"] .unread-badge`);
    if (badge) {
        badge.textContent = count > 99 ? '99+' : count;
        badge.style.display = count > 0 ? 'flex' : 'none';
    }

    // Badge am Bell-Icon (Gesamtanzahl)
    const total = [...unreadCounts.values()].reduce((a, b) => a + b, 0);
    const bellBadge = document.getElementById('bell-badge');
    if (bellBadge) {
        bellBadge.textContent = total > 99 ? '99+' : total;
        bellBadge.style.display = total > 0 ? 'flex' : 'none';
    }

    // Notification-Panel aktualisieren wenn offen
    if (document.getElementById('notif-panel')?.classList.contains('open')) {
        populateNotifPanel();
    }
}

function populateNotifPanel() {
    const list = document.getElementById('notif-list');
    if (!list) return;

    const rooms = roomList.filter(r => (unreadCounts.get(r.room_id) ?? 0) > 0);
    if (rooms.length === 0) {
        list.innerHTML = '<div class="nav-panel-empty">Keine neuen Benachrichtigungen</div>';
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