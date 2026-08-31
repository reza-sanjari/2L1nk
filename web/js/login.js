
let _appInfo = null;

async function loadAppInfo() {
    if (_appInfo) return _appInfo;
    try {
        const res = await fetch('/api/info');
        if (!res.ok) throw new Error();
        _appInfo = await res.json();
    } catch (e) { _appInfo = null; }
    return _appInfo;
}

async function initVersion() {
    const data = await loadAppInfo();
    if (!data) return;
    const el = document.getElementById('app-version');
    if (el) { el.textContent = 'v' + data.version; el.style.display = 'inline-block'; }
}

function openChangelog() {
    document.getElementById('changelog-modal').style.display = 'flex';
    document.body.style.overflow = 'hidden';
    renderChangelog();
}

function closeChangelog() {
    document.getElementById('changelog-modal').style.display = 'none';
    document.body.style.overflow = '';
}

function closeChangelogOverlay(e) {
    if (e.target === document.getElementById('changelog-modal')) closeChangelog();
}

async function renderChangelog() {
    const body = document.getElementById('cl-modal-body');
    const data = await loadAppInfo();
    if (!data || !data.changelog || !data.changelog.length) {
        body.innerHTML = '<div class="cl-loading">No changelog available.</div>';
        return;
    }
    body.innerHTML = data.changelog.map(entry => {
        const notes = entry.notes.map(n => `<li>${n}</li>`).join('');
        return `<div class="cl-entry">
            <div class="cl-entry-header">
                <span class="cl-version">${entry.version}</span>
                <span class="cl-date">${entry.date}</span>
            </div>
            <ul class="cl-notes">${notes}</ul>
        </div>`;
    }).join('');
}

// IndexedDB Key Store
const IDB_NAME    = '2l1nk_keystore';
const IDB_STORE   = 'keypairs';
const IDB_VERSION = 1;

function openKeyDB() {
    return new Promise((resolve, reject) => {
        const req = indexedDB.open(IDB_NAME, IDB_VERSION);
        req.onupgradeneeded = e => e.target.result.createObjectStore(IDB_STORE, { keyPath: 'username' });
        req.onsuccess = e => resolve(e.target.result);
        req.onerror   = e => reject(e.target.error);
    });
}

async function saveKeysToIDB(username, keys) {
    const db  = await openKeyDB();
    return new Promise((resolve, reject) => {
        const tx  = db.transaction(IDB_STORE, 'readwrite');
        tx.objectStore(IDB_STORE).put({ username, ...keys });
        tx.oncomplete = resolve;
        tx.onerror    = e => reject(e.target.error);
    });
}

async function loadKeysFromIDB(username) {
    const db = await openKeyDB();
    return new Promise((resolve, reject) => {
        const req = db.transaction(IDB_STORE, 'readonly')
                      .objectStore(IDB_STORE).get(username);
        req.onsuccess = e => resolve(e.target.result ?? null);
        req.onerror   = e => reject(e.target.error);
    });
}

async function deleteKeysFromIDB(username) {
    const db = await openKeyDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(IDB_STORE, 'readwrite');
        tx.objectStore(IDB_STORE).delete(username);
        tx.oncomplete = resolve;
        tx.onerror    = e => reject(e.target.error);
    });
}

// Saved Logins (localStorage)
const SAVED_KEY = '2l1nk_saved_logins';

function getSavedLogins() {
    try { return JSON.parse(localStorage.getItem(SAVED_KEY)) ?? []; }
    catch { return []; }
}

function saveLogin(username, gatewayKey) {
    const logins = getSavedLogins().filter(l => l.username !== username);
    logins.unshift({ username, gatewayKey });
    localStorage.setItem(SAVED_KEY, JSON.stringify(logins.slice(0, 5)));
}

function removeLogin(username) {
    const logins = getSavedLogins().filter(l => l.username !== username);
    localStorage.setItem(SAVED_KEY, JSON.stringify(logins));
    deleteKeysFromIDB(username).catch(() => {});
    renderFastLogins();
}

function renderFastLogins() {
    const logins  = getSavedLogins();
    const section = document.getElementById('fast-login-section');
    const list    = document.getElementById('fast-login-list');
    if (logins.length === 0) { section.style.display = 'none'; return; }
    section.style.display = 'flex';
    list.innerHTML = '';
    logins.forEach(l => {
        const card = document.createElement('div');
        card.className = 'fast-login-card';
        const avatar = document.createElement('div');
        avatar.className = 'fast-login-avatar';
        avatar.textContent = l.username.charAt(0).toUpperCase();
        const info = document.createElement('div');
        info.className = 'fast-login-info';
        const name = document.createElement('div');
        name.className = 'fast-login-name';
        name.textContent = l.username;
        info.appendChild(name);
        const goBtn = document.createElement('button');
        goBtn.className = 'fast-login-go';
        goBtn.title = 'Sign in';
        goBtn.innerHTML = '<i class="fas fa-sign-in-alt"></i>';
        goBtn.onclick = () => quickLogin(l.username, l.gatewayKey);
        const removeBtn = document.createElement('button');
        removeBtn.className = 'fast-login-remove';
        removeBtn.title = 'Remove';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.onclick = () => removeLogin(l.username);
        card.appendChild(avatar);
        card.appendChild(info);
        card.appendChild(goBtn);
        card.appendChild(removeBtn);
        list.appendChild(card);
    });
}

