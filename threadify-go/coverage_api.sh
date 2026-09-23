#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/api"
echo "Running AI gateway coverage..."
go test -race -coverprofile=coverage.out -coverpkg=./gateway/... ./...
go tool cover -html=coverage.out -o coverage.html
echo "Coverage report generated: api/coverage.html"
