#!/bin/bash
# Generate Go protobuf files from proto definition
# Requires: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#           go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

mkdir -p proto

protoc \
    -I../proto \
    --go_out=proto \
    --go_opt=paths=source_relative \
    --go-grpc_out=proto \
    --go-grpc_opt=paths=source_relative \
    ../proto/hyperliquid.proto

echo "Generated: proto/hyperliquid.pb.go, proto/hyperliquid_grpc.pb.go"
