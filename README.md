# Simple HTTP Server

Welcome to the **Simple HTTP Server** project! This is a straightforward HTTP server written in Go, designed for simplicity and efficiency. It provides a reliable foundation for serving static files and handling HTTP requests.

## Features

- **Static File Serving**: Serve HTML, CSS, JavaScript, and other static files.
- **Customizable Configuration**: Easily configure server settings through a simple configuration file.
- **IPv4 and IPv6 Support**: Bind to both IPv4 and IPv6 addresses.
- **Logging**: Detailed logging of requests with timestamps and client IPs.
- **Graceful Shutdown**: Ensures all ongoing requests are handled before shutting down.
- **HTTPS Support**: Optional HTTPS support with custom certificate and port configuration.

## Installation

To get started, you'll need to have Go installed on your machine. You can download Go from [the official Go website](https://golang.org/dl/).

1. **Clone the repository:**

   ```bash
   git clone https://github.com/OpenLinux21/go-httpserver.git
   cd go-httpserver
   ```

2. **Build the server:**

   ```bash
   go mod init web && go mod tidy
   go build -o web_server
   ```

## Configuration

The server reads its configuration from a file named config.conf in the same directory. Example configuration:

   ```ini
# Basic Configuration
ip-address = 0.0.0.0
port = 8081
root = ./website/
index = index.html;index.htm
404-error = 404.html
403-error = 403.html

# HTTPS Configuration
enable-https = true
cert-file = server.crt
key-file = server.key
https-port = 8443
```

### Basic Configuration
- `ip-address`: The IP address to bind the server to. Can be an IPv4 or IPv6 address.
- `port`: The port number on which the HTTP server listens.
- `root`: The root directory where static files are served from.
- `index`: A semicolon-separated list of index files to use when a directory is requested.
- `404-error`: The file to serve when a requested file is not found.
- `403-error`: The file to serve when access to a file is forbidden.

### HTTPS Configuration
- `enable-https`: Enable or disable HTTPS support (true/false).
- `cert-file`: Path to the SSL certificate file.
- `key-file`: Path to the SSL private key file.
- `https-port`: The port number on which the HTTPS server listens.

## Usage

```bash
./web_server
```
Start the server: Run the compiled binary. The server will listen on the address and port specified in the configuration file.
Handle requests: The server will serve files from the root directory and use the specified index files for directory requests.

## Logging

The server logs request details including client IP, port, requested file, and the amount of data sent. Logs are written to both the console and a file named latest.log.

## Graceful Shutdown

To shut down the server, simply send a Ctrl+C signal. The server will gracefully shut down, ensuring all ongoing requests are completed.

### Generating Self-Signed Certificate

For testing HTTPS, you can generate a self-signed certificate using OpenSSL:

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout server.key -out server.crt \
    -subj "/C=CN/ST=State/L=City/O=Organization/CN=localhost"
```

## Contribution

Feel free to contribute to this project by submitting issues or pull requests. Your feedback and contributions are highly appreciated!
