#!/bin/bash

set -e

# Build ntppool-agent for Linux amd64 with race detector in Docker container
echo "Building ntppool-agent for Linux amd64 with race detector..."

# Prepare docker run command with optional cache mounts
DOCKER_ARGS="--rm -v $(pwd):/workspace -w /workspace"
ENV_ARGS=""

# Add cache volume mounts and environment variables if set
if [ -n "$GOCACHE" ]; then
    DOCKER_ARGS="$DOCKER_ARGS -v $GOCACHE:/root/.cache/go-build"
    ENV_ARGS="$ENV_ARGS -e GOCACHE=/root/.cache/go-build"
fi

if [ -n "$GOMODCACHE" ]; then
    DOCKER_ARGS="$DOCKER_ARGS -v $GOMODCACHE:/go/pkg/mod"
    ENV_ARGS="$ENV_ARGS -e GOMODCACHE=/go/pkg/mod"
fi

# Use golang debian image
docker run --platform linux/amd64 $DOCKER_ARGS $ENV_ARGS \
    golang:1.24 \
    sh -c "
        apt-get update && apt-get install -y git make && \
        export CGO_ENABLED=1 && \
        go build -v -race -o ntppool-agent-linux-amd64 ./cmd/ntppool-agent
    "

echo "Build complete: ntppool-agent-linux-amd64"
