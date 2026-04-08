# Projektdokumentation: 2L1nk

**Projekt:** 2L1nk – Selbstgehosteter, Ende-zu-Ende-verschlüsselter Chat
**Autoren:** Florian Kirsch, Reza Sanjari
**Schuljahr:** 2025/2026

---

## Teil 1 – Theoretischer Teil

### 1.1 Projektbeschreibung

2L1nk ist eine selbstgehostete Chat-Anwendung mit Ende-zu-Ende-Verschlüsselung (E2EE). Das bedeutet, dass alle Nachrichten bereits im Browser des Absenders verschlüsselt werden und erst im Browser des Empfängers wieder entschlüsselt werden. Der Server selbst hat zu keinem Zeitpunkt Zugriff auf den Inhalt der Nachrichten – er speichert und leitet ausschließlich verschlüsselte Daten weiter.

Dieses Prinzip wird als **Zero-Knowledge-Architektur** bezeichnet: Der Server "weiß" nichts über den tatsächlichen Inhalt der Kommunikation.

Kernfunktionen:
- Registrierung ohne Benutzername/Passwort (Identität basiert auf kryptographischen Schlüsseln)
- Verschlüsselte Gruppenräume mit automatischer Schlüsselrotation
- Unterstützung für ephemere (nicht gespeicherte) Benutzer
- Interaktive Terminal-Benutzeroberfläche (TUI) zur Serververwaltung

---

### 1.2 Verwendete Technologien und Programmiersprachen

#### Programmiersprachen

| Sprache | Einsatzbereich | Beschreibung |
|---|---|---|
| **Go** | Backend | Kompilierte, statisch typisierte Sprache von Google. Verwendet für Server, HTTP-API, WebSocket und die gesamte Serverlogik. Besonders geeignet durch eingebaute Nebenläufigkeit (Goroutines) und einfaches Cross-Compilation. |
| **JavaScript** (ES6+) | Frontend | Skriptsprache im Browser. Zuständig für die gesamte Benutzeroberfläche, Echtzeit-Kommunikation via WebSocket und alle kryptographischen Operationen auf Clientseite. Kein Framework – reines Vanilla JS. |
| **HTML** | Frontend | Struktursprache für den Aufbau der Webseiten (Chatraum, Login, Einstellungen). Wird vom Backend als statische Dateien ausgeliefert und via `go:embed` in die Binärdatei eingebettet. |
| **CSS** | Frontend | Stylesheet-Sprache für das visuelle Design der Anwendung. Beinhaltet 6 Farbthemen (Lila, Blau, Grün, Rot, Orange, Cyan) und unterstützt verschiedene Hintergrundstile sowie anpassbare Schriftgrößen. |
| **SQL** | Datenbank | Abfragesprache für die SQLite-Datenbank. Wird für alle Datenbankzugriffe verwendet (Benutzer, Räume, Nachrichten, Schlüssel). Migrationen sind in Go eingebettet und laufen automatisch beim Serverstart. |

#### Datenbank

Das Projekt verwendet **SQLite** als Datenbank.

SQLite ist eine eingebettete, dateibasierte Datenbank – sie benötigt keinen separaten Datenbankserver und speichert alle Daten in einer einzigen Datei (`2L1nk.db`). Das macht die Anwendung besonders einfach zu deployen: Eine einzige Binärdatei enthält den kompletten Server inklusive Frontend und Datenbank-Engine.

Als Treiber wird **modernc/sqlite** (Version 1.47.0) eingesetzt – ein Pure-Go-Treiber, der kein CGO benötigt und damit problemlos für alle Plattformen (Linux, Windows, Raspberry Pi) kompiliert werden kann.

#### Backend-Bibliotheken

| Bibliothek | Version | Zweck |
|---|---|---|
| **Echo v4** | 4.15.1 | HTTP-Framework: Routing, Middleware (Rate Limiting, Timeouts, Security Headers) |
| **Gorilla WebSocket** | 1.5.3 | Bidirektionale Echtzeit-Kommunikation zwischen Server und Browser |
| **golang.org/x/crypto** | 0.49.0 | Ed25519-Signaturen und X25519-Schlüsselaustausch serverseitig |
| **Uber Zap** | 1.27.1 | Strukturiertes, performantes Logging (JSON-Format) |
| **google/uuid** | 1.6.0 | Generierung von UUIDs für Räume und Nachrichten |

#### Frontend-Bibliotheken

