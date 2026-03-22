
// ============================================================
// CRYPTO MODULE
// Key Pair Model: Ed25519 (Signing/Identity) + X25519 (ECDH)
//
// Ein Ed25519-Schlüsselpaar dient als Identität.
// Zusätzlich wird ein X25519-Schlüsselpaar für ECDH generiert
// (Verschlüsselung von Room-Keys an Mitglieder).
// Nachrichten werden mit AES-256-GCM symmetrisch verschlüsselt.
// ============================================================

const AppCrypto = (() => {
    const subtle = crypto.subtle;

    function bufToB64(buf) {
        return btoa(String.fromCharCode(...new Uint8Array(buf)));
    }

    function b64ToBuf(b64) {
        return Uint8Array.from(atob(b64), c => c.charCodeAt(0)).buffer;
    }

    function strToBuf(str) {
        return new TextEncoder().encode(str);
    }

    function bufToStr(buf) {
        return new TextDecoder().decode(buf);
    }

    async function exportPublicKeyRaw(key) {
        return bufToB64(await subtle.exportKey('raw', key));
    }

    async function exportKeyJWK(key) {
        return JSON.stringify(await subtle.exportKey('jwk', key));
    }

    // --- Identität generieren & speichern ---

    async function generateIdentity() {
        // Ed25519: Signing / Authentifizierung
        const signingKP = await subtle.generateKey(
            { name: 'Ed25519' }, true, ['sign', 'verify']
        );
        // X25519: ECDH / Room-Key Verschlüsselung
        const dhKP = await subtle.generateKey(
            { name: 'X25519' }, true, ['deriveKey', 'deriveBits']
        );

        sessionStorage.setItem('ed25519_private', await exportKeyJWK(signingKP.privateKey));
        sessionStorage.setItem('ed25519_public',  await exportPublicKeyRaw(signingKP.publicKey));
        sessionStorage.setItem('x25519_private',  await exportKeyJWK(dhKP.privateKey));
        sessionStorage.setItem('x25519_public',   await exportPublicKeyRaw(dhKP.publicKey));

        return {
            signingPublicKey: sessionStorage.getItem('ed25519_public'),
            dhPublicKey:      sessionStorage.getItem('x25519_public')
        };
    }

    async function loadIdentity() {
        const edPrivRaw = sessionStorage.getItem('ed25519_private');
        const xPrivRaw  = sessionStorage.getItem('x25519_private');
        if (!edPrivRaw || !xPrivRaw) return null;

        const signingPrivate = await subtle.importKey(
            'jwk', JSON.parse(edPrivRaw), { name: 'Ed25519' }, false, ['sign']
        );
        const dhPrivate = await subtle.importKey(
            'jwk', JSON.parse(xPrivRaw), { name: 'X25519' }, false, ['deriveKey', 'deriveBits']
        );
        return { signingPrivate, dhPrivate };
    }

    // --- Ed25519: Signieren & Verifizieren ---

    async function sign(data) {
        const id = await loadIdentity();
        if (!id) throw new Error('Keine Identität geladen');
        const sig = await subtle.sign('Ed25519', id.signingPrivate, strToBuf(data));
        return bufToB64(sig);
    }

    async function verify(data, signatureB64, signerPublicKeyB64) {
        const pubKey = await subtle.importKey(
            'raw', b64ToBuf(signerPublicKeyB64), { name: 'Ed25519' }, false, ['verify']
        );
        return await subtle.verify('Ed25519', pubKey, b64ToBuf(signatureB64), strToBuf(data));
    }

    // --- X25519 ECDH + AES-GCM: Room-Key Verschlüsselung ---

    // Verschlüsselt einen Room-Key für einen Empfänger.
    // Gibt { ephemeralPub, iv, ciphertext } (alles Base64) zurück.
    async function encryptRoomKey(roomKeyBuf, recipientDHPublicB64) {
        const ephemeral = await subtle.generateKey(
            { name: 'X25519' }, true, ['deriveKey']
        );
        const ephemeralPubB64 = await exportPublicKeyRaw(ephemeral.publicKey);

        const recipientPub = await subtle.importKey(
            'raw', b64ToBuf(recipientDHPublicB64), { name: 'X25519' }, false, []
        );
        const sharedKey = await subtle.deriveKey(
            { name: 'X25519', public: recipientPub },
            ephemeral.privateKey,
            { name: 'AES-GCM', length: 256 },
            false,
            ['encrypt']
        );

        const iv = crypto.getRandomValues(new Uint8Array(12));
        const ciphertext = await subtle.encrypt(
            { name: 'AES-GCM', iv },
            sharedKey,
            roomKeyBuf
        );

        return {
            ephemeralPub: ephemeralPubB64,
            iv:           bufToB64(iv),
            ciphertext:   bufToB64(ciphertext)
        };
    }

    // Entschlüsselt einen Room-Key mit dem eigenen X25519-Private-Key.
    async function decryptRoomKey({ ephemeralPub, iv, ciphertext }) {
        const id = await loadIdentity();
        if (!id) throw new Error('Keine Identität geladen');

        const senderPub = await subtle.importKey(
            'raw', b64ToBuf(ephemeralPub), { name: 'X25519' }, false, []
        );
        const sharedKey = await subtle.deriveKey(
            { name: 'X25519', public: senderPub },
            id.dhPrivate,
            { name: 'AES-GCM', length: 256 },
            false,
            ['decrypt']
        );

        return await subtle.decrypt(
            { name: 'AES-GCM', iv: b64ToBuf(iv) },
            sharedKey,
            b64ToBuf(ciphertext)
        );
    }

    // --- AES-GCM: Nachrichten verschlüsseln & entschlüsseln ---

    async function encryptMessage(plaintext, roomKeyBuf) {
        const roomKey = await subtle.importKey(
            'raw', roomKeyBuf, { name: 'AES-GCM' }, false, ['encrypt']
        );
        const iv = crypto.getRandomValues(new Uint8Array(12));
        const ciphertext = await subtle.encrypt(
            { name: 'AES-GCM', iv }, roomKey, strToBuf(plaintext)
        );
        return { iv: bufToB64(iv), ciphertext: bufToB64(ciphertext) };
    }

    async function decryptMessage({ iv, ciphertext }, roomKeyBuf) {
        const roomKey = await subtle.importKey(
            'raw', roomKeyBuf, { name: 'AES-GCM' }, false, ['decrypt']
        );
        const plainBuf = await subtle.decrypt(
            { name: 'AES-GCM', iv: b64ToBuf(iv) }, roomKey, b64ToBuf(ciphertext)
        );
        return bufToStr(plainBuf);
    }

    // Generiert einen zufälligen AES-256 Room-Key.
    function generateRoomKey() {
        return crypto.getRandomValues(new Uint8Array(32)).buffer;
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

    let socket;
    let roomList = [];
    let gefilterteListe = [];
    connectLocalChat();
    function connectLocalChat() {
        socket = new WebSocket('ws://localhost:8080/api/ws');
        socket.onopen = () => {
            const authPayload = {
                "type": "auth",
                "payload": {
                    "Chat-Session-ID": sessionStorage.getItem('sessionId'),
                    "Chat-Timestamp": Math.floor(Date.now() / 1000), // Aktueller Unix-Timestamp
                    "Chat-Signature": "test"
                }
            };

            socket.send(JSON.stringify(authPayload));
        };
    }
    async function fetchRooms() {
        try {
            const response = await fetch('http://localhost:8080/api/test/rooms', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            });

            if (!response.ok) {
                throw new Error(`HTTP-Fehler: ${response.status}`);
            }

            roomList = await response.json();
            renderFunc(roomList);

        } catch (error) {
            console.error("Fehler beim Abrufen der API-Daten:", error);
        }
    }
    function searchfunction(event) {
        const searchTerm = event.target.value.toLowerCase();

        const gefilterteListe = roomList.filter(room => {
            return room.RoomID.toLowerCase().includes(searchTerm) ||
                room.Host.toLowerCase().includes(searchTerm);
        });
        renderFunc(gefilterteListe);
        console.log("Daten erfolgreich geladen:", gefilterteListe);
    }
    function clickroom(room) {
        const maininfo = document.querySelector('.maininfo');
        maininfo.style.display = 'none';
        const main = document.getElementById('main');
        main.style.display = 'flex';
        main.innerHTML = `
                <div class="chat-messages" id="chat">
                </div>

                <div class="chat-input">
                    <input type="text" id="schreibnachricht" placeholder="Nachricht schreiben...">
                    <button onclick="sendMessage('${room.RoomID}')">Senden</button>
                </div>

            `;
        loadChat(room);
    }
    function loadChat(room) {
        //console.log("Lade Chat für Raum:", room.RoomID);
        const loadchatList = [
            { sender: 'received', text: 'Willkommen im Chat!' },
            { sender: 'sent', text: 'Danke! Freue mich hier zu sein.' },
            { sender: 'received', text: 'Wie findest du die App bisher?' },
            { sender: 'sent', text: 'Sieht super aus, besonders das Design!' }
        ];
        loadchatList.forEach(message => {
            const msg = document.createElement('div');
            msg.className = `bubble ${message.sender}`;
            msg.innerText = message.text;
            document.getElementById('chat').appendChild(msg);
        });
    }
    function renderFunc(RenderList) {

        const container = document.getElementById('chat-list-container');
        const emptyState = document.getElementById('empty-state');

        // Prüfen, ob Container existiert
        if (!container) return;

        if (RenderList && RenderList.length > 0) {
            // Räume gefunden
            if (emptyState) emptyState.style.display = 'none';
            container.innerHTML = "";

            RenderList.forEach(room => {
                const div = document.createElement('div');
                div.className = 'chat-item';
                div.innerHTML = `
                        <div style="font-weight: bold; cursor: pointer; padding: 10px; border-bottom: 1px solid #4b0082;">
                            👤${room.RoomID}
                            <div style="font-size: 0.7rem; opacity: 0.6;">Host: ${room.Host}</div>
                        </div>
                    `;
                div.onclick = () => clickroom(room);
                container.appendChild(div);
            });
        } else {
            // Falls Liste leer ist
            if (emptyState) emptyState.style.display = 'flex';
            container.innerHTML = '<i class="fas fa-users" style="font-size: 2rem; margin-bottom: 10px;"></i><p>Keine aktiven Chats</p>';
        }
    }
    function sendMessage(roomID) {
        const senderFP = "";
        const ciphertext = document.getElementById('schreibnachricht').value;
        document.getElementById('schreibnachricht').value = "";
        if (socket && socket.readyState === WebSocket.OPEN) {
            const messagePayload = {
                "type": "message",
                "payload": {
                    "room_id": roomID,
                    "epoch": 0, // Falls das dynamisch ist, hier anpassen
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
        if (ciphertext !== "") {
            const msg = document.createElement('div');
            msg.className = 'bubble sent';
            msg.innerText = ciphertext;
            document.getElementById('chat').appendChild(msg);
            document.getElementById('chat').scrollTop = document.getElementById('chat').scrollHeight;
        }
    }
    function whoAmI() {
        const username = sessionStorage.getItem('username');
        const usernamefield = document.getElementById('username');
        usernamefield.textContent = username;
        const usernameshortfield = document.getElementById('usernameshort');
        usernameshortfield.textContent = username.substring(0, 2).toUpperCase();

        // Sicherheit: Falls niemand eingeloggt ist, direkt zurückwerfen
        if (!username) {
            window.location.href = "index.html";
            return null;
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

        // Ruft deine API-Funktion auf
        await newChat(groupName);

        // Aufräumen
        inputField.value = "";
        document.getElementById('newChatModal').close();
    }
    async function newChat(groupName) {
        // 1. SessionID aus dem sessionStorage abrufen
        const sessionId = sessionStorage.getItem('sessionId');

        // 2. Zeitstempel und Platzhalter für die Signatur
        const timestamp = Math.floor(Date.now() / 1000).toString();
        const signature = "DEINE_SIGNATUR_LOGIK"; // Hier muss deine HMAC-Signatur hin

        try {
            const response = await fetch('http://localhost:8080/api/rooms', {
                method: 'POST',
                headers: {
                    'Chat-Session-ID': sessionId, // Hier wird die ID dynamisch eingesetzt
                    'Chat-Signature': signature,
                    'Chat-Timestamp': timestamp,
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    groupName: groupName
                })
            });

            const data = await response.json();

            // UI-Feedback
            alert("✅ Gruppe '" + groupName + "' wurde erstellt!");

        } catch (err) {
            console.error("Fehler beim Request:", err);
            alert("❌ Fehler: " + err.message);
        }
    }
    function logout() {
        sessionStorage.removeItem('username');
        window.location.href = "Login";
    }
    window.addEventListener('DOMContentLoaded', () => {
        whoAmI();
        fetchRooms();
    });