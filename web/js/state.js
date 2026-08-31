
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
const pendingSent = new Set();
const roomKeys = new Map();
const memberEd25519Keys = new Map();
const unreadCounts = new Map();
const roomMessageCache = new Map();
const pendingEncryptedMessages = new Map();
let pendingScrollMsgId = null;