| Bibliothek | Version | Zweck |
|---|---|---|
| **TweetNaCl.js** | 1.0.3 | Kryptographiebibliothek: Ed25519, X25519, NaCl SecretBox |
| **Web Crypto API** | Browser-nativ | SHA-256-Hashing für Request-Signaturen |

#### Terminal-Benutzeroberfläche (TUI)

| Bibliothek | Version | Zweck |
|---|---|---|
| **BubbleTea** | 1.3.10 | Framework für interaktive Terminal-UIs (Elm-Architektur) |
| **Lipgloss** | 1.1.0 | Styling und Farbgebung für Terminalausgaben |
| **Bubbles** | 1.0.0 | Wiederverwendbare TUI-Komponenten (Eingabefelder, Spinner etc.) |

#### Build & Deployment

| Technologie | Zweck |
|---|---|
| **GNU Make** | Build-Automatisierung (Cross-Compilation für Linux und Windows) |
| **Go embed** | Frontend-Dateien (HTML, CSS, JS) werden direkt in die Binärdatei eingebettet |

---

### 1.3 Kryptographische Grundlagen

Das Sicherheitsmodell von 2L1nk basiert auf modernen, bewährten kryptographischen Verfahren, die vollständig clientseitig implementiert sind.

#### Ed25519 – Digitale Signaturen

Ed25519 ist ein Algorithmus zur Erstellung digitaler Signaturen basierend auf elliptischen Kurven (Edwards-Kurve über dem Primkörper 2²⁵⁵-19). Jeder Benutzer generiert beim ersten Start ein Ed25519-Schlüsselpaar:

- **Private Key:** Wird nur im Browser gespeichert (nie an den Server gesendet)
- **Public Key:** Wird an den Server übertragen
- **Fingerprint:** `SHA-256(Ed25519 Public Key)` dient als eindeutige Benutzer-ID im gesamten System

Alle API-Anfragen werden mit dem privaten Schlüssel signiert, damit der Server die Identität verifizieren kann, ohne ein Passwort zu kennen.

#### X25519 – Schlüsselaustausch (ECDH)

X25519 ist ein Diffie-Hellman-Schlüsselaustauschprotokoll auf Basis elliptischer Kurven. Es ermöglicht zwei Parteien, einen gemeinsamen geheimen Schlüssel zu vereinbaren, ohne ihn jemals direkt zu übertragen.

In 2L1nk wird X25519 verwendet, um den symmetrischen Raumschlüssel für jeden Teilnehmer individuell zu verschlüsseln (`room_key_slots`). So kann jeder Raumteilnehmer seinen Schlüssel entschlüsseln, ohne dass der Server den Raumschlüssel kennt.

#### NaCl SecretBox – Symmetrische Verschlüsselung

Nachrichten werden mit NaCl SecretBox verschlüsselt, einer Kombination aus:
- **XSalsa20** (Stream-Cipher) zur Verschlüsselung
- **Poly1305** (Message Authentication Code) zur Integritätsprüfung

Das Ergebnis ist ein authentifiziertes Chiffrat: Manipulation wird erkannt, der Server sieht nur zufällige Bytes.

#### Epoch-basierte Schlüsselrotation

Jeder Raum hat eine aktuelle **Epoch** (Versionsnummer des Raumschlüssels). Bei jedem Beitreten oder Verlassen eines Benutzers wird:

1. Die Epoch um 1 erhöht
2. Ein neuer zufälliger Raumschlüssel generiert
3. Dieser Schlüssel für alle aktuellen Mitglieder individuell mit X25519 verschlüsselt
4. Die neuen `room_key_slots` an den Server übermittelt

Dadurch können ausgeschlossene Benutzer vergangene Nachrichten nicht mehr entschlüsseln (Forward Secrecy).

---

### 1.4 Datenbankdesign

Als Datenbank wird **SQLite** eingesetzt – eine eingebettete, dateibasierte SQL-Datenbank. SQLite benötigt keinen separaten Datenbankserver und eignet sich ideal für selbstgehostete Anwendungen.

Die Migrationen laufen automatisch beim Serverstart (`internal/db/migrations.go`).

#### Tabellen

**`users`** – Registrierte Benutzeridentitäten
| Spalte | Typ | Beschreibung |
|---|---|---|
| `fingerprint` | TEXT (PK) | SHA-256 des Ed25519 Public Keys |
| `public_key` | TEXT | Ed25519 Public Key (Base64) |
| `x25519_public_key` | TEXT | X25519 Public Key (Base64) |
| `username` | TEXT | Optionaler Anzeigename |
| `created_at` | INTEGER | Unix-Timestamp |

