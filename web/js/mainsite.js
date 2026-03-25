
let socket;
let roomList = [];
let gefilterteListe = [];
let currentRoomId = null;
const pendingSent = new Set(); // eigene gesendete Nachrichten (roomId:ciphertext) — verhindert Echo-Duplikate

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
            const chatEl = document.getElementById('chat');
            if (!chatEl) return;
            const div = document.createElement('div');
            div.className = 'bubble received';
            div.innerText = payload.ciphertext;
            chatEl.appendChild(div);
            chatEl.scrollTop = chatEl.scrollHeight;
        }
    };

    socket.onerror = (err) => console.error("WebSocket Fehler:", err);
    socket.onclose = ()  => console.warn("WebSocket geschlossen");
}
async function fetchRooms() {

    console.log("Daten erfolgreich fetch:");
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
        console.log("Räume geladen:", roomList);
        renderFunc(roomList);

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
    console.log("Daten erfolgreich geladen:", gefilterteListe);
}
function clickroom(room) {
    currentRoomId = room.room_id;

    const maininfo = document.querySelector('.maininfo');
    maininfo.style.display = 'none';
    const main = document.getElementById('main');
    main.style.display = 'flex';
    main.innerHTML = `
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
    loadChat(room);
}
async function loadChat(room) {
    const chatEl = document.getElementById('chat');
    const myFP   = sessionStorage.getItem('my_fingerprint');

    try {
        const response = await authFetch('GET', `/api/rooms/${room.room_id}/messages`);
        if (!response.ok) throw new Error(`HTTP-Fehler: ${response.status}`);

        const data     = await response.json();
        const messages = (data.messages ?? []).reverse();

        messages.forEach(msg => {
            const div = document.createElement('div');
            div.className = `bubble ${msg.sender_fp === myFP ? 'sent' : 'received'}`;
            div.innerText = msg.ciphertext;
            chatEl.appendChild(div);
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

    body.appendChild(leftCol);
    body.appendChild(rightCol);
    modal.appendChild(header);
    modal.appendChild(body);
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
            makeRow(u.username, 'add-btn', '+ Hinzufügen', () => addMember(room.room_id, u.fingerprint))
        ));
    }
    fetchRooms(); // aktualisiert die roomList mit den neuesten User-Infos (z.B. falls jemand gerade online gekommen ist)
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

async function addMember(roomId, fingerprint) {
    const res = await authFetch('POST', `/api/rooms/${roomId}/users`, { users: [fingerprint] });
    if (res.ok) { await fetchRooms(); const room = roomList.find(r => r.room_id === roomId); if (room) openRoomMenu(room); }
    else alert('Fehler beim Hinzufügen');
}

async function removeMember(roomId, fingerprint) {
    const res = await authFetch('DELETE', `/api/rooms/${roomId}/users`, { user_fp: fingerprint });
    if (res.ok) { await fetchRooms(); const room = roomList.find(r => r.room_id === roomId); if (room) openRoomMenu(room); }
    else alert('Fehler beim Entfernen');
}
function sendMessage(roomID) {
    const ciphertext = document.getElementById('schreibnachricht').value;
    document.getElementById('schreibnachricht').value = "";
    if (socket && socket.readyState === WebSocket.OPEN) {
        pendingSent.add(`${roomID}:${ciphertext}`);
        const messagePayload = {
            "type": "message",
            "payload": {
                "room_id": roomID,
                "epoch": 0,
                "ciphertext": ciphertext
            }
        };
        socket.send(JSON.stringify(messagePayload));
        send(ciphertext);
    } else {
        console.error("Socket ist nicht bereit. Verbindung prüfen!");
    }
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

        console.log("Sende Request mit Canonical:", canonical); // Zum Debuggen mit Go-Log vergleichen

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
window.addEventListener('DOMContentLoaded', async () => {
    await whoAmI();
    connectLocalChat();
    fetchRooms();
});