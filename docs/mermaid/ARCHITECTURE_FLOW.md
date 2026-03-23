# Architecture Flow — WebSocket Auth → Datenbank

## Nachrichtenfluss (Message Flow)

```mermaid
sequenceDiagram
    actor Client
    participant WsHandler as api/handlers/ws.go<br/>(Ws Handler)
    participant SessionStore as session/store.go<br/>(Session Store)
    participant HubUser as hub/user.go<br/>(User.ReadPump)
    participant Hub as hub/hub.go<br/>(Hub.Run)
    participant HubHandler as hub/hub_handler.go<br/>(handleInboundMessage)
    participant EventConsumer as app/event_consumer.go<br/>(startEventConsumer)
    participant MessageService as service/message_service.go<br/>(MessageService)
    participant MessageRepo as infrastructure/db/message_repository.go<br/>(MessageRepository)
    participant SQLite as SQLite DB

    %% --- WebSocket Upgrade ---
    Client->>WsHandler: HTTP GET /ws (Upgrade)
    WsHandler-->>Client: 101 Switching Protocols
    WsHandler-->>Client: {"type":"info","payload":{"message":"initiated"}}

    %% --- Auth Handshake ---
    Client->>WsHandler: {"type":"auth", "payload":{"Chat-Session-ID":"...", "Chat-Timestamp":..., "Chat-Signature":"..."}}
    WsHandler->>SessionStore: Get(sessionID)
    alt Session ungültig
        SessionStore-->>WsHandler: not found
        WsHandler-->>Client: Verbindung geschlossen
    else Session gültig
        SessionStore-->>WsHandler: *session.User
        WsHandler->>Hub: RegisterUser ← hub.User{Fingerprint, Username, ws}
        Hub->>Hub: handleRegisterUser()<br/>speichert User in Hub.Users map
        Note over WsHandler,Hub: goroutine: WritePump gestartet
        WsHandler->>HubUser: ReadPump(Hub.InboundMessages) — blockiert
    end

    %% --- Nachricht senden ---
    Client->>HubUser: {"type":"message", "payload":{"roomId":"...", "ciphertext":"...", "epoch":...}}
    HubUser->>HubUser: JSON unmarshal → WSMessageEnvelope<br/>envelope.Sender = User
    HubUser->>Hub: InboundMessages ← envelope

    %% --- Hub verarbeitet Nachricht ---
    Hub->>HubHandler: handleInboundMessage(msg)
    HubHandler->>HubHandler: getRoom(payload.RoomID)<br/>isUserInRoom(sender, room)
    alt User nicht im Raum oder Raum existiert nicht
        HubHandler-->>Hub: abgebrochen (kein Event)
    else Validierung OK
        HubHandler->>HubHandler: outboundEnvelope{SenderFP, Type, Payload} marshalen
        HubHandler->>HubUser: sendMessageToRoom()<br/>→ User.OutGoingMessages chan
        HubUser-->>Client: Nachricht an alle Raummitglieder (WritePump)

        HubHandler->>Hub: emit(HubEvent{Type: HubEventMessageCreated, Payload: MessageCreatedPayload})
        Note over Hub: Events chan (gepuffert, 256) — non-blocking
    end

    %% --- Asynchrone Persistierung ---
    Hub-)EventConsumer: HubEvent{HubEventMessageCreated, ...}<br/>(via buffered channel)
    EventConsumer->>MessageService: ProcessMessage(MessageCreatedPayload)
    MessageService->>MessageService: roomRepo.GetByID(roomID)<br/>— prüft ob Raum persistiert ist
    alt Raum ephemer (nicht in DB)
        MessageService-->>EventConsumer: nil (übersprungen)
    else Raum persistent
        MessageService->>MessageRepo: Save(&MessageRecord{ID, RoomID, SenderFP, Epoch, Ciphertext, CreatedAt})
        MessageRepo->>SQLite: INSERT INTO messages (...)
        SQLite-->>MessageRepo: OK
        MessageRepo-->>MessageService: nil
        MessageService-->>EventConsumer: nil
    end

    %% --- Disconnect ---
    Client->>HubUser: Verbindung getrennt
    HubUser-->>WsHandler: ReadPump returns
    WsHandler->>Hub: UnregisterUser ← User
    Hub->>Hub: handleUnregisterUser()<br/>löscht User aus Hub.Users map
```

## Komponentenübersicht

| Schicht | Komponente | Aufgabe |
|---|---|---|
| Transport | `api/handlers/ws.go` | WebSocket-Upgrade, Auth-Handshake, Pump-Start |
| Auth | `session/store.go` | In-Memory Session-Lookup (sessionID → User) |
| Realtime | `hub/hub.go` | Zentraler Event-Loop (channel-basiert) |
| Realtime | `hub/hub_handler.go` | Nachrichtenrouting, Room-Validierung, Event-Emission |
| Realtime | `hub/user.go` | ReadPump / WritePump pro WebSocket-Verbindung |
| Event-Bridge | `app/event_consumer.go` | Asynchroner Consumer: Hub-Events → Services |
| Business | `service/message_service.go` | Ephemeral-Check, Delegation an Repository |
| Persistenz | `infrastructure/db/message_repository.go` | SQL INSERT/SELECT gegen SQLite |

## Wichtige Design-Entscheidungen

- **Auth first**: Der WsHandler akzeptiert als erste Nachricht ausschließlich `type:"auth"` — jede andere Nachricht schließt die Verbindung sofort.
- **Non-blocking emit**: `hub.emit()` verwirft Events wenn der Kanal voll ist (256 Puffer), damit der Hub-Loop nie blockiert.
- **Ephemeral Rooms**: Nachrichten in Räumen ohne DB-Eintrag werden nicht persistiert — kein Fehler, nur stilles Überspringen (`ProcessMessage`).
- **Entkopplung Hub ↔ DB**: Der Hub weiß nichts von der Datenbank. Die Persistierung läuft vollständig asynchron über den Event-Consumer.
