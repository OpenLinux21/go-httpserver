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
and HTTPS ports must differ.

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
compression negotiation, and rate limiting.

## Project Layout

```text
cmd/server/          startup and server lifecycle
internal/config/     configuration parsing and validation
internal/handlers/   root-confined file serving and directory listings
internal/logger/     access-log setup
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
