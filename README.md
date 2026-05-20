# [revshells.io](https://revshells.io)

Callback/session broker for authorized pentests, CTF, or homelab. Targets call back over HTTPS, operators attach from browser or CLI.


## Build

CI builds static **rsd**, **rsctl**, and **rs-agent** for:

| Label | GOOS / GOARCH |
|-------|----------------|
| linux-amd64 (x86_64) | linux / amd64 |
| linux-arm64 (aarch64) | linux / arm64 |
| linux-386 (x86) | linux / 386 |
| darwin-amd64 (x86_64) | darwin / amd64 |
| darwin-arm64 (aarch64) | darwin / arm64 |

```
make all          # rsd, rsctl, and cross-compiled rs-agent binaries
# or
go build -o rsd ./cmd/rsd
go build -o rsctl ./cmd/rsctl
make agents       # required: agents-bin/linux-amd64, linux-arm64, ...
```

## Run

```
./rsd --listen :8080 --public-url https://rs.example.com --db rsd.db --agents-dir agents-bin
```

`--agents-dir` must contain `rs-agent` builds from `make agents`. Callback URLs use the request `Host` (so Docker targets can use `host.docker.internal:8080` without editing the script).

`--max-sessions-per-workspace` (default `12`) limits how many active sessions each workspace can create.

Put Caddy/nginx in front for TLS:

```
rs.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## CLI

```bash
# Mint token from http://localhost:8080/ (rsctl panel), then:
rsctl login http://localhost:8080 <workspace-cli-token>

# List sessions
rsctl list

# Attach (raw terminal over WebSocket)
rsctl attach <session-id>

# Kill session
rsctl kill <session-id>

```

## Browser UI

Open `http://localhost:8080/` for the **sessions hub**:

- **New session** creates another UUID (multiple sessions in localStorage)
- **rsctl** — mint workspace CLI token (24h, scoped to sessions you create in this browser)
- Session list shows target info when connected: user@host, OS, kernel
- **Light/dark mode**, disclaimer modal

```bash
# paste from browser hub
rsctl login http://localhost:8080 <token>
rsctl list
rsctl attach <session-id>
```

## Modes

| Mode | Transport | PTY | How |
|------|-----------|-----|-----|
| Interactive PTY | WebSocket | yes | Target runs Go/Python agent with PTY |
| Command channel | HTTP poll | no | Target polls for commands, posts results |
| One-shot event | HTTP POST | no | Single POST, no live session |

The system auto-detects based on what the target reports during registration.

## Sessions

- 6 hour inactivity TTL (any interaction resets the timer)
- Multiple targets can call back to the same session UUID
- Multiple operators can attach simultaneously

## API

```
POST   /api/sessions              create session (bearer auth)
GET    /api/sessions              list sessions (bearer auth)
GET    /api/sessions/:id          session detail (bearer auth)
DELETE /api/sessions/:id          kill session (bearer auth)

POST   /s/:id/:secret/register   target registers capabilities
GET    /s/:id/:secret/poll       long-poll (30s block)
POST   /s/:id/:secret/push       target posts output
POST   /s/:id/:secret/event      one-shot event
WS     /s/:id/:secret/connect    target WebSocket (PTY or command)
WS     /s/:id/attach             operator WebSocket (bearer or cookie auth)
GET    /s/:id/:secret/sh         bash shim
GET    /s/:id/:secret/py         python shim
```

## Storage

BBolt (single file, `rsd.db`). No external dependencies.

Authorized security testing and lab use only.
