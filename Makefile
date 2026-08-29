DOCKER_COMPOSE = docker compose
BINARY = location
MONITORING_DATA_DIR ?= $(HOME)/.monitoring

-include .env
export

.PHONY: build
build:
	go build -o $(BINARY) ./cmd/location

.PHONY: up
up: monitoring
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
	@mkdir -p $(MONITORING_DATA_DIR)/grafana-datasources
	envsubst < monitoring/grafana/datasource.tpl.yaml > $(MONITORING_DATA_DIR)/grafana-datasources/location.yaml
	@echo "location: grafana datasource provisioned at $(MONITORING_DATA_DIR)/grafana-datasources/location.yaml"

.PHONY: monitoring-down
monitoring-down:
	@rm -f $(MONITORING_DATA_DIR)/grafana-datasources/location.yaml

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: format
format:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

.PHONY: clean
clean:
	rm -f $(BINARY)
