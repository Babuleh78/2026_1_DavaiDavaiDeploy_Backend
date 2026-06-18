#!/usr/bin/env bash
# Regenerate gRPC stubs from proto files.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
#
# Install plugins:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

protoc \
  --proto_path="${REPO_ROOT}/proto" \
  --go_out="${REPO_ROOT}" \
  --go_opt=paths=source_relative \
  --go_opt=Mnotifications.proto=./internal/pkg/notifications/delivery/grpc/gen \
  --go-grpc_out="${REPO_ROOT}" \
  --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=Mnotifications.proto=./internal/pkg/notifications/delivery/grpc/gen \
  "${REPO_ROOT}/proto/notifications.proto"

echo "Done — stubs written to internal/pkg/notifications/delivery/grpc/gen/"
