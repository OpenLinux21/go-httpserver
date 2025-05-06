# Go HTTP Server

A lightweight, high-performance HTTP/HTTPS server written in Go. This project provides a simple yet feature-complete web server that supports static file serving, directory listing, HTTPS, Gzip compression, and more.

## Features

- **HTTP/HTTPS Support**: Supports both HTTP and HTTPS protocols
- **Gzip Compression**: Automatically compresses text files (HTML, CSS, JS, etc.)
- **Security Features**:
  - Custom security headers
  - HTTPS support
  - XSS protection
  - Content Security Policy (CSP)
- **Directory Listing**: Beautiful directory browsing interface
- **Custom Error Pages**: Support for 404, 403, and other error pages
- **Request Logging**: Detailed access logging
- **Static File Serving**: Efficient file serving
- **Configurability**: Easy server behavior adjustment through configuration file

## Project Structure

```
.
├── cmd/
│   └── server/          # Main application entry point
├── internal/
│   ├── config/         # Configuration management
│   ├── handlers/       # HTTP request handlers
│   ├── logger/         # Logging functionality
│   ├── middleware/     # HTTP middleware (gzip, security headers)
│   └── utils/          # Utility functions
├── website/            # Web content directory
├── build.sh           # Build script
├── config.conf        # Configuration file
├── go.mod            # Go module file
└── README.md         # Project documentation
```

## Quick Start

### Build

Use the provided build script to build the project:

```bash
./build.sh
```

This will create a statically linked binary named `web_server`.

### Configuration

The server is configured through `config.conf`. Example configuration:

```ini
# Basic Configuration
ip-address = 0.0.0.0
port = 8081
root = ./website
index = index.html;index.htm
404-error = 404.html
403-error = 403.html

# HTTPS Configuration
enable-https = true
cert-file = server.crt
key-file = server.key
https-port = 8443
```

### Run

Start the server:

```bash
./web_server
```

The server will listen on the configured ports (both HTTP and HTTPS if enabled).

## Features in Detail

### 1. Static File Serving

The server can efficiently serve static files, supporting:
- Automatic MIME type detection
- Range requests
- Conditional requests
- Directory indexing

### 2. Directory Listing

When accessing a directory, the server displays a beautiful directory listing, including:
- File/directory names
- Last modified time
- File size
- File type icons
- Parent directory link

### 3. Security Features

The server implements multiple layers of security:
- Content Security Policy (CSP)
- XSS protection
- Clickjacking protection
- MIME type sniffing protection
- HTTPS support

### 4. Performance Optimization

- Gzip compression
- Static file caching
- Efficient file serving
- Concurrent request handling

### 5. Logging

The server provides detailed access logs, including:
- Timestamp
- Client IP
- Request method
- Request path
- Status code
- Response time
- User agent
- Error information (if any)

## Development Guide

### Project Modules

- `cmd/server`: Main application entry point, responsible for server startup and configuration
- `internal/config`: Configuration management, handles config file reading and parsing
- `internal/handlers`: HTTP request handling, implements file serving and directory listing
- `internal/logger`: Log management, handles request logging
- `internal/middleware`: Middleware functionality, including Gzip compression and security headers
- `internal/utils`: Utility functions, provides common functionality

### Adding New Features

1. Add new functionality in the appropriate module
2. Update the configuration system (if needed)
3. Add corresponding tests
4. Update documentation

### Module Development Guide

#### 1. Creating a New Module

To add a new module to the server:

1. Create a new directory under `internal/` for your module:
   ```bash
   mkdir internal/yourmodule
   ```

2. Create the main module file:
   ```bash
   touch internal/yourmodule/yourmodule.go
   ```

3. Define your module's package and interfaces:
   ```go
   package yourmodule

   // YourModule defines the interface for your module
   type YourModule interface {
       // Define your module's methods
       Initialize() error
       Process() error
   }

   // Implementation of your module
   type yourModule struct {
       // Add your module's fields
   }

   // New creates a new instance of your module
   func New() YourModule {
       return &yourModule{}
   }
   ```

#### 2. Adding Configuration

If your module needs configuration:

1. Add configuration fields to `internal/config/config.go`:
   ```go
   type Config struct {
       // ... existing fields ...
       YourModuleConfig struct {
           Enabled bool
           Options map[string]string
       }
   }
   ```

2. Update the default configuration in `config.conf`:
   ```ini
   # Your Module Configuration
   your-module-enabled = true
   your-module-option1 = value1
   ```

#### 3. Integrating with Existing Code

To integrate your module with the server:

1. Import your module in `cmd/server/main.go`:
   ```go
   import (
       "github.com/OpenLinux21/go-httpserver/internal/yourmodule"
   )
   ```

2. Initialize your module in the main function:
   ```go
   func main() {
       // ... existing code ...
       
       yourModule := yourmodule.New()
       if err := yourModule.Initialize(); err != nil {
           log.Fatalf("Failed to initialize your module: %v", err)
       }
   }
   ```

#### 4. Best Practices

1. **Error Handling**:
   - Use meaningful error messages
   - Implement proper error wrapping
   - Log errors appropriately

2. **Testing**:
   - Write unit tests for your module
   - Include integration tests
   - Use table-driven tests where appropriate

3. **Documentation**:
   - Document all exported types and functions
   - Include usage examples
   - Update README.md with new features

4. **Code Style**:
   - Follow Go's standard formatting
   - Use meaningful variable names
   - Keep functions small and focused

5. **Performance**:
   - Use appropriate data structures
   - Implement proper resource cleanup
   - Consider concurrency where needed

#### 5. Example Module

Here's a simple example of a rate limiter module:

```go
// internal/ratelimit/ratelimit.go
package ratelimit

import (
    "sync"
    "time"
)

type RateLimiter interface {
    Allow(ip string) bool
}

type rateLimiter struct {
    requests map[string][]time.Time
    mu       sync.RWMutex
    limit    int
    window   time.Duration
}

func New(limit int, window time.Duration) RateLimiter {
    return &rateLimiter{
        requests: make(map[string][]time.Time),
        limit:    limit,
        window:   window,
    }
}

func (r *rateLimiter) Allow(ip string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    windowStart := now.Add(-r.window)

    // Clean old requests
    var valid []time.Time
    for _, t := range r.requests[ip] {
        if t.After(windowStart) {
            valid = append(valid, t)
        }
    }

    if len(valid) >= r.limit {
        return false
    }

    valid = append(valid, now)
    r.requests[ip] = valid
    return true
}
```

## License

This project is licensed under the Apache2 License - see the LICENSE file for details
