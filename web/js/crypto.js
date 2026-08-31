
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

    function generateIdentity() {
        const signingKP = nacl.sign.keyPair();
        const dhKP = nacl.box.keyPair();

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

async function hashBody(bodyString) {
    const msgBuffer = new TextEncoder().encode(bodyString);
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
}

function cacheMemberEd25519Keys(members) {
    if (!Array.isArray(members)) return;
    for (const m of members) {
        if (!m || !m.fingerprint || !m.ed25519_public_key) continue;
        memberEd25519Keys.set(m.fingerprint, m.ed25519_public_key);
    }
}

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
