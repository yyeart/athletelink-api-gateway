#!/usr/bin/env bash

.tools/protoc/bin/protoc \
  -I api/proto \
  -I .tools/protoc/include \
  --plugin=protoc-gen-go=.tools/bin/protoc-gen-go \
  --plugin=protoc-gen-go-grpc=.tools/bin/protoc-gen-go-grpc \
  --go_out=api/gen \
  --go_opt=paths=source_relative \
  --go-grpc_out=api/gen \
  --go-grpc_opt=paths=source_relative \
  api/proto/athletelink/gateway/v1/core.proto