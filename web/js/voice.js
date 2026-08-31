
let voiceRoomId = null;
let localStream = null;
const peerConnections = new Map();
const peerStates = new Map();
const pendingIceCandidates = new Map();
let isMuted = false;
const voiceParticipants = new Set();
const mutedUsers = new Set();

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
        alert('Microphone not available: ' + e.message);
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

    flushPendingIceCandidates(remoteFP);
    return pc;
}

async function handleSignalMessage(payload) {
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
    const myFP = sessionStorage.getItem('my_fingerprint');

    if (payload.voice_users) {
        payload.voice_users.forEach(fp => voiceParticipants.add(fp));
        if (voiceRoomId === payload.room_id) {
            for (const fp of payload.voice_users) {
                await createPeerConnection(fp, voiceRoomId, true);
            }
        }
    } else {
        voiceParticipants.add(payload.fingerprint);
    }

    if (voiceRoomId === payload.room_id) {
        if (payload.voice_users) {
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
        joinBtn.textContent = isInVoice ? '🔴 Leave' : '🎙️ Voice';
        joinBtn.className = isInVoice ? 'voice-btn active' : 'voice-btn';
    }

    const muteBtn = document.getElementById('voice-mute-btn');
    if (muteBtn) {
        muteBtn.style.display = isInVoice ? '' : 'none';
        muteBtn.textContent = isMuted ? '🔇 Muted' : '🎤 Mute';
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

    const avatarRow = document.getElementById('voice-avatars');
    if (avatarRow) {
        avatarRow.innerHTML = '';
        voiceParticipants.forEach(fp => {
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
        label.textContent = `${name}${isMe ? ' (me)' : ''}${isMutedFP ? ' 🔇' : ''}`;
        div.appendChild(label);
        list.appendChild(div);
    });

    if (voiceParticipants.size === 0) {
        const empty = document.createElement('div');
        empty.style.cssText = 'padding:10px 14px;font-size:0.75rem;color:rgba(255,255,255,0.3)';
        empty.textContent = 'No one in voice';
        list.appendChild(empty);
    }
}
