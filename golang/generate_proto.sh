#!/bin/bash
set -euo pipefail
# Generate Go protobuf files from proto definition
# Requires: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#           go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

mkdir -p proto

protoc \
    -I../proto \
    --go_out=proto \
    --go_opt=paths=source_relative \
    --go_opt=Mhyperliquid.proto=github.com/example/hyperliquid-grpc/proto \
    --go-grpc_out=proto \
    --go-grpc_opt=paths=source_relative \
    --go-grpc_opt=Mhyperliquid.proto=github.com/example/hyperliquid-grpc/proto \
    ../proto/hyperliquid.proto

protoc \
    -I../proto \
    --go_out=proto \
    --go_opt=paths=source_relative \
    --go_opt=Morderbook.proto=github.com/example/hyperliquid-grpc/proto \
    --go-grpc_out=proto \
    --go-grpc_opt=paths=source_relative \
    --go-grpc_opt=Morderbook.proto=github.com/example/hyperliquid-grpc/proto \
    ../proto/orderbook.proto

echo "Generated: proto/hyperliquid.pb.go, proto/hyperliquid_grpc.pb.go"
echo "Generated: proto/orderbook.pb.go, proto/orderbook_grpc.pb.go"
