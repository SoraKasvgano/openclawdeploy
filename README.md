[英文](README.md)      [简体中文](README_ZH.md)

# OpenClaw Deploy

OpenClaw Deploy is a two-binary Go project for managing and deploying OpenClaw nodes.

- `server`: user management, device bind/filter/delete, remote `openclaw.json` delivery, AI-token API access, Swagger UI, SMTP settings, and registration control
- `client`: local web console, local login protection, device identity generation, local `openclaw.json` editing, server communication status display, heartbeat sync, and an auto-generated deploy script

Both web UIs are embedded into the binaries. No external CDN is required at runtime.

## Current implementation

Implemented today:

- Single-binary `client` and `server`
- Embedded frontend assets via `embed`
- REST-style HTTP API using the Go standard library
- Server-side register, login, forgot password, reset password
- Admin console, SMTP settings, registration toggle, user management
- User self-service profile update (`email` and `password` only)
- Device bind, remark edit, delete, and remote `openclaw.json` delivery
- Admin device filtering by owner username
- Direct protected API access through AI token
- Swagger UI and OpenAPI JSON
- Client-side local web UI and local API auth
- Default local client credential `admin / admin`, editable by the user
- Client heartbeat polling and remote config apply
- Client UI shows server configuration state and latest communication result
- Server config hot reload, including listener rebind on port/address change
- Compact alphanumeric device IDs with backward compatibility for legacy IDs
- Server persistence moved to no-CGO SQLite
- Automatic startup migration from legacy `data/server-state.json`
- Volatile heartbeat state is kept in memory only and is not written to SQLite

Known gaps compared with the original design:

- The client shows a deterministic identity matrix preview, not a standard QR code
- `clientdeploy.sh` is still Linux-oriented (`bash` and `systemd`)
- Server state storage is single-node SQLite, not a distributed multi-writer design

## Repository layout

```text
.
|-- client/
|   |-- main.go
|   |-- backend/
|   |-- frontend/
|   `-- build.bat
|-- server/
|   |-- main.go
|   |-- backend/
|   |-- frontend/
|   |-- swaggerui/
|   `-- build.bat
|-- internal/
|   `-- shared/
|-- design.md
|-- install.sh
`-- openclaw.json
```

`internal/shared` contains internal helpers reused by both binaries, such as atomic file writes, JSON helpers, hashing, runtime path detection, and device ID normalization.

## Requirements

- Go `1.25.0`
- Linux or macOS for the intended node workflow
- Windows is supported for build and UI smoke testing
- Server-side SQLite uses `modernc.org/sqlite`, so CGO is not required

## Quick start

Run from the repository root:

```bash
go run ./server
```

Open:

- Server UI: `http://127.0.0.1:18080/`
- Swagger UI: `http://127.0.0.1:18080/swagger/`
- OpenAPI JSON: `http://127.0.0.1:18080/openapi.json`

Then run a client:

```bash
go run ./client
```

Open:

- Client UI: `http://127.0.0.1:17896/`

If you use `go run`, runtime files are created in the current working directory because the runtime base-dir logic explicitly avoids the Go temporary build directory.

## Server behavior

### Default listen address

- Listen address: `0.0.0.0:18080`
- UI log hint: `http://127.0.0.1:18080/`

### First start

On first boot, the server creates:

- `serverconfig.json`
- `data/server-state.sqlite`

It also ensures:

- default admin account: `admin / admin`
- a plain-text `ai_token` is written into `serverconfig.json`
- automatic migration from legacy `data/server-state.json` if that file exists

### Hot reload

Editing `serverconfig.json` while the server is running triggers automatic reload.

Supported live changes:

- `web_port`
- `listen_addr`
- `public_base_url`
- `session_ttl_hours`
- `ai_token`
- `smtp`

If `web_port` or `listen_addr` changes, the HTTP listener is rebound automatically.

### State storage

Persistent server state now uses SQLite. The default state database is:

- `data/server-state.sqlite`

Persisted data includes:

- registration setting
- users
- sessions
- password reset tokens
- device ownership, remark, and pending config delivery state

Volatile heartbeat data is intentionally not persisted:

- `last_seen_at`
- `sync_interval_seconds`
- runtime `status`
- client-reported `openclaw_json/openclaw_hash`

This keeps frequent heartbeat writes out of persistent storage.

### External API auth

Protected endpoints accept:

- `X-API-Token: <ai_token>`
- `Authorization: Bearer <ai_token>`
- normal login session cookie / session token

Example:

