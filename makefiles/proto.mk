.PHONY: proto-tools proto-gen proto-check proto-breaking
proto-tools:
	@mkdir -p .bin
	@test -x .bin/buf && test "$$(.bin/buf --version)" = "$(BUF_VERSION)" || GOBIN=$(CURDIR)/.bin go install github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
	@test -x .bin/protoc-gen-go && test "$$(.bin/protoc-gen-go --version)" = "protoc-gen-go $(PROTOC_GEN_GO_VERSION)" || GOBIN=$(CURDIR)/.bin go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@test -x .bin/protoc-gen-go-grpc && test "$$(.bin/protoc-gen-go-grpc --version)" = "protoc-gen-go-grpc $(PROTOC_GEN_GO_GRPC_VERSION)" || GOBIN=$(CURDIR)/.bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v$(PROTOC_GEN_GO_GRPC_VERSION)
proto-gen: proto-tools
	.bin/buf generate
proto-check: proto-gen
	.bin/buf format --diff --exit-code proto
	.bin/buf build proto
	.bin/buf lint proto
# Use a previous released revision containing the initial geofence contract.
proto-breaking: proto-tools
	@test -n "$(PROTO_BREAKING_AGAINST)" || { echo 'Set PROTO_BREAKING_AGAINST to the previous released proto module'; exit 1; }
	.bin/buf breaking proto --against '$(PROTO_BREAKING_AGAINST)'
