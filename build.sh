#!/bin/bash

# Set environment variables
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64

# Build the project
echo "Building web server..."
go build -a -ldflags '-extldflags "-static"' -o web_server ./cmd/server

# Check if build was successful
if [ $? -eq 0 ]; then
    echo "Build successful!"
    echo "Binary created: web_server"
else
    echo "Build failed!"
    exit 1
fi 