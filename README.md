# StreamDrop Desktop v1

Локальный десктоп-сервис, который переливает файл из интернета прямо в облако:
поток идёт `HTTP → ОЗУ → rclone rcat`, полезные данные **не пишутся на диск**.
Интерфейс — одностраничное приложение, отдаваемое тем же процессом на
`127.0.0.1`. Закрытие вкладки не прерывает передачу: сервис живёт в трее.

- **Платформа-эталон:** Windows 10/11 x64 (portable ZIP, без консольного окна).
- **Стек:** Go 1.27 (chi, modernc SQLite, goose, systray), React 19 + Vite + Tailwind.
- **Сеть:** только loopback; API защищён одноразовым токеном и проверкой Origin.

## Возможности

- Предполётная инспекция ссылки: редиректы (до 10), HEAD → Range-GET,
  `Content-Disposition`, имя из URL, размер, тип, флаг HTML.
- Два режима: `ring_buffer` (настраиваемый буфер 32–1024 MiB) и `pure_stream`.
- Движки: `curl`, `yt-dlp` и встроенный Go-источник (fallback для `auto`).
- Очередь: лимит параллелизма 1–5, ручной порядок, пауза очереди и задачи.
- Retry с экспоненциальной задержкой; докачка по `Range`, если источник её
  поддерживает, иначе полный перезапуск с очисткой объекта в облаке.
- SHA-256 на лету (без повторного чтения), лимиты скорости, ETA, спарклайны.
- Облака: Mail.ru (WebDAV), Яндекс.Диск, Google Drive, OneDrive, S3, WebDAV,
  FTP, SFTP; проводник по папкам, квоты, создание папок, OAuth на фиксированном
  порту `127.0.0.1:35357`.
- Профили авторизации источника: cookies браузера, импорт Netscape
  `cookies.txt`, ручная строка Cookie + User-Agent/Referer.
- Менеджер компонентов (rclone / yt-dlp / curl) с атомарной установкой и
  проверкой SHA-256.
- Уведомления: системные toast + Telegram (токен шифруется, URL и логины в
  сообщения не попадают).
- Самообновление с GitHub Releases (SHA-256; подпись — если настроен ключ),
  применение при следующем запуске.
- Локальные crash-дампы (включаются в настройках, без секретов).
- Локализация ru/en, форматирование через `Intl`, ошибки по кодам.

## Быстрый старт (Windows)

```bash
make build-windows          # соберёт web/dist и bin/streamdrop.exe
./bin/streamdrop.exe        # откроет http://127.0.0.1:8085
```

Если порт 8085 занят, берётся следующий свободный (8086, 8087, …); фактический
адрес пишется в лог и показывается в интерфейсе.

Portable-режим: положите компоненты рядом с бинарником в
`deps/<id>/<version>/` (`rclone.exe`, `yt-dlp.exe`, `curl.exe`). Такой каталог
имеет приоритет над `%LOCALAPPDATA%\StreamDrop\deps`. Иначе компоненты
скачиваются из интерфейса (кнопка «Компоненты»).

## Сборка

Требования: Go 1.27, Node.js 22+, npm.

```bash
cd web && npm ci && npm run build && cd ..
go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/streamdrop.exe ./cmd/streamdrop
```

Фронтенд обязан собираться **до** `go build`: `web/embed.go` встраивает
`web/dist` через `go:embed`.

Полезные цели `Makefile`: `web`, `build`, `build-windows`, `test`, `vet`,
`depcheck`, `run`, `package`, `clean`.

## Запуск и конфигурация

Приоритет источников: значения по умолчанию → `config.json` → переменные
окружения → флаги.

| Флаг | Env | По умолчанию | Назначение |
| :--- | :--- | :--- | :--- |
| `--port` | `STREAMDROP_PORT` | `8085` | предпочитаемый порт UI |
| `--host` | `STREAMDROP_HOST` | `127.0.0.1` | только loopback |
| `--data-dir` | `STREAMDROP_DATA_DIR` | `%LOCALAPPDATA%\StreamDrop` | БД, логи, компоненты |
| `--config` | — | `<data-dir>/config.json` | JSON-конфигурация |
| `--no-tray` | `STREAMDROP_NO_TRAY=1` | выкл. | без иконки в трее |
| `--no-browser` | — | выкл. | не открывать браузер |
| `--update-repo` | `STREAMDROP_UPDATE_REPO` | `streamdrop/streamdrop` | GitHub owner/repo |
| `--log-level` | `STREAMDROP_LOG_LEVEL` | `info` | debug/info/warn/error |
| `--version` | — | — | печать версии |

`STREAMDROP_CONSOLE_LOG=1` дублирует лог в stdout (удобно при отладке; в
релизной сборке консоли нет). Ключи `config.json`: `port`, `host`, `data_dir`,
`no_tray`, `open_browser`, `update_repo`, `log_level`, `oauth_callback_port`.

### Где лежат данные

`%LOCALAPPDATA%\StreamDrop\`:

- `streamdrop.db` — SQLite (настройки, история, подключения, токены);
- `logs\` — журнал с ротацией (10 MB × 5);
- `deps\<id>\<version>\` — установленные компоненты;
- `crash\` — локальные пакеты диагностики;
- `update\staging\` — скачанные обновления.

## Безопасность

- Сервер слушает только `127.0.0.1` (валидация хоста).
- Токен создаётся при старте, встраивается в `index.html` как
  `<meta name="streamdrop-token">`; REST требует `X-StreamDrop-Token`,
  WebSocket — `?token=`.
- `LocalGuard` проверяет Host, Origin и `Sec-Fetch-Site`; CORS без wildcard.
- Секреты (пароли облаков, cookies, OAuth-токены, Telegram) шифруются:
  DPAPI на Windows, AES-256-GCM с ключом от machine-id на POSIX.
- API никогда не возвращает секреты: только маски и признаки наличия.
- Телеметрии нет; crash-дампы остаются локальными и не содержат секретов.
- Telegram-сообщения не включают URL источников и логины.

## Компоненты

| Компонент | Версия | Лицензия | Роль |
| :--- | :--- | :--- | :--- |
| rclone | v1.68.2 | MIT | загрузка (rcat), RC-демон, проводник, квоты |
| yt-dlp | 2025.09.26 | Unlicense | разбор HTML/медиа-ссылок |
| curl | 8.23.0 | curl | прямой HTTP-стрим и HEAD-пробы |

Контрольные суммы SHA-256 закреплены для `windows/amd64` (архив и бинарник) и
проверяются `cmd/depcheck`; для остальных платформ компонент устанавливается с
пометкой `checksum_not_pinned` (честный `verified=false`). rclone обязателен,
yt-dlp и curl — опциональны (встроенный Go-источник закрывает `auto`).

```bash
# Проверка скачанных архивов и распакованных бинарников
go run ./cmd/depcheck -os windows -arch amd64 -staging <dir> -deps <deps-dir>
```

## Тесты и проверки

```bash
go test ./...            # юнит-тесты всех пакетов
go vet ./...
cd web && npm ci && npm run build   # tsc --noEmit + vite build
```

Ключевые тесты:

- `internal/engine` — инварианты кольцевого буфера, инспектор, парсеры
  заголовков, а также сквозной пайплайн с фейковыми `curl`/`rclone`:
  совпадение байт, SHA-256 и **отсутствие файлов на диске**; рестарт при
  источнике без Range.
- `internal/deps` — каталог, платформенные пины, извлечение архивов (включая
  мног файловый curl), проверка хешей.
- `internal/queue` — лимит параллелизма и отсутствие переподписки.
- `internal/oauth` — PKCE (RFC-вектор), одноразовые сессии, колбэк на
  эфемерном порту.
- `internal/server/middleware` — отказ внешнему Origin и запросу без токена.
- `internal/stats` — золотые значения износа (1480 ГиБ).
- `internal/security` — round-trip шифрования секретов.

CI: `.github/workflows/ci.yml` (сборка SPA, vet, тесты, Windows-smoke с
проверкой встроенного UI, токена и 401 без токена). Релиз:
`.github/workflows/release.yml` — 6 таргетов, компоненты для Windows,
`depcheck`, публикация ZIP по тегу `v*`.

## Ограничения v1

- OAuth для Google Drive/OneDrive требует собственных client id/secret, если
  провайдер их не предоставляет; Яндекс.Диск можно подключить токеном.
- Автоизвлечение cookies: Chromium 127+ использует App-Bound Encryption, ключ
  недоступен стороннему процессу — используйте `cookies.txt` или ручной Cookie.
  Firefox (NSS) поддерживается best-effort.
- Подпись обновлений проверяется только при наличии настроенного публичного
  ключа; по умолчанию контролируется SHA-256.
- Пины контрольных сумм покрывают `windows/amd64`; другие платформы —
  best-effort.
- Ручной smoke против реальных Mail.ru/Яндекс/Drive/OneDrive требует
  пользовательских учётных данных и в этой среде не выполнялся.

## Вне v1

Сознательно не входит (см. `implementation_plan.md`, Scope Guard): мобильные
клиенты, доступ из локальной сети и мультипользовательские профили, прокси/VPN
режимы, cron-расписания, torrent/magnet, WebDAV-сервер как назначение,
рекурсивный поиск по всему облаку, докачка после смерти процесса, новые языки,
MSI-установщик, телеметрия и браузерные push-уведомления.

## Структура

```
cmd/streamdrop      запуск сервиса
cmd/depcheck        проверка целостности компонентов (CI)
internal/app        сборка контейнера, порт, graceful shutdown, tray
internal/config     конфигурация
internal/engine     буфер, инспектор, пайплайн, процессы, gate
internal/transfer   задачи, retry, метрики, очистка
internal/queue      планировщик
internal/rclone     RC-клиент, демон, проводник, provisioner
internal/deps       каталог, установка, проверка
internal/auth       cookies, профили браузеров
internal/oauth      PKCE-сессии и колбэк
internal/notify     toast + Telegram
internal/update     проверка и применение обновлений
internal/ws         WebSocket-хаб и snapshot
internal/server     REST, middleware, встроенная статика
web                 React SPA (Vite, Tailwind, i18n ru/en)
```
