
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