```bash
curl -H "X-API-Token: <ai_token>" http://127.0.0.1:18080/api/v1/admin/summary
```

### Device management endpoints

Current device operations include:

- `POST /api/v1/devices/bind`
- `PUT /api/v1/devices/{deviceID}/remark`
- `PUT /api/v1/devices/{deviceID}/config`
- `DELETE /api/v1/devices/{deviceID}`

Admins can also filter devices by owner username:

```text
GET /api/v1/devices?owner_username=<keyword>
```

## Client behavior

### Default listen address

- Listen address: `0.0.0.0:17896`
- UI log hint: `http://127.0.0.1:17896/`

### First start

On first boot, the client creates:

- `config.json`
- `clientdeploy.sh`
- the target `openclaw.json` if it does not exist

The client also generates a device ID and prints an ASCII identity matrix in the console log.

### Local auth

The client web UI and client-local APIs now have their own login layer. This is separate from server auth and does not affect heartbeat sync.

Default local credential:

- username: `admin`
- password: `admin`

The user can change these from the client UI.

### Device ID format

Device IDs are normalized to alphanumeric characters only.

Example:

```text
legacy: 233|00:22:5d:a3:46:da|2026-03-11 17:07:15|100.64.0.3
new:    23300225da346da202603111707151006403
```

Compatibility rules:

- new clients generate the compact format
- existing `config.json` values are normalized on startup
- the server treats legacy and normalized IDs as the same device

### OpenClaw config path

Default target path:

- macOS: `~/.openclaw/openclaw.json`
- Linux: `~/.openclaw/openclaw.json`

At runtime the client resolves this as:

```text
filepath.Join(home, ".openclaw", "openclaw.json")
```

### Sync loop

The client heartbeats to:

```text
<server_url>/api/v1/client/heartbeat
```

Default sync interval:

- `30` seconds

Additional behavior:

- if `server_url` is empty, the client skips polling and still works as a local editor/UI
- if `server_url` is provided as a bare host like `127.0.0.1:18080`, the client normalizes it to `http://127.0.0.1:18080`
- the client UI shows whether a server is configured, the latest sync time, and whether the latest sync succeeded

## Config files

### `serverconfig.json`

Typical fields:

```json
{
  "web_port": 18080,
  "listen_addr": "0.0.0.0",
  "public_base_url": "",
  "session_ttl_hours": 72,
  "ai_token": "generated-on-first-start",
  "smtp": {
    "host": "",
    "port": 25,
    "username": "",
    "password": "",
    "from": ""
  }
}
```

### `config.json` (client)

Typical fields:

```json
{
  "device_id": "generated-on-first-start",
  "device_created_at": "2026-03-12 10:00:00",
  "web_username": "admin",
  "web_password": "admin",
  "web_port": 17896,
  "listen_addr": "0.0.0.0",
  "server_url": "http://127.0.0.1:18080",
  "sync_interval_seconds": 30,
  "openclaw_config_path": "/home/user/.openclaw/openclaw.json",
  "allow_remote_reboot": false
}
```

## Building

### Native builds

```bash
go build ./server
go build ./client
```

### Test suite

```bash
go test ./...
```

### Batch build scripts

Current scripts:

- `client/build.bat`
- `server/build.bat`

They currently emit Linux and macOS artifacts into `client/dist` and `server/dist`.

### Manual Windows build examples

Client:

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go build -trimpath -o client/dist/openclaw-client-windows-amd64.exe ./client
```

Server:

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go build -trimpath -o server/dist/openclaw-server-windows-amd64.exe ./server
```

## Embedded UI and routing notes

- frontend assets are embedded into the binaries
- Swagger assets are embedded locally
- no remote CDN dependency is required
- the server uses `net/http.ServeMux`, not Gin
- `/Swagger` and `/swagger` are normalized to the embedded Swagger UI

## Security model

- normal users can only manage devices visible to them
- normal users can delete only their own device records
- normal users can update only their own `email` and `password`
- username is not self-editable
- SMTP settings, registration toggle, user management, and admin summaries require admin privileges
- admins can view all devices and filter them by owner username
- `ai_token` can call protected server APIs without first logging into the web UI
- client-local auth is separate from server auth

## Notes

- `install.sh` exists in the repository root as a reference artifact
- the generated `clientdeploy.sh` currently runs the upstream install command and writes a `systemd` service when available
- BOM-prefixed `serverconfig.json` files are accepted by the current loader
- the SQLite state database is intended for a single local server process, not for multi-writer shared-file deployments