async function quickLogin(username, gatewayKey) {
    document.getElementById('gateway').value   = gatewayKey;
    document.getElementById('loginname').value = username;
    await connectToGateway(1);
}

// Mode Modal
function showModeModal() {
    const gateKey   = document.getElementById('gateway').value.trim();
    const loginName = document.getElementById('loginname').value.trim();
    if (!gateKey)             { alert("Please enter a gateway code!"); return; }
    if (!loginName)           { alert("Please enter a username!"); return; }
    if (loginName.length > 50){ alert("The username must be at most 50 characters long!"); return; }
    document.getElementById('mode-overlay').classList.add('open');
}

function closeModeModal() {
    document.getElementById('mode-overlay').classList.remove('open');
}

// Helpers
function b64(bytes) { return btoa(String.fromCharCode(...bytes)); }

function fromB64(str) {
    return Uint8Array.from(atob(str), c => c.charCodeAt(0));
}

// Main Login — mode: 0 = ephemeral, 1 = persistent
async function connectToGateway(mode) {
    closeModeModal();

    const gateKey   = document.getElementById('gateway').value.trim();
    const loginName = document.getElementById('loginname').value.trim();

    if (!gateKey)   { alert("Please enter a gateway code!"); return; }
    if (!loginName) { alert("Please enter a username!"); return; }

    const isPersistent = mode === 1;

    let ed25519Secret, ed25519Public, x25519Secret, x25519Public;

    const stored = isPersistent
        ? await loadKeysFromIDB(loginName).catch(() => null)
        : null;

    if (stored) {
        ed25519Secret = stored.ed25519_secret;
        ed25519Public = stored.ed25519_public;
        x25519Secret  = stored.x25519_secret;
        x25519Public  = stored.x25519_public;
    } else {
        const signingKP = nacl.sign.keyPair();
        const dhKP      = nacl.box.keyPair();
        ed25519Secret = b64(signingKP.secretKey);
        ed25519Public = b64(signingKP.publicKey);
        x25519Secret  = b64(dhKP.secretKey);
        x25519Public  = b64(dhKP.publicKey);
    }

    sessionStorage.setItem('ed25519_secret', ed25519Secret);
    sessionStorage.setItem('ed25519_public',  ed25519Public);
    sessionStorage.setItem('x25519_secret',   x25519Secret);
    sessionStorage.setItem('x25519_public',   x25519Public);

    const timestamp = Math.floor(Date.now() / 1000).toString();
    const nonce     = b64(nacl.randomBytes(16));
    const body = JSON.stringify({ gateToken: gateKey, publicKey: ed25519Public, x25519PublicKey: x25519Public, username: loginName, mode });
    const hashBuf  = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(body));
    const bodyHash = Array.from(new Uint8Array(hashBuf)).map(b => b.toString(16).padStart(2, '0')).join('');
    const canonical =
        `GATE\n${timestamp}\n${nonce}\n${mode}\n${loginName}\n` +
        `${ed25519Public}\n${x25519Public}\n${bodyHash}`;

    const sig       = nacl.sign.detached(new TextEncoder().encode(canonical), fromB64(ed25519Secret));
    const signature = b64(sig);

    try {
        const response = await fetch('/api/auth/gate', {
            method: 'POST',
            headers: {
                'Chat-Timestamp': timestamp,
                'Chat-Nonce':     nonce,
                'Chat-Signature': signature,
                'Content-Type':   'application/json'
            },
            body
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({ error: response.statusText }));
            throw new Error(errorData.error ?? `Server error ${response.status}`);
        }

        if (isPersistent && !stored) {
            await saveKeysToIDB(loginName, {
                ed25519_secret: ed25519Secret,
                ed25519_public: ed25519Public,
                x25519_secret:  x25519Secret,
                x25519_public:  x25519Public,
            }).catch(() => {});
        }

        if (isPersistent) {
            saveLogin(loginName, gateKey);
        }

        sessionStorage.setItem('username',   loginName);
        sessionStorage.setItem('gatewayKey', gateKey);
        sessionStorage.setItem('my_mode',    isPersistent ? 'Persistent' : 'Ephemeral');
        const data = await response.json();
        sessionStorage.setItem('sessionId', data.sessionId);
        window.location.href = "Mainsite";
    } catch (err) {
        alert("Error connecting to gateway:\n" + err.message);
    }
}

document.addEventListener('DOMContentLoaded', () => { renderFastLogins(); initVersion(); });
