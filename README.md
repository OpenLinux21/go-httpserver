# Simple HTTP Server

Welcome to the **Simple HTTP Server** project! This is a straightforward HTTP server written in Go, designed for simplicity and efficiency. It provides a reliable foundation for serving static files and handling HTTP requests.

## Features

- **Static File Serving**: Serve HTML, CSS, JavaScript, and other static files.
- **Customizable Configuration**: Easily configure server settings through a simple configuration file.
- **IPv4 and IPv6 Support**: Bind to both IPv4 and IPv6 addresses.
- **Logging**: Detailed logging of requests with timestamps and client IPs.
- **Graceful Shutdown**: Ensures all ongoing requests are handled before shutting down.

## Installation

To get started, you'll need to have Go installed on your machine. You can download Go from [the official Go website](https://golang.org/dl/).

1. **Clone the repository:**

   ```bash
   git clone https://github.com/OpenLinux21/go-httpserver.git
   cd go-httpserver
2. **Build the server:**

   ```bash
   go build -o web web.go

## Configuration

The server reads its configuration from a file named config.conf in the same directory. The configuration file should have the following format:

   ```ini
ip-address=0.0.0.0
port=8080
root=./public
index=index.html;home.html
404-error=404.html
403-error=403.html
```

ip-address: The IP address to bind the server to. Can be an IPv4 or IPv6 address.
port: The port number on which the server listens.
root: The root directory where static files are served from.
index: A semicolon-separated list of index files to use when a directory is requested.
404-error: The file to serve when a requested file is not found.
403-error: The file to serve when access to a file is forbidden.


3. **Usage**

   ```bash
./web
```

Start the server: Run the compiled binary. The server will listen on the address and port specified in the configuration file.
Handle requests: The server will serve files from the root directory and use the specified index files for directory requests.

## Logging

The server logs request details including client IP, port, requested file, and the amount of data sent. Logs are written to both the console and a file named latest.log.

## Graceful Shutdown

To shut down the server, simply send a Ctrl+C signal. The server will gracefully shut down, ensuring all ongoing requests are completed.

## Example

   ```ini
ip-address=0.0.0.0
port=8081
root=./public
index=index.html
404-error=404.html
403-error=403.html
```

In this example, the server will bind to the IPv4 localhost address (0.0.0.0) and listen on port 8081.

## Contribution

Feel free to contribute to this project by submitting issues or pull requests. Your feedback and contributions are highly appreciated!
