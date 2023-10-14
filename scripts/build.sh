#!/bin/bash
set -x -e -o pipefail

rm -rf bin/
mkdir bin/

# Build binaries for linux-based Docker container.
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64

for d in ./cmd/*; do
	if test -f ${d}/docker/Dockerfile; then
	  go build -o bin/ ${d}
	fi
done
