#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

mkdir -p bin

echo "Building server..."
go build -o bin/server ./server/main.go

echo "Building implant..."
go build -o bin/implant ./implant/main.go

echo "Building build-implant..."
go build -o bin/build-implant ./cmd/build-implant/main.go

echo "Done. Binaries in ./bin/"
