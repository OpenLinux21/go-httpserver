#!/bin/bash

# ---------------------------------------
# Interactive Go cross-build script
# ---------------------------------------

# OS options
OS_LIST=("linux" "windows" "darwin")
ARCH_MAP_linux=("amd64" "arm64" "arm" "386" "s390x")
ARCH_MAP_windows=("amd64" "arm64" "386")
ARCH_MAP_darwin=("amd64" "arm64")

echo "Select target operating system:"
select CHOSEN_OS in "${OS_LIST[@]}"; do
    if [[ -n "$CHOSEN_OS" ]]; then
        echo "Selected OS: $CHOSEN_OS"
        break
    else
        echo "Invalid selection. Please enter a valid number."
    fi
done

# Determine available architectures
case "$CHOSEN_OS" in
    linux)   ARCH_LIST=("${ARCH_MAP_linux[@]}") ;;
    windows) ARCH_LIST=("${ARCH_MAP_windows[@]}") ;;
    darwin)  ARCH_LIST=("${ARCH_MAP_darwin[@]}") ;;
esac

echo
echo "Select target architecture:"
select CHOSEN_ARCH in "${ARCH_LIST[@]}"; do
    if [[ -n "$CHOSEN_ARCH" ]]; then
        echo "Selected architecture: $CHOSEN_ARCH"
        break
    else
        echo "Invalid selection. Please enter a valid number."
    fi
done

# Set environment variables
export CGO_ENABLED=0
export GOOS="$CHOSEN_OS"
export GOARCH="$CHOSEN_ARCH"

# Output binary name
OUTPUT="web_server"
if [[ "$CHOSEN_OS" == "windows" ]]; then
    OUTPUT="${OUTPUT}.exe"
fi

echo
echo "Building for GOOS=$GOOS GOARCH=$GOARCH ..."
go build -a -ldflags '-extldflags "-static"' -o "$OUTPUT" ./cmd/server

# Result check
if [[ $? -eq 0 ]]; then
    echo "-----------------------------------------"
    echo "Build successful!"
    echo "Binary created: $OUTPUT"
    echo "Target: $GOOS / $GOARCH"
    echo "-----------------------------------------"
else
    echo "Build failed!"
    exit 1
fi
 