**`rooms`** – Chaträume
| Spalte | Typ | Beschreibung |
|---|---|---|
| `id` | TEXT (PK) | UUID |
| `name` | TEXT | Name des Raums |
| `current_epoch` | INTEGER | Aktuelle Schlüssel-Epoch |
| `host_fp` | TEXT | Fingerprint des Erstellers (kein FK) |
| `key_creator_fp` | TEXT | Fingerprint des aktuellen Schlüsselerstellers (kein FK) |

**`room_members`** – Persistente Raumteilnehmer
| Spalte | Typ | Beschreibung |
|---|---|---|
| `room_id` | TEXT | Raum-UUID |
| `member_fp` | TEXT | Fingerprint des Mitglieds |
| `joined_at` | INTEGER | Beitrittszeitpunkt |

**`messages`** – Verschlüsselte Nachrichten
| Spalte | Typ | Beschreibung |
|---|---|---|
| `id` | TEXT (PK) | UUID |
| `room_id` | TEXT (FK) | Zugehöriger Raum |
| `sender_fp` | TEXT | Fingerprint des Senders (**kein FK** – erlaubt ephemere Sender) |
| `epoch` | INTEGER | Epoch des verwendeten Raumschlüssels |
| `type` | INTEGER | 0=Text, 1=System, 2=Signal/WebRTC |
| `ciphertext` | BLOB | Verschlüsselter Nachrichteninhalt |

**`room_key_slots`** – Verschlüsselte Raumschlüssel pro Mitglied
| Spalte | Typ | Beschreibung |
|---|---|---|
| `room_id` + `epoch` + `recipient_fp` | Composite PK | Eindeutiger Schlüssel pro Mitglied und Epoch |
| `encrypted_key` | BLOB | Raumschlüssel, verschlüsselt mit X25519 des Empfängers |

**`gate_tokens`** – Zugangstokens
| Spalte | Typ | Beschreibung |
|---|---|---|
| `token` | TEXT (UNIQUE) | 64-Zeichen Hex-String |
| `max_uses` | INTEGER | Maximale Verwendungsanzahl (0 = unbegrenzt) |
| `use_count` | INTEGER | Bisherige Verwendungsanzahl |
| `is_active` | INTEGER | Aktiv-Flag |

**`voice_sessions`** / **`voice_participants`** – Für zukünftige Voice-Call-Funktion (Schema bereits angelegt, Logik noch nicht implementiert)

#### Designentscheidungen

- `messages.sender_fp` hat **keinen Fremdschlüssel** auf `users`: Ephemere Benutzer werden nicht in der Datenbank gespeichert, können aber trotzdem Nachrichten senden.
- Der Server speichert **ausschließlich Chiffretexte** – niemals Klartextnachrichten oder Klartextschlüssel.

---

### 1.5 Sicherheitsmodell

#### Gate-Token (Zugangskontrolle)

Der Server ist nicht öffentlich zugänglich. Jede neue Verbindung muss zunächst einen **Gate-Token** vorweisen – einen 64-stelligen zufälligen Hex-String, der beim Serverstart generiert und in der Konsole ausgegeben wird. Nur wer diesen Token kennt, kann sich registrieren.

#### Session-Authentifizierung

Nach erfolgreicher Gate-Authentifizierung erhält der Client eine **Session-ID**, die im In-Memory-Session-Store des Servers hinterlegt wird. Alle weiteren API-Anfragen verwenden diese Session-ID.

#### Request-Signing

Jede API-Anfrage wird mit dem privaten Ed25519-Schlüssel des Nutzers signiert. Das kanonische Format der Signatur ist:

```
HTTP-Methode + "\n" + Pfad + "\n" + Timestamp + "\n" + SHA-256(Request-Body)
```

Dadurch kann der Server sicherstellen, dass die Anfrage tatsächlich vom Besitzer des jeweiligen Schlüssels stammt und nicht manipuliert wurde.

#### WebSocket-Authentifizierung

Bei WebSocket-Verbindungen sendet der Client als erste Nachricht ein `type: "auth"`-Paket mit Session-ID und Signatur. Der Server validiert die Session und registriert den Benutzer im Hub.

---

## Teil 2 – Praktischer Teil

### 2.1 Architektur-Überblick

