#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
test -z "$(gofmt -l cmd internal migrations tests)"
go mod verify
go vet ./...
go test -race ./...
