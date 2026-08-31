
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
            if (p.host) cacheMemberEd25519Keys([p.host]);
            cacheMemberEd25519Keys(p.users ?? []);
            const idx = roomList.findIndex(r => r.room_id === p.room_id);
            if (idx !== -1) {
                const oldRoom = roomList[idx];
                const merged = { ...oldRoom, ...p };
                const oldHostPersistent = oldRoom.host && oldRoom.host.mode !== 0;
                const hostChanged = p.host && oldRoom.host && p.host.fingerprint !== oldRoom.host.fingerprint;
                if (oldHostPersistent && hostChanged) {
                    merged.host = oldRoom.host;
                }
                roomList[idx] = merged;
            }
            renderFunc(roomList);
            if (p.room_id === currentRoomId && Array.isArray(p.users)) {
                renderChatUserList(p.users);
            }
            return;
        }

        if (envelope.type === "room_key_rotation") {
            const p = envelope.payload;
            const rotIdx = roomList.findIndex(r => r.room_id === p.room_id);
            if (rotIdx !== -1) roomList[rotIdx] = { ...roomList[rotIdx], epoch: p.epoch };
            if (p.room_id === currentRoomId) currentRoomEpoch = p.epoch;
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
                if (p.room_id === currentRoomId) currentRoomEpoch = p.epoch;
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
            for (const [roomId, msgs] of roomMessageCache.entries()) {
                roomMessageCache.set(roomId, msgs.filter(m => m.isMine ? true : m.senderFP !== p.sender_fp));
            }
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

            const myFP = sessionStorage.getItem('my_fingerprint');
            const sentKey = `${payload.room_id}:${payload.ciphertext}`;
            if (payload.sender_fp === myFP || pendingSent.has(sentKey)) {
                pendingSent.delete(sentKey);
                return;
            }

            (async () => {
                if (!await verifyIncomingMessageSignature(payload)) return;
                renderVerifiedIncomingMessage(payload);
            })();
        }
    };

    socket.onerror = (err) => console.error("WebSocket Fehler:", err);
    socket.onclose = () => {
        console.warn("WebSocket geschlossen, reconnect in Kürze...");
        if (sessionStorage.getItem('sessionId')) scheduleWsReconnect();
    };
}

function renderVerifiedIncomingMessage(payload) {
    if (payload.room_id !== currentRoomId) {
        unreadCounts.set(payload.room_id, (unreadCounts.get(payload.room_id) ?? 0) + 1);
        updateUnreadUI(payload.room_id);
        const cfg = Settings.load();
        if (cfg.notifSound) playNotifSound();
        if (cfg.notifDesktop) {
            const room = roomList.find(r => r.room_id === payload.room_id);
            showDesktopNotif(room?.name ?? 'Neue Nachricht', null);
        }
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
            badge.title = 'Temporary user';
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

    if (!roomMessageCache.has(payload.room_id)) roomMessageCache.set(payload.room_id, []);
    roomMessageCache.get(payload.room_id).push({
        isMine: false,
        senderFP: payload.sender_fp,
        senderName: payload.sender_name ?? null,
        senderMode: payload.sender_mode ?? 1,
        text,
        time: Math.floor(Date.now() / 1000)
    });
    chatEl.querySelector('.chat-empty')?.remove();
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

        roomList.forEach(r => {
            if (r.host?.fingerprint) {
                localStorage.setItem(`2l1nk_host_${r.room_id}`, r.host.fingerprint);
            }
            if (r.host) cacheMemberEd25519Keys([r.host]);
            cacheMemberEd25519Keys(r.users ?? []);
        });

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
