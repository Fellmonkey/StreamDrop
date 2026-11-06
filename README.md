# StreamDrop Desktop

**Your helper for moving files between the internet and clouds** — without writing to disk and without extra servers.

**Download:** [StreamDrop v1.0.0 — Windows x64](https://github.com/Fellmonkey/StreamDrop/releases/latest) (single portable exe, no installer)

The stream goes `HTTP → RAM → rclone rcat`: large files travel from the web straight into cloud storage, bypassing the drive. The UI opens in a browser at `127.0.0.1`; closing the tab doesn't interrupt transfers — the service keeps running in the tray.

> Русская версия: [`README.ru.md`](README.ru.md)

[screen-capture (26).webm](https://github.com/user-attachments/assets/c60951db-2987-401d-8d75-b5501bcc86fe)

## For whom and why

This is a **personal tool**, not an enterprise platform: your files, your home network, your computer. StreamDrop solves one task — “move a file where it's needed without occupying disk space and without intermediate steps”.

The classic flow is: download to disk → wait → upload to cloud. StreamDrop does this **in one pass**:

- data flows through RAM — **the drive doesn't wear out** and doesn't fill up;
- **no free space needed** for a temporary file, even for a multi-gigabyte ISO;
- parallelism and the queue make transfer **in batches** convenient — not only for big files, but also when there are many of them;
- everything is controlled from one window; status and speed are visible even in the tray.

Honest caveats:

- Speed is still limited by your internet channel and the cloud provider.
- Time savings matter where the disk is slower than the channel; with a fast NVMe, the benefit is mainly wear and convenience.
- It is not a universal downloader: direct file links, HTML pages (via yt-dlp) and a limited set of protocols are supported.

**The idea is broader than “download to cloud”.** The project's foundation is rclone, which natively moves data between any pair of storages: cloud → cloud, local disk → cloud, and so on. StreamDrop unlocks this gradually: today the internet → cloud scenario is available; the next steps are “another cloud” and “local folder” sources, so files can be moved without making the PC an intermediate (see roadmap).

## Features

- Pre-flight inspection: redirects, HEAD → Range-GET, filename/size/type, HTML detection.
- Two modes: `ring_buffer` (32–1024 MiB) and `pure_stream`; engines `curl` / `yt-dlp` / built-in.
- Queue: parallelism 1–5, manual ordering, pause, retry, `Range` resume, on-the-fly SHA-256.
- Clouds: Mail.ru (WebDAV), Yandex.Disk, Google Drive, OneDrive, S3, WebDAV, FTP, SFTP; explorer, quotas, folder creation. OAuth on `127.0.0.1:35357`.
- Source authorization profiles: browser cookies (Windows), `cookies.txt` import, User-Agent/Referer.
- Components (rclone / yt-dlp / curl) install automatically with SHA-256 verification.
- Notifications: toasts + Telegram.
- Self-update from GitHub Releases.
- “Saved” metrics: bytes never written to disk; the time a disk round-trip would have taken; rewrite capacity.
- ru/en localization.

## Quick start (Windows)

```bash
make build-windows            # web/dist + bin/streamdrop.exe
./bin/streamdrop.exe          # opens http://127.0.0.1:8085
```

If the port is busy, the next free one is used; the address is logged. Components are downloaded on first run.

## Build

Requirements: Go 1.27, Node.js 22+, npm.

```bash
cd web && npm ci && npm run build && cd ..
go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/streamdrop.exe ./cmd/streamdrop
```

The frontend must be built before `go build` (`web/embed.go`). Icon: `cmd/streamdrop/rsrc_windows_amd64.syso`.

## Configuration

| Flag | Default | Purpose |
| :--- | :--- | :--- |
| `--port` | `8085` | UI port |
| `--host` | `127.0.0.1` | loopback only |
| `--data-dir` | `%LOCALAPPDATA%\StreamDrop` | DB, logs, components |
| `--no-tray` | off | no tray icon |
| `--no-browser` | off | do not open the browser |
| `--update-repo` | `Fellmonkey/StreamDrop` | GitHub owner/repo |

Data: `streamdrop.db`, `logs\`, `deps\`, `crash\`, `update\staging\`. The daemon also writes `run\daemon.json` (port, per-run token, pid) — the discovery record used by external clients; it is removed on shutdown.

## AI integration (MCP)

`streamdrop.exe` itself can serve as an MCP server for agents (Claude Desktop, OpenCode, Cursor) over stdio — no extra downloads, no second binary. It exposes 15 typed tools (status, clouds, transfers, queue, components, updates). The token and port are read from `run\daemon.json` on every call, so a daemon restart needs no reconfiguration.

Configure your MCP client to run:

```
C:\Users\<you>\Downloads\StreamDrop\streamdrop.exe mcp
```

Example client configs (replace `<you>` and the path): `examples/mcp-claude.json`, `examples/mcp-opencode.json`. If your data directory is not the default, append `--data-dir <path>` to the arguments.

### Agent skill (portable)

`skills/streamdrop/SKILL.md` is a portable agent skill: it teaches an agent the domain rules (invariants, workflows, error codes) on top of the MCP tools. Wire it into your harness:

- **OpenCode:** copy it to `.opencode/skills/streamdrop/SKILL.md`, or add `"skills": ["./skills"]` to `opencode.json` so OpenCode discovers the whole `skills/` catalog.
- **Claude Code:** copy it to `.claude/skills/streamdrop/SKILL.md`.
- **Any other agent:** attach `skills/streamdrop/SKILL.md` as an instruction document.

- Tools return machine-readable error codes (`source_unreachable`, `daemon_unreachable`, `validation_failed`, ...); the daemon must be running for any tool to succeed.
- MCP is optional: if you do not use AI agents, ignore the `mcp` subcommand entirely. MCP servers never start on their own — they only run when an MCP client spawns them.

## Security

Loopback only; REST protected by a per-run token and Origin/Host checks; secrets encrypted (DPAPI / AES-256-GCM); no telemetry; the API never returns secrets — not in logs, not in responses.

## Limitations (honest)

- **Reference platform is Windows 10/11 x64**; macOS/Linux build but are not reference releases.
- **Google Drive / OneDrive** require your own OAuth app (instructions inside the wizard); **Yandex.Disk** uses a verification-code flow.
- Chromium 127+ (App-Bound Encryption) cannot be read by a third-party process — use `cookies.txt`.
- No resume after process death and no recursive cloud-wide search.
- Updates are verified by SHA-256; signature only when a key is configured.

## Roadmap

Priorities, no dates promised:

1. **Full Linux/macOS support** — reference builds, installers, self-update.
2. **Sources: another cloud and a local folder** — cloud → cloud, disk → cloud, and back. The mechanics already exist in rclone (remote-to-remote); only source wiring in the UI is needed.
3. **Torrent / magnet sources** — streaming without full download.
4. **Telegram** — queue control from chat, per-file events, status on demand.
5. **Schedules (cron)** — “grab Friday's release”.
6. **WebDAV server as a destination** — receive files from other tools.
7. **Recursive search and filters** across the cloud.
8. **Proxy/VPN modes** for sources.

Deliberately not planned: telemetry, browser push notifications, mobile clients.

## Tests

```bash
go test ./...
go vet ./...
cd web && npm ci && npm run build
```

Key suites: pipeline with fake `curl`/`rclone` (bytes, SHA-256, no files on disk), PKCE/sessions, LocalGuard, wear metrics, e2e against Mail.ru, Playwright UI scenarios.

## Release status

v1 functionality is implemented and covered by tests. For the first public release, only this remains: a `v*` tag, CI building of the release archive, and publishing to GitHub Releases.

## Layout

```
cmd/streamdrop      service entry point
cmd/depcheck        component integrity check
internal/app        container, port, tray, graceful shutdown
internal/engine     buffer, inspector, pipeline
internal/transfer   tasks, retry, metrics
internal/queue      scheduler
internal/rclone     RC client, daemon, explorer, provisioner
internal/deps       component catalog
internal/auth       cookies, browser profiles
internal/oauth      OAuth sessions
internal/notify     toast + Telegram
internal/update     updates
internal/ws         WebSocket hub
internal/server     REST + embedded static
web                 React SPA (Vite, Tailwind, i18n ru/en)
```
