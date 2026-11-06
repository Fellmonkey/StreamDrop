---
name: StreamDrop
description: Operate the StreamDrop desktop app — streaming files from URLs straight into cloud storage. Use when managing transfers, the queue, cloud connections (remotes), components, or checking app status and updates through its MCP server.
---

# StreamDrop

StreamDrop is a local desktop service (Windows 10/11 x64 reference) that streams files from HTTP(S) URLs directly into a cloud (Mail.ru, Yandex.Disk, Google Drive, OneDrive, S3, WebDAV, FTP, SFTP) without writing them to disk. The UI runs at `http://127.0.0.1:<port>`; the daemon lives in the system tray.

Your access point is the **MCP server**: run `streamdrop.exe mcp` (or the standalone `streamdrop-mcp.exe`) — 15 typed tools over stdio. If MCP is not configured, the same operations are available via the REST API at `/api/v1/*` with the `X-StreamDrop-Token` header from `<dataDir>/run/daemon.json`.

## Prerequisites

- The **daemon must be running** for any tool to succeed. If it is not, every tool returns `daemon_unreachable` — start StreamDrop first and retry.
- Default data dir (Windows): `%LOCALAPPDATA%\StreamDrop`. The MCP server resolves it automatically from its own location; use `--data-dir <path>` for non-default layouts.
- The engine needs the **rclone** component for cloud uploads. Check `get_status` → `engine_ready`; if false, install components (see “Self-healing” below).

## Tools (15)

| Tool | Purpose |
|---|---|
| `get_status` | daemon health: version, engine_ready, active transfers, components |
| `list_remotes` | connected clouds + status + quota |
| `list_transfers` | transfers, optional `status` filter (`queued\|running\|paused\|retrying\|completed\|failed\|aborted`) |
| `get_transfer` | transfer details + recent logs |
| `create_transfers` | create one or more transfers (atomic batch); required `remote_id` |
| `pause_transfer` / `resume_transfer` | pause/resume a transfer |
| `abort_transfer` / `delete_transfer` | abort; delete finished from history (destructive) |
| `get_queue` | queue state: paused, parallel_limit, entries |
| `pause_queue` / `resume_queue` | pause/resume the whole queue |
| `list_dependencies` | components (rclone, yt-dlp, curl) + status |
| `install_dependency` | install a component in background (poll `list_dependencies`) |
| `check_updates` | check for a newer StreamDrop release |

## Invariants

- **`create_transfers` inspects every URL itself** and is atomic: if any URL fails, nothing is queued. There is no separate inspect tool.
- **`remote_id` is required** for transfers — get it from `list_remotes`. `remote_path` is the destination folder.
- **Tool calls run concurrently.** Await each result before issuing another state-changing call (e.g. `pause_queue` then `resume_queue`).
- **Secrets are never returned** (remote credentials, cookies, tokens). Never ask the user for them; configuration happens in the UI.
- Destructive tools (`abort_transfer`, `delete_transfer`) are marked destructive — confirm with the user before calling.

## Workflows

### 1. Health check
1. `get_status`. If `engine_ready` is false → `list_dependencies`; install missing required components (`install_dependency`, id `rclone`), then poll `list_dependencies` until status is `installed`.

### 2. Create a transfer
1. `get_status` (health).
2. `list_remotes` → pick the target cloud `id`.
3. `create_transfers` → `{"remote_id": "...", "source_url": "https://..."}` (or `urls` batch, `remote_path`, `engine`, `mode`, `buffer_size_bytes`, ...).
4. Track via `get_queue` / `get_transfer`.

### 3. Diagnose a failed transfer
1. `get_transfer` (includes logs). Look at `status`, `error_kind`, `error_message`.
2. Common recovery: `abort_transfer` + recreate with `auth_profile_id`; or fix the remote (`list_remotes` status) and retry.

### 4. Self-healing components
- `list_dependencies` shows `missing|corrupted|outdated`. `install_dependency` runs in background — poll `list_dependencies` until `installed`.

### 5. Updates
- `check_updates` → if `available: true`, tell the user to apply the update in the UI (the `apply_update` tool is intentionally not exposed; it is destructive and human-confirmed).

## Error codes

| Code | Meaning / action |
|---|---|
| `daemon_unreachable` | daemon not running; start StreamDrop |
| `source_unreachable` / `source_interrupted` | source URL problem; verify URL, retry |
| `validation_failed` | missing/invalid argument (check schema) |
| `remote_not_found` | bad `remote_id`; re-list remotes |
| `not_found` | transfer/entity does not exist |
| `dependency_missing` | component absent; install it |
| `cloud_unreachable` | target cloud problem; check `list_remotes` |
| `auth_required` / `auth_rejected` | source needs authorization profile (`auth_profile_id`) |

Errors are machine-readable: `{"code": "...", "kind": ..., "http_status": ..., "params": {...}}`.
