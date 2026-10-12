# Canonical build/image versions. Go comes from the module manifest.
GO_VERSION := $(shell awk '$$1 == "go" {print $$2; exit}' go.mod)
ALPINE_VERSION := 3.21
POSTGRES_VERSION := 17-alpine
VERSION_VARS := GO_VERSION ALPINE_VERSION POSTGRES_VERSION
export $(VERSION_VARS)
