#!/bin/bash
set -x -e -o pipefail

rm -rf bin/
mkdir bin/

# Build binaries for linux-based Docker container.
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64

go build -o bin/easyp-server ./cmd/easyp

dockerTag="easyp/server:v0.1.0"
docker build -f Dockerfile --tag ${dockerTag} .
docker push ${dockerTag}