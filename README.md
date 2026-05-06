# 2L1nk

Self-hosted, single-binary encrypted chat and voice communication. The server never decrypts anything — all cryptographic operations happen in the browser.

No accounts, no cloud dependency, no plaintext stored.

---

## Install

Download the binary for your platform from the [Releases page](../../releases). Binaries are available for Linux, Windows, and macOS on both x86-64 and ARM64.

On Linux/macOS, make it executable after downloading:

```bash
chmod +x 2L1nk-linux-x86-64
```

---

## Running

Three modes are available:

| Mode | Command | Description |
|------|---------|-------------|
| TUI (default) | `./2L1nk` | Interactive terminal UI — manage the server, gate, tunnels, and logs |
| Server | `./2L1nk --server` | Run the server directly in the foreground (headless) |
| Temp server | `./2L1nk --tempserver` | Like `--server`, but securely wipes the database on exit |

```bash
# Start with the TUI (recommended)
./2L1nk

# Run headless
./2L1nk --server

# Ephemeral session — all data deleted on exit
./2L1nk --tempserver
```

---

## TUI

Running `./2L1nk` without flags opens the interactive control panel. Navigate with `↑↓` or `j/k`, confirm with `Enter`, go back with `Esc` or `q`.

| Item | Description |
|------|-------------|
| **Run / Stop Server** | Start or stop the chat server as a background process |
| **Gate Key** | Manage the access token (see below) |
| **View Logs** | Stream the live server log |
| **Outbound Tunnels** | Configure tunnels that expose your server to the internet (see below) |
| **Links** | Local network URL, public IP, and live tunnel URLs — select to copy or view a QR code |
| **Reset Database** | Stop the server and wipe all accounts, rooms, and messages |
| **Options** | Toggle `No Logs` (disable log file) and `Temp Server` (wipe DB on stop) |
| **Nuke** | Irreversibly overwrite and delete the database, logs, PID, options, and all tunnel data |

For a full walkthrough see the in-app [Setup Guide](web/pages/Setup.html).

---

## Gate Key

The gate key is an access token that controls who can reach the web interface. It is not used for encryption — think of it as an invite code.

On first run a random 64-character hex key is generated and stored in the database. Users present it once to register; after that they hold a session valid for 24 hours.

From the TUI **Gate Key** screen you can:

- View the current key and its use count
- Set a **max-uses** cap — the key auto-rotates once the limit is reached (useful for single-use invites)
- Rotate to a new random key or set a custom one
- Browse the full history of past keys

Changes take effect immediately without restarting the server.

---

## Tunnels

Tunnels expose your local server to the internet without port-forwarding. Managed from the TUI **Outbound Tunnels** menu — add, start, stop, delete, and tail live logs per tunnel. The detected public URL is shown as soon as the tunnel is up, and each tunnel can be set to auto-start alongside the server.

Presets are included for Cloudflare and several SSH-based services (e.g. Cloudflare: `cloudflared tunnel --url http://localhost:{PORT}`), plus a **Custom** option for any shell command with an optional `{PORT}` placeholder.

Tunnel configurations are persisted alongside the database.

---

## Security

- Messages are encrypted with **AES-256-GCM** using per-room keys that rotate on every join and leave.
- User identity is an **Ed25519** key pair generated in the browser. The private key never leaves the client.
- Room keys are distributed via **X25519** key exchange — the server only forwards encrypted key blobs it cannot read.
- The server stores only ciphertexts. A full database leak exposes no plaintext content.
- Gate authentication uses Ed25519 signatures with a 30-second timestamp window and per-signature nonce tracking to prevent replay attacks.
