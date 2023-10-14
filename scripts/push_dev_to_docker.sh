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
	  dockerTag="kriogenik/${d#"./cmd/"}:stage"
	  docker build -f ${d}/docker/Dockerfile --tag ${dockerTag} .
	  echo ${dockerTag}
	  docker push ${dockerTag}
	fi
done
