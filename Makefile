DOCKER_COMPOSE_BASE = docker compose --project-directory "$(CURDIR)"
include makefiles/compose.mk
DOCKER_COMPOSE = docker compose --project-directory "$(CURDIR)" $(COMPOSE_FLAGS)
BINARY = location
MONITORING_DATA_DIR ?= $(HOME)/.monitoring
GRAFANA_LOCATION_DB_ADDRESS ?= location-db:5432

-include .env
export
include makefiles/versions.mk

.PHONY: build
build:
	go build -o $(BINARY) ./cmd/location

.PHONY: up
up: active-check monitoring
	$(DOCKER_COMPOSE) up -d --build

.PHONY: down
down: monitoring-down
	$(DOCKER_COMPOSE) down

.PHONY: logs
logs:
	$(DOCKER_COMPOSE) logs -f

.PHONY: deploy
deploy: monitoring
	$(DOCKER_COMPOSE) up -d --build --no-cache

.PHONY: restart
restart:
	$(DOCKER_COMPOSE) restart location

# Publishes this project's Grafana datasource definition into the shared
# provisioning directory that Grafana mounts. Grafana stays generic and only
# consumes whatever datasource files appear there.
.PHONY: monitoring
monitoring:
	@mkdir -p $(MONITORING_DATA_DIR)/prometheus-targets
	@cp monitoring/prometheus/location.yml $(MONITORING_DATA_DIR)/prometheus-targets/location.yml
	@mkdir -p $(MONITORING_DATA_DIR)/grafana-datasources
	envsubst '$${GRAFANA_LOCATION_DB_ADDRESS} $${GRAFANA_DB_PASSWORD}' < monitoring/grafana/datasource.tpl.yaml > $(MONITORING_DATA_DIR)/grafana-datasources/location.yaml
	@echo "location: grafana datasource provisioned at $(MONITORING_DATA_DIR)/grafana-datasources/location.yaml"

.PHONY: monitoring-down
monitoring-down:
	@rm -f $(MONITORING_DATA_DIR)/prometheus-targets/location.yml
	@rm -f $(MONITORING_DATA_DIR)/grafana-datasources/location.yaml

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: format
format:
	gofmt -w cmd internal migrations

.PHONY: clean
clean:
	rm -f $(BINARY)

.PHONY: versions compose-build
versions:
	@$(foreach v,$(VERSION_VARS),printf '%s=%s\n' '$(v)' '$($(v))';)
compose-build:
	$(DOCKER_COMPOSE) build

# CI builds/publishes once; VM activates immutable digests (Python 3 + Docker required).
export IMAGE_COMPOSE_FILE := compose/docker-compose.yml
.PHONY: image-build image-publish image-deploy image-activate
image-build:
	python3 scripts/images.py build
image-publish:
	python3 scripts/images.py publish
image-deploy:
	python3 scripts/images.py deploy
image-activate: active-check monitoring
	$(IMAGE_COMPOSE) up -d --no-build --pull never --wait postgres
	$(IMAGE_COMPOSE) run --rm --no-deps --pull never location migrate
	$(IMAGE_COMPOSE) up -d --no-build --pull never --wait --wait-timeout 180
	curl --fail --silent --max-time 10 http://127.0.0.1:18084/readyz

.PHONY: install run migrate test-race
install:
	go mod download
run:
	go run ./cmd/location
migrate:
	go run ./cmd/location migrate
test-race:
	go test -race ./...

.PHONY: mobile-device
mobile-device:
	python3 scripts/mobile-device.py

.PHONY: check
check: test vet config-check
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

.PHONY: image-check
image-check:
	python3 -B scripts/test_images.py

.PHONY: config-check
config-check:
	$(DOCKER_COMPOSE) --env-file /dev/null config --quiet --no-env-resolution

.PHONY: image-compose
export IMAGE_ARGS
image-compose:
	python3 scripts/images.py compose

include makefiles/ci-deploy.mk
include makefiles/tests.mk

# Read-only validation of every tracked Compose combination, without runtime secrets.
.PHONY: compose-layout-check
compose-layout-check:
	$(DOCKER_COMPOSE_BASE) --env-file /dev/null -f compose/docker-compose.yml --profile '*' config --no-interpolate --no-env-resolution --quiet

check: compose-layout-check

.PHONY: active-check
active-check:
	@test ! -e .retired || { echo "This stack has been retired" >&2; exit 1; }
