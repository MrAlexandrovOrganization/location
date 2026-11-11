# Canonical build/image versions. Go comes from the module manifest.
GO_VERSION := $(shell awk '$$1 == "go" {print $$2; exit}' go.mod)
ALPINE_VERSION := 3.21
POSTGRES_VERSION := 17-alpine
BUF_VERSION := 1.67.0
PROTOC_GEN_GO_GRPC_VERSION := 1.6.1
PROTOC_GEN_GO_VERSION := $(shell awk '$$1 == "google.golang.org/protobuf" {print $$2; exit}' go.mod)
VERSION_VARS := GO_VERSION ALPINE_VERSION POSTGRES_VERSION BUF_VERSION PROTOC_GEN_GO_GRPC_VERSION PROTOC_GEN_GO_VERSION
export $(VERSION_VARS)

POSTGRES_IMAGE ?= postgres:$(POSTGRES_VERSION)
export POSTGRES_IMAGE
