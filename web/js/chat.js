
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
        const room = roomList.find(r => r.room_id === currentRoomId);
        if (room) renderChatUserList(room.users);
    }
    panel.classList.toggle('open');
}

async function renderChatUserList(users) {
    const list = document.getElementById('chat-user-list');
    if (!list) return;
    list.innerHTML = '';

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
    if (window.innerWidth <= 768) closeSidebar();

    document.querySelectorAll('.chat-item').forEach(el => el.classList.remove('active'));
    const activeItem = document.querySelector(`.chat-item[data-room-id="${room.room_id}"]`);
    if (activeItem) activeItem.classList.add('active');

    const maininfo = document.querySelector('.maininfo');
    maininfo.style.display = 'none';
    const main = document.getElementById('main');
    main.style.display = 'flex';
    main.innerHTML = `
                <div class="chat-header">
                    <div class="chat-header-left">
                        <span class="chat-header-name">${escapeHtml(room.name)}</span>
                        <button class="leave-room-btn" onclick="leaveRoom('${room.room_id}')" title="Leave chat"><i class="fas fa-sign-out-alt"></i></button>
                    </div>
                    <div class="chat-header-actions">
                        <div class="voice-controls">
                            <div id="voice-avatars" class="voice-avatars-row"></div>
                            <button id="voice-join-btn" class="voice-btn" onclick="toggleVoice('${room.room_id}')">🎙️ Voice</button>
                            <button id="voice-mute-btn" class="voice-btn" onclick="toggleMute()" style="display:none">🎤 Mute</button>
                        </div>
                        <div class="chat-user-panel-wrapper" style="position:relative">
                            <button class="chat-user-btn" onclick="toggleVoicePanel()" title="Voice participants">
                                <i class="fas fa-headphones"></i>
                            </button>
                            <div class="voice-panel" id="voice-panel">
                                <div class="voice-panel-title">IN VOICE</div>
                                <div id="voice-participants-list"></div>
                            </div>
                        </div>
                        <div class="chat-user-panel-wrapper">
                            <button class="chat-user-btn" onclick="toggleChatUserList()" title="Members">
                                <i class="fas fa-users"></i>
                            </button>
                            <div class="chat-user-panel" id="chat-user-panel">
                                <div class="chat-user-panel-title">MEMBERS</div>
                                <div id="chat-user-list"></div>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="chat-messages" id="chat">
                </div>
                <div class="chat-input">
                    <div class="input-bar">
                        <input type="text" id="schreibnachricht" placeholder="Write a message...">
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
    if (room.host) cacheMemberEd25519Keys([room.host]);
    cacheMemberEd25519Keys(room.users ?? []);

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
            const cached = roomMessageCache.get(room.room_id) ?? [];
            if (cached.length === 0) {
                const empty = document.createElement('div');
                empty.className = 'chat-empty';
                empty.textContent = 'No messages yet.';
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
                            badge.title = 'Temporary user';
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
                    badge.title = 'Temporary user';
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
            if (room.room_id === currentRoomId) div.classList.add('active');
            div.setAttribute('data-room-id', room.room_id);

            div.innerHTML = `
                <div class="chat-item-row">
                    <div style="flex:1;min-width:0;cursor:pointer;" class="room-info">
                        <div style="font-weight:bold;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(room.name)}</div>
                    </div>
                    <span class="unread-badge" style="display:none"></span>
                    ${isHost ? `<button class="room-menu-btn" title="Manage members"><i class="fas fa-user-plus"></i></button>` : ''}
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

        unreadCounts.forEach((count, roomId) => {
            if (count > 0) updateUnreadUI(roomId);
        });
    } else {
        container.innerHTML = '<i class="fas fa-users" style="font-size: 2rem; margin-bottom: 10px;"></i><p>No active chats</p>';
    }
}

// Member Modal

let activeMemberModal = null;

function closeMemberModal() {
    if (activeMemberModal) { activeMemberModal.remove(); activeMemberModal = null; }
}

