#!/bin/bash
# Generate Python protobuf files from proto definition

python -m grpc_tools.protoc \
    -I../proto \
    --python_out=. \
    --grpc_python_out=. \
    ../proto/hyperliquid.proto

echo "Generated: hyperliquid_pb2.py, hyperliquid_pb2_grpc.py"