Das Backend folgt einer klar geschichteten Architektur:

```
cmd/2L1nk/main.go
    └── internal/app/app.go          (Composition Root / Dependency Injection)
            ├── internal/server/      (HTTP-Server, Echo)
            ├── internal/api/         (Routes, Handler, Middleware)
            │       └── handlers/     (ein Handler pro Ressource)
            ├── internal/service/     (Business-Logik, kein DB/HTTP-Wissen)
            ├── internal/infrastructure/db/  (SQL-Repositories)
            ├── internal/hub/hub.go   (WebSocket Event Loop)
            └── internal/app/event_consumer.go  (Hub → DB Persistenz)
```

**Wichtige Architekturprinzipien:**

- **Dependency Injection:** Alle Abhängigkeiten werden in `app.go` zusammengesteckt und als `service.Container` an Handler weitergegeben.
- **Repository Pattern:** Datenbankzugriffe sind in Repositories abstrahiert (`internal/infrastructure/db/`), Services kennen keine SQL-Details.
- **Trennung Hub / DB:** Services dürfen nicht direkt vom Hub abhängen. DB-Persistenz läuft über `event_consumer.go`, der asynchron die `hub.Events`-Queue liest.

---

### 2.2 Hub-Architektur (WebSocket Event Loop)

Der Hub (`internal/hub/hub.go`) ist das Herzstück der Echtzeit-Kommunikation. Er läuft als **einzelne Goroutine** und verarbeitet alle WebSocket-Zustandsänderungen über typisierte Go-Channels:

```
RegisterUser / UnregisterUser      → Nutzer verbinden / trennen
JoinRoom / AddToRoom / RemoveFromRoom → Raumverwaltung
InboundMessages / Broadcast        → Nachrichtenverteilung
LoadRoomAndDeliver                 → Raumhistorie laden und ausliefern
EpochKeysSubmitted                 → Neue Epoch-Schlüssel empfangen
SendErrorToUser                    → Fehler an Client senden
```

Da alle Zustandsänderungen sequentiell durch diese eine Goroutine laufen, sind keine Locks für den Hub-internen Zustand nötig – Race Conditions sind strukturell ausgeschlossen.

Der `EventConsumer` läuft parallel und liest aus `hub.Events`, um Ereignisse (z.B. neue Nachrichten) in die Datenbank zu schreiben, ohne den Hub-Loop zu blockieren.

---

### 2.3 Backend – Implementierungsdetails

#### HTTP-API (Echo Framework)

Das Echo-Framework stellt folgende Middleware-Schicht bereit:

- **Rate Limiting:** Maximal 100 Requests pro Store
- **Timeout:** 10 Sekunden pro Request
- **Security Headers:** Automatische Sicherheits-Header (XSS-Schutz, Content-Type-Options etc.)
- **Request-ID-Tracking:** Jeder Request erhält eine eindeutige ID für das Logging

#### Wichtige API-Endpunkte

| Endpunkt | Methode | Beschreibung |
|---|---|---|
| `/api/health` | GET | Serverstatus und Zeitstempel |
| `/api/auth/gate` | POST | Gate-Authentifizierung → Session-ID |
| `/api/ws` | GET (Upgrade) | WebSocket-Verbindung |
| `/api/rooms` | POST | Neuen Raum erstellen |
| `/api/rooms/:id/epoch-keys` | POST | Neue Epoch-Schlüssel einreichen |
| `/api/rooms/:id/messages` | GET | Verschlüsselte Nachrichten abrufen |
| `/api/rooms/:id/key-slots` | GET | Verschlüsselte Raumschlüssel abrufen |

#### Service-Container-Pattern

Alle Handler erhalten einen `service.Container` (nicht einzelne Services):

```go
type Container struct {
    Health  *HealthService
    Gate    *GateService
    Room    *RoomService
    Message *MessageService
}
```

Um einen neuen Service hinzuzufügen: Service in Container eintragen und in `app.go` verdrahten.

#### Fehlerbehandlung

Services geben typisierte Fehler zurück. Handler übersetzen diese in HTTP-Statuscodes. Es gilt die Regel: **nicht gleichzeitig loggen und zurückgeben** – entweder wird ein Fehler geloggt oder zurückgegeben, nicht beides.

---

### 2.4 Frontend – JavaScript-Implementierung

Das Frontend ist in Vanilla JavaScript (ohne Framework) implementiert und wird über `go:embed` direkt in die Binärdatei eingebettet. Dadurch ist keine separate Webserver-Infrastruktur nötig.

