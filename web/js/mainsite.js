
let socket;
let roomList = [];
let gefilterteListe = [];
let currentRoomId = null;
let currentRoomEpoch = 0;
const pendingSent = new Set(); // eigene gesendete Nachrichten (roomId:ciphertext) — verhindert Echo-Duplikate
const roomKeys = new Map();   // `${roomId}:${epoch}` → Uint8Array (Room-Key)

// ============================================================
// VOICE CHAT STATE
// ============================================================
let voiceRoomId = null;
let localStream = null;
const peerConnections = new Map(); // FP → RTCPeerConnection
let isMuted = false;
const voiceParticipants = new Set(); // FP von Usern aktuell in Voice

const ICE_CONFIG = {
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
};

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
        const dhKP      = nacl.box.keyPair();    // X25519

        sessionStorage.setItem('ed25519_secret', bufToB64(signingKP.secretKey));
        sessionStorage.setItem('ed25519_public', bufToB64(signingKP.publicKey));
        sessionStorage.setItem('x25519_secret',  bufToB64(dhKP.secretKey));
        sessionStorage.setItem('x25519_public',  bufToB64(dhKP.publicKey));

        return {
            signingPublicKey: bufToB64(signingKP.publicKey),
            dhPublicKey:      bufToB64(dhKP.publicKey)
        };
    }

    function loadIdentity() {
        const edSec = sessionStorage.getItem('ed25519_secret');
        const xSec  = sessionStorage.getItem('x25519_secret');
        if (!edSec || !xSec) return null;
        return {
            signingSecretKey: b64ToBuf(edSec),
            dhSecretKey:      b64ToBuf(xSec)
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
        const nonce     = nacl.randomBytes(nacl.box.nonceLength);
        const key       = roomKey instanceof Uint8Array ? roomKey : new Uint8Array(roomKey);
        const encrypted = nacl.box(key, nonce, b64ToBuf(recipientDHPublicB64), ephemeral.secretKey);
        return {
            ephemeralPub: bufToB64(ephemeral.publicKey),
            nonce:        bufToB64(nonce),
            ciphertext:   bufToB64(encrypted)
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
        const key   = roomKey instanceof Uint8Array ? roomKey : new Uint8Array(roomKey);
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

function connectLocalChat() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(`${wsProtocol}//${window.location.host}/api/ws`);

    socket.onopen = () => {
        const authPayload = {
            "type": "auth",
            "payload": {
                "Chat-Session-ID": sessionStorage.getItem('sessionId'),
                "Chat-Timestamp": Math.floor(Date.now() / 1000),
                "Chat-Signature": "test"
            }
        };
        socket.send(JSON.stringify(authPayload));
    };

    socket.onmessage = (event) => {
        const envelope = JSON.parse(event.data);

        if (envelope.type === "join_room" || envelope.type === "leave_room") {
            fetchRooms();
            return;
        }

        if (envelope.type === "room_key_rotation") {
            const p = envelope.payload;
            if (p.room_id === currentRoomId) currentRoomEpoch = p.epoch;
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

        if (envelope.type === "message") {
            const payload = envelope.payload;

            // eigene Echo-Nachricht ignorieren
            const myFP    = sessionStorage.getItem('my_fingerprint');
            const sentKey = `${payload.room_id}:${payload.ciphertext}`;
            if (envelope.sender_fp === myFP || pendingSent.has(sentKey)) {
                pendingSent.delete(sentKey);
                return;
            }

            // nur anzeigen wenn dieser Raum gerade offen ist
            if (payload.room_id !== currentRoomId) return;
            const text = decryptText(payload.ciphertext, payload.room_id, payload.epoch);
            if (!text) return; // leer oder nicht entschlüsselbar → nicht anzeigen
            const chatEl = document.getElementById('chat');
            if (!chatEl) return;
            const wrapper = document.createElement('div');
            wrapper.className = 'msg-wrapper';
            if (payload.sender_name) {
                const label = document.createElement('div');
                label.className = 'msg-label';
                label.textContent = payload.sender_name;
                wrapper.appendChild(label);
            }
            const div = document.createElement('div');
            div.className = 'bubble received';
            div.innerText = text;
            wrapper.appendChild(div);
            chatEl.appendChild(wrapper);
            chatEl.scrollTop = chatEl.scrollHeight;
        }
    };

    socket.onerror = (err) => console.error("WebSocket Fehler:", err);
    socket.onclose = ()  => console.warn("WebSocket geschlossen");
}
async function fetchRooms() {

    const timestamp = Math.floor(Date.now() / 1000);
    const path = '/api/users/me/rooms';

    const emptyBodyHash = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
    const canonical = `GET\n${path}\n${timestamp}\n${emptyBodyHash}`;
    const signature = AppCrypto.sign(canonical);

    try {
        const response = await fetch(`/api/users/me/rooms`, {
            method: 'GET',
            headers: {
                'Chat-Session-ID': sessionStorage.getItem('sessionId'),
                'Chat-Timestamp':  timestamp,
                'Chat-Signature':  signature
            }
        });

        if (!response.ok) {
            throw new Error(`HTTP-Fehler: ${response.status}`);
        }

        roomList = (await response.json()).rooms ?? [];
        renderFunc(roomList);
        const cr = roomList.find(r => r.room_id === currentRoomId);
        if (cr) renderChatUserList(cr.users);
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
    }
    panel.classList.toggle('open');
}

function renderChatUserList(users) {
    const list = document.getElementById('chat-user-list');
    if (!list) return;
    list.innerHTML = '';
    (users ?? []).forEach(u => {
        const div = document.createElement('div');
        div.className = 'chat-user-entry';
        div.innerHTML = `<span class="chat-user-dot"></span><span>${u.username}</span>`;
        list.appendChild(div);
    });
}

function clickroom(room) {
    currentRoomId = room.room_id;
    currentRoomEpoch = room.epoch ?? 0;
    // Auf Mobile: Sidebar schließen wenn ein Raum geöffnet wird
    if (window.innerWidth <= 768) closeSidebar();

    const maininfo = document.querySelector('.maininfo');
    maininfo.style.display = 'none';
    const main = document.getElementById('main');
    main.style.display = 'flex';
    main.innerHTML = `
                <div class="chat-header">
                    <span class="chat-header-name">${room.name}</span>
                    <div class="chat-header-actions">
                        <div class="voice-controls">
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
                    <input type="text" id="schreibnachricht" placeholder="Nachricht schreiben...">
                    <button onclick="sendMessage('${room.room_id}')">Senden</button>
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
    const myFP   = sessionStorage.getItem('my_fingerprint');

    const fpToName = {};
    if (room.host) fpToName[room.host.fingerprint] = room.host.username;
    (room.users ?? []).forEach(u => { fpToName[u.fingerprint] = u.username; });

    try {
        const response = await authFetch('GET', `/api/rooms/${room.room_id}/messages`);
        if (!response.ok) throw new Error(`HTTP-Fehler: ${response.status}`);

        const data     = await response.json();
        const messages = (data.messages ?? []).filter(m => m.ciphertext && m.ciphertext.trim() !== '').reverse();

        chatEl.innerHTML = '';

        if (messages.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'chat-empty';
            empty.textContent = 'Noch keine Nachrichten.';
            chatEl.appendChild(empty);
            return;
        }

        messages.forEach(msg => {
            const isMine = msg.sender_fp === myFP;
            const text = decryptText(msg.ciphertext, room.room_id, msg.epoch);
            if (!text) return; // leer oder nicht entschlüsselbar → überspringen
            if (isMine) {
                const div = document.createElement('div');
                div.className = 'bubble sent';
                div.innerText = text;
                chatEl.appendChild(div);
            } else {
                const wrapper = document.createElement('div');
                wrapper.className = 'msg-wrapper';
                const label = document.createElement('div');
                label.className = 'msg-label';
                label.textContent = fpToName[msg.sender_fp] ?? (msg.sender_fp.slice(0, 8) + '…');
                wrapper.appendChild(label);
                const div = document.createElement('div');
                div.className = 'bubble received';
                div.innerText = text;
                wrapper.appendChild(div);
                chatEl.appendChild(wrapper);
            }
        });

        chatEl.scrollTop = chatEl.scrollHeight;
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

            div.innerHTML = `
                <div class="chat-item-row">
                    <div style="flex:1;cursor:pointer;" class="room-info">
                        <div style="font-weight:bold;">👤${room.name}</div>
                    </div>
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

    function makeRow(username, btnClass, btnText, onClick) {
        const row = document.createElement('div');
        row.className = 'member-row';
        const name = document.createElement('span');
        name.textContent = username;
        const btn = document.createElement('button');
        btn.className = btnClass;
        btn.textContent = btnText;
        btn.onclick = onClick;
        row.appendChild(name);
        row.appendChild(btn);
        return row;
    }

    // Placeholder während Laden
    leftList.innerHTML  = '<div class="member-col-empty">Lädt...</div>';
    rightList.innerHTML = '<div class="member-col-empty">Lädt...</div>';

    const myFP    = sessionStorage.getItem('my_fingerprint');
    const allResp = await authFetch('GET', '/api/users').catch(() => null);
    const allUsers = allResp?.ok ? (await allResp.json()) : [];
    const onlineUsers = Array.isArray(allUsers) ? allUsers : [];

    const memberFPs  = new Set((room.users ?? []).map(u => u.fingerprint));
    const removable  = (room.users ?? []).filter(u => u.fingerprint !== myFP);
    const addable    = onlineUsers.filter(u => !memberFPs.has(u.fingerprint) && u.fingerprint !== myFP);

    // Mitglieder-Liste befüllen
    leftList.innerHTML = '';
    if (removable.length === 0) {
        leftList.innerHTML = '<div class="member-col-empty">Keine weiteren Mitglieder</div>';
    } else {
        removable.forEach(u => leftList.appendChild(
            makeRow(u.username, 'rem-btn', '– Entfernen', () => removeMember(room.room_id, u.fingerprint))
        ));
    }

    // Hinzufügen-Liste befüllen
    rightList.innerHTML = '';
    if (addable.length === 0) {
        rightList.innerHTML = '<div class="member-col-empty">Keine online User verfügbar</div>';
    } else {
        addable.forEach(u => rightList.appendChild(
            makeRow(u.username, 'add-btn', '+ Hinzufügen', () => addMember(room.room_id, u))
        ));
    }
}

async function authFetch(method, path, body = null) {
    const bodyString = body ? JSON.stringify(body) : null;
    const timestamp  = Math.floor(Date.now() / 1000);
    const bodyHash   = bodyString
        ? await hashBody(bodyString)
        : 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
    const canonical = `${method}\n${path}\n${timestamp}\n${bodyHash}`;
    const signature = AppCrypto.sign(canonical);

    const opts = {
        method,
        headers: {
            'Chat-Session-ID': sessionStorage.getItem('sessionId'),
            'Chat-Timestamp':  timestamp,
            'Chat-Signature':  signature,
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
function sendMessage(roomID) {
    const plaintext = document.getElementById('schreibnachricht').value.trim();
    document.getElementById('schreibnachricht').value = "";
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

    const encrypted = AppCrypto.encryptMessage(plaintext, roomKey);
    const ciphertext = JSON.stringify(encrypted); // { nonce, ciphertext } als String

    pendingSent.add(`${roomID}:${ciphertext}`);
    socket.send(JSON.stringify({
        type: "message",
        payload: {
            room_id: roomID,
            epoch: currentRoomEpoch,
            ciphertext,
            sender_name: sessionStorage.getItem('username')
        }
    }));
    send(plaintext); // eigene Nachricht lokal als Klartext anzeigen
}

function send(ciphertext) {
    if (ciphertext === "") return;
    const chatEl = document.getElementById('chat');
    const msg = document.createElement('div');
    msg.className = 'bubble sent';
    msg.innerText = ciphertext;
    chatEl.appendChild(msg);
    chatEl.scrollTop = chatEl.scrollHeight;
}
async function whoAmI() {
    const username  = sessionStorage.getItem('username');
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
        }
    } catch (e) {
        console.warn('whoAmI: Server nicht erreichbar', e);
    }

    const usernamefield = document.getElementById('username');
    usernamefield.textContent = username;
    const usernameshortfield = document.getElementById('usernameshort');
    usernameshortfield.textContent = username.substring(0, 2).toUpperCase();

    return username;
}
async function submitNewChat() {
    const inputField = document.getElementById('groupNameInput');
    const groupName = inputField.value.trim();

    if (!groupName) {
        alert("⚠️ Bitte gib einen Gruppennamen ein!");
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

        // 2. Canonical String bauen
        // PRÜFE: Erwartet dein Go-Server hier wirklich POST (großgeschrieben)?
        const canonical = `POST\n${path}\n${timestamp}\n${bodyHash}`;

        // 3. Signatur erzeugen
        const signature = AppCrypto.sign(canonical);

       

        const response = await fetch(`${path}`, {
            method: 'POST',
            headers: {
                'Chat-Session-ID': sessionId,
                'Chat-Timestamp': timestamp, // Hier wird der String gesendet
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

    peerConnections.forEach((pc) => pc.close());
    peerConnections.clear();

    if (localStream) {
        localStream.getTracks().forEach(t => t.stop());
        localStream = null;
    }

    // Remove all audio elements
    document.querySelectorAll('audio[data-voice]').forEach(a => a.remove());

    voiceParticipants.clear();
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
    updateVoiceUI();
}

async function createPeerConnection(targetFP, roomId, isInitiator) {
    if (peerConnections.has(targetFP)) {
        peerConnections.get(targetFP).close();
    }

    const pc = new RTCPeerConnection(ICE_CONFIG);
    peerConnections.set(targetFP, pc);

    if (localStream) {
        localStream.getTracks().forEach(t => pc.addTrack(t, localStream));
    }

    pc.ontrack = (event) => {
        let audio = document.querySelector(`audio[data-voice="${targetFP}"]`);
        if (!audio) {
            audio = document.createElement('audio');
            audio.dataset.voice = targetFP;
            audio.autoplay = true;
            document.body.appendChild(audio);
        }
        audio.srcObject = event.streams[0];
    };

    pc.onicecandidate = (event) => {
        if (event.candidate) {
            sendSignal(roomId, targetFP, { type: 'ice', candidate: event.candidate });
        }
    };

    pc.onconnectionstatechange = () => {
        if (['disconnected', 'failed', 'closed'].includes(pc.connectionState)) {
            const audio = document.querySelector(`audio[data-voice="${targetFP}"]`);
            if (audio) audio.remove();
            if (peerConnections.get(targetFP) === pc) peerConnections.delete(targetFP);
        }
    };

    if (isInitiator) {
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        sendSignal(roomId, targetFP, { type: 'offer', sdp: offer.sdp });
    }

    return pc;
}

async function handleSignalMessage(payload) {
    // payload: { room_id, from_fp, signal }
    if (!voiceRoomId || payload.room_id !== voiceRoomId) return;

    const fromFP = payload.from_fp;
    const sig = payload.signal;

    try {
        if (sig.type === 'offer') {
            const pc = await createPeerConnection(fromFP, voiceRoomId, false);
            await pc.setRemoteDescription({ type: 'offer', sdp: sig.sdp });
            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);
            sendSignal(voiceRoomId, fromFP, { type: 'answer', sdp: answer.sdp });
        } else if (sig.type === 'answer') {
            const pc = peerConnections.get(fromFP);
            if (pc) await pc.setRemoteDescription({ type: 'answer', sdp: sig.sdp });
        } else if (sig.type === 'ice') {
            const pc = peerConnections.get(fromFP);
            if (pc && sig.candidate) await pc.addIceCandidate(sig.candidate);
        }
    } catch (e) {
        console.error('WebRTC signal error:', e);
    }
}

async function handleVoiceJoined(payload) {
    // payload: { room_id, fingerprint, voice_users }
    const myFP = sessionStorage.getItem('my_fingerprint');

    if (payload.voice_users) {
        payload.voice_users.forEach(fp => voiceParticipants.add(fp));
    } else {
        voiceParticipants.add(payload.fingerprint);
    }

    // If we're in voice in this room and someone else joined, initiate connection
    if (voiceRoomId === payload.room_id && payload.fingerprint !== myFP) {
        await createPeerConnection(payload.fingerprint, voiceRoomId, true);
    }

    if (payload.room_id === currentRoomId) updateVoiceUI();
}

function handleVoiceLeft(payload) {
    // payload: { room_id, fingerprint }
    voiceParticipants.delete(payload.fingerprint);

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

    if (voiceParticipants.size === 0) {
        const empty = document.createElement('div');
        empty.style.cssText = 'padding:10px 14px;font-size:0.75rem;color:rgba(255,255,255,0.3)';
        empty.textContent = 'Niemand im Voice';
        list.appendChild(empty);
        return;
    }

    const myFP = sessionStorage.getItem('my_fingerprint');
    const room = roomList.find(r => r.room_id === currentRoomId);
    const fpToName = {};
    if (room) {
        if (room.host) fpToName[room.host.fingerprint] = room.host.username;
        (room.users ?? []).forEach(u => { fpToName[u.fingerprint] = u.username; });
    }

    voiceParticipants.forEach(fp => {
        const div = document.createElement('div');
        div.className = 'voice-participant';
        const name = fpToName[fp] || fp.slice(0, 8) + '…';
        const isMe = fp === myFP;
        div.innerHTML = `<span class="voice-dot"></span><span>${name}${isMe ? ' (Du)' : ''}${isMe && isMuted ? ' 🔇' : ''}</span>`;
        list.appendChild(div);
    });
}

window.addEventListener('DOMContentLoaded', async () => {
    await whoAmI();
    connectLocalChat();
    fetchRooms();
    setInterval(fetchRooms, 15000);
});

// Beim Wechsel auf Desktop: Sidebar-Zustand zurücksetzen
window.addEventListener('resize', () => {
    if (window.innerWidth > 768) {
        document.querySelector('nav').classList.remove('open');
        document.getElementById('nav-overlay').classList.remove('active');
        document.body.style.overflow = '';
    }
});