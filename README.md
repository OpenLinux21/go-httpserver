# Go HTTP Server

A small HTTP/HTTPS static-file server written in Go. It serves regular files,
index pages, and escaped directory listings from a confined document root.

## Features

- HTTP and optional HTTPS listeners
- Static files with MIME detection, conditional requests, and byte ranges
- Gzip for text responses, with correct `Vary` and range handling
- Escaped directory listings and custom 403/404 pages
- Per-client throttling with temporary bans and `429` responses
- Security headers, trusted-proxy controls, and graceful shutdown
- Cross-compilation for Linux, Windows, and macOS

## Requirements

- Go 1.25 or newer

## Run

```bash
go run ./cmd/server
```

On first run, the server creates `config.conf` and `website/` if they do not
exist. The checked-in example listens on `http://0.0.0.0:8081`.

## Configuration

```ini
# Basic Configuration
ip-address = 0.0.0.0
port = 8081
root = ./website
index = index.html;index.htm
404-error = 404.html
403-error = 403.html
trusted-proxies =

# HTTPS Configuration
enable-https = false
cert-file = server.crt
key-file = server.key
https-port = 8443

# Logging Configuration
log-buffer-size = 1MiB
log-flush-threshold = 75
log-flush-interval = 5
log-max-entries = 12500
```

List values are separated by semicolons. Configuration is validated at
startup. Index files and custom error pages must remain inside `root`.

`trusted-proxies` is empty by default, so client-supplied forwarding headers
are ignored. When running behind a reverse proxy, set only its IP addresses or
CIDR ranges, for example:

```ini
trusted-proxies = 127.0.0.1;10.0.0.0/8
```

When HTTPS is enabled, both certificate files must already exist and the HTTP
and HTTPS ports must differ. The plaintext listener then answers every
request with a permanent redirect (`308`) to the same URL on the HTTPS port.

## Logging

All logs (access log, panic reports, rate-limit notices, startup messages)
share one buffered writer that mirrors every entry to stdout immediately and
batches file writes:

- `log-buffer-size` (256KiB..64MiB, default `1MiB`): in-memory buffer
  capacity. Accepts plain byte counts or `KiB`/`MiB`/`GiB` suffixes.
- `log-flush-threshold` (50..95, default `75`): buffer fill percentage that
  triggers a write-back to `latest.log`.
- `log-flush-interval` (0..3600 seconds, default `5`; `0` disables): pending
  entries are written back at least this often so quiet servers still flush.
- `log-max-entries` (250..10000000, default `12500`): entries kept in
  `latest.log`. Once the cap plus a 10% margin is reached, the file is
  rewritten atomically, dropping the oldest entries, so its size stays
  bounded no matter how much traffic arrives.

On shutdown (including `SIGINT`/`SIGTERM`) everything still buffered is
flushed and synced. Entries still in memory are lost only on an unclean kill
(`SIGKILL`, OOM), which can discard up to the most recent
`100 - log-flush-threshold` percent of the buffer. A partially written line
left by such a kill is repaired on the next start.

## Build

Build interactively:

```bash
./build.sh
```

Or provide the target explicitly for CI:

```bash
./build.sh linux amd64
./build.sh windows arm64
```

The output is `web_server` or `web_server.exe`.

## Test

```bash
go test ./...
go test -race ./...
go vet ./...
```

Tests cover root confinement, escaped listings, HTTP method/error behavior,
HTTPS redirects, compression negotiation, rate limiting, and the buffered
logger (threshold/interval/exit flushes, entry cap, restart recovery).

## Project Layout

```text
cmd/server/          startup and server lifecycle
internal/config/     configuration parsing and validation
internal/handlers/   root-confined file serving and directory listings
internal/logger/     buffered log writer with capped history file
internal/middleware/ security headers, compression, and rate limiting
website/             default document root
```

## Security Notes

- Files are opened through `os.Root`; symlinks cannot escape the configured
  document root.
- Only regular files and directories are served.
- Only `GET` and `HEAD` are accepted for static content.
- Do not add untrusted networks to `trusted-proxies`.
- Put the service behind a reverse proxy when you need public TLS automation,
  request-body controls, or centralized log rotation.

## License

Apache License 2.0. See [LICENSE](LICENSE).