#### AppCrypto-Modul (`web/js/mainsite.js`)

Das AppCrypto-Modul kapselt alle kryptographischen Operationen:

| Funktion | Beschreibung |
|---|---|
| `generateIdentity()` | Erstellt Ed25519-Schlüsselpaar, speichert in sessionStorage/localStorage |
| `sign(data)` | Ed25519-Signatur über beliebige Daten |
| `verify(data, sig, pubKey)` | Signaturverifikation |
| `generateRoomKey()` | 32 Byte zufälliger symmetrischer Schlüssel |
| `encryptRoomKey(roomKey, recipientDHPub)` | Raumschlüssel mit X25519 für Empfänger verschlüsseln |
| `decryptRoomKey(...)` | Raumschlüssel entschlüsseln |
| `encryptMessage(plaintext, roomKey)` | Nachricht mit NaCl SecretBox verschlüsseln |
| `decryptMessage({nonce, ciphertext}, roomKey)` | Nachricht entschlüsseln |

#### Schlüsselspeicherung

- **Ephemere Nutzer:** Schlüssel in `sessionStorage` (gehen beim Tab-Schließen verloren)
- **Persistente Nutzer:** Schlüssel in `localStorage` (bleiben über Sessions erhalten, werden auch in `users`-Tabelle gespeichert)

#### Einstellungen

Das Frontend speichert Benutzereinstellungen (Theme, Schriftgröße, Benachrichtigungen etc.) in `localStorage`. Es stehen 6 Farbthemen zur Auswahl: Lila, Blau, Grün, Rot, Orange, Cyan.

---

### 2.5 Terminal-Benutzeroberfläche (TUI)

Die TUI ist mit **BubbleTea** implementiert, das dem Elm-Architekturmuster folgt (Model / Update / View):

- **Model:** Gesamter Zustand der Anwendung
- **Update:** Reine Funktion, die auf Events reagiert und neuen State zurückgibt
- **View:** Reine Funktion, die den State als String rendert

#### Verfügbare Menüpunkte

| Menü | Funktion |
|---|---|
| Run Server | Startet den Server als Subprozess mit Logging |
| Stop Server | Beendet den Server geordnet |
| Gate Key | Zeigt aktuellen Token, Nutzungsstatistiken, History; ermöglicht Rotation |
| View Logs | Echtzeit-Logansicht |
| Outbound Tunnels | Tunnel konfigurieren (z.B. für externe Erreichbarkeit) |
| Reset Database | Löscht alle Daten sicher |
| Options | Port, DB-Pfad und weitere Einstellungen |
| Nuke | Vollständige Datenlöschung mit Bestätigung |

#### Betriebsmodi

```bash
./2L1nk                  # TUI (Standard) – interaktive Serververwaltung
./2L1nk --server         # Headless-Server – läuft ohne UI, ideal für Produktion
./2L1nk --tempserver     # Temporärer Server – ephemere DB, automatisch aufgeräumt
```

---

### 2.6 Build & Deployment

#### Build-Befehle

```bash
make build          # Linux + Windows Binaries → bin/linux/ und bin/windows/
make build-static   # Statisch gelinkte Binaries (kein CGO, ideal für Raspberry Pi)
make test           # Alle Tests mit Race Detector
make fmt            # Go-Dateien formatieren
make lint           # go vet ausführen
```

#### Einzelnen Test ausführen

```bash
cd Projekt && go test ./internal/service/... -run TestFunctionName -v
```

#### Umgebungsvariablen

| Variable | Standard | Beschreibung |
|---|---|---|
| `PORT` | `8080` | HTTP-Port des Servers |
| `DB_PATH` | `2L1nk.db` | Pfad zur SQLite-Datenbankdatei |

#### Deployment

Da das Frontend via `go:embed` in die Binärdatei eingebettet ist, besteht das gesamte Deployment aus einer einzigen ausführbaren Datei. Kein Webserver, kein Node.js, keine externe Datenbank nötig.

---

### 2.7 Bekannte offene Punkte

| Punkt | Beschreibung |
|---|---|
| Request-Signaturvalidierung | Ist im Code vorbereitet, aber noch nicht serverseitig durchgesetzt (`ws.go` ~Zeile 81) |
| Voice Calls | Datenbankschema (`voice_sessions`, `voice_participants`) ist bereits angelegt; die WebRTC-Logik ist noch nicht implementiert |