async function openRoomMenu(room) {
    closeMemberModal();

    const overlay = document.createElement('div');
    overlay.className = 'member-modal-overlay';
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeMemberModal(); });

    const modal = document.createElement('div');
    modal.className = 'member-modal';

    const header = document.createElement('div');
    header.className = 'member-modal-header';
    const title = document.createElement('h3');
    title.textContent = 'MANAGE MEMBERS';
    const closeBtn = document.createElement('button');
    closeBtn.className = 'member-modal-close';
    closeBtn.textContent = '✕';
    closeBtn.onclick = closeMemberModal;
    header.appendChild(title);
    header.appendChild(closeBtn);

    const body = document.createElement('div');
    body.className = 'member-modal-body';

    const leftCol = document.createElement('div');
    leftCol.className = 'member-col';
    const leftTitle = document.createElement('div');
    leftTitle.className = 'member-col-title';
    leftTitle.textContent = 'Members';
    const leftList = document.createElement('div');
    leftList.className = 'member-col-list';
    leftCol.appendChild(leftTitle);
    leftCol.appendChild(leftList);

    const rightCol = document.createElement('div');
    rightCol.className = 'member-col';
    const rightTitle = document.createElement('div');
    rightTitle.className = 'member-col-title';
    rightTitle.textContent = 'Add';
    const rightList = document.createElement('div');
    rightList.className = 'member-col-list';
    rightCol.appendChild(rightTitle);
    rightCol.appendChild(rightList);

    const footer = document.createElement('div');
    footer.className = 'member-modal-footer';
    const deleteBtn = document.createElement('button');
    deleteBtn.className = 'del-group-btn';
    deleteBtn.textContent = 'Delete group';
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

    leftList.innerHTML = '<div class="member-col-empty">Loading...</div>';
    rightList.innerHTML = '<div class="member-col-empty">Loading...</div>';

    const myFP = sessionStorage.getItem('my_fingerprint');
    const allResp = await authFetch('GET', '/api/users').catch(() => null);
    const allUsersRaw = allResp?.ok ? (await allResp.json()) : [];
    const allUsers = Array.isArray(allUsersRaw) ? allUsersRaw : [];

    const onlineFPs = new Set(allUsers.filter(u => u.online).map(u => u.fingerprint));
    const memberFPs = new Set((room.users ?? []).map(u => u.fingerprint));
    const removable = (room.users ?? []).filter(u => u.fingerprint !== myFP);
    const addable = allUsers.filter(u => u.online && !memberFPs.has(u.fingerprint) && u.fingerprint !== myFP);

    leftList.innerHTML = '';
    if (removable.length === 0) {
        leftList.innerHTML = '<div class="member-col-empty">No other members</div>';
    } else {
        removable.forEach(u => leftList.appendChild(
            makeRow(u.username, 'rem-btn', '– Remove', onlineFPs.has(u.fingerprint), () => removeMember(room.room_id, u.fingerprint))
        ));
    }

    rightList.innerHTML = '';
    if (addable.length === 0) {
        rightList.innerHTML = '<div class="member-col-empty">No online users available</div>';
    } else {
        addable.forEach(u => rightList.appendChild(
            makeRow(u.username, 'add-btn', '+ Add', true, () => addMember(room.room_id, u))
        ));
    }
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
    } else alert('Error adding member');
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
    } else alert('Error removing member');
}

async function deleteGroup(roomId) {
    const myFP = sessionStorage.getItem('my_fingerprint');
    if (!confirm('Really delete this group?')) return;
    const res = await authFetch('DELETE', `/api/rooms/${roomId}/users/${myFP}`);
    if (res.ok) {
        closeMemberModal();
        currentRoomId = null;
        currentRoomEpoch = 0;
        const main = document.getElementById('main');
        main.style.display = 'none';
        document.querySelector('.maininfo').style.display = '';
        await fetchRooms();
    } else alert('Error deleting group');
}

async function leaveRoom(roomId) {
    const myFP = sessionStorage.getItem('my_fingerprint');
    const isEphemeral = Number(sessionStorage.getItem('my_mode') ?? 1) === 0;
    if (!confirm('Really leave this chat?')) return;
    const res = await authFetch('DELETE', `/api/rooms/${roomId}/users/${myFP}`);
    if (res.ok || (isEphemeral && res.status === 404)) {
        currentRoomId = null;
        currentRoomEpoch = 0;
        const main = document.getElementById('main');
        main.style.display = 'none';
        document.querySelector('.maininfo').style.display = '';
        await fetchRooms();
    } else {
        alert('Error leaving chat');
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
    const ciphertext = JSON.stringify(encrypted);

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
    send(plaintext);

    if (!roomMessageCache.has(roomID)) roomMessageCache.set(roomID, []);
    roomMessageCache.get(roomID).push({
        isMine: true,
        senderName: null,
        senderMode: Number(sessionStorage.getItem('my_mode') ?? 1),
        text: plaintext,
        time: Math.floor(Date.now() / 1000)
    });
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

async function submitNewChat() {
    const inputField = document.getElementById('groupNameInput');
    const groupName = inputField.value.trim();

    if (!groupName) {
        alert("Please enter a group name!");
        return;
    }
    if (groupName.length > 100) {
        alert("Group name must be at most 100 characters!");
        return;
    }

    await newChat(groupName);

    inputField.value = "";
    document.getElementById('newChatModal').close();
}

async function newChat(groupName) {
    const sessionId = sessionStorage.getItem('sessionId');
    const timestamp = Math.floor(Date.now() / 1000).toString();
    const path = '/api/rooms';

    const bodyObj = { groupName: groupName };
    const bodyString = JSON.stringify(bodyObj);

    try {
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
            const errorText = await response.text();
            console.error("Server Antwort (Error):", errorText);
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        const data = await response.json();
        fetchRooms();

    } catch (err) {
        console.error("Fehler beim Request:", err);
        alert("Error: " + err.message);
    }
}
