DOCKER ?= docker
TEST_DB_CONTAINER ?= test-location-postgres
TEST_DB_PORT ?= 55439

.PHONY: test-db-up test-db-down test-integration
test-db-up:
	$(DOCKER) run --detach --rm --name $(TEST_DB_CONTAINER) --label io.mralexandrov.location.test=1 --tmpfs /var/lib/postgresql/data:rw,size=128m --memory=256m -p 127.0.0.1:$(TEST_DB_PORT):5432 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=location_test $(POSTGRES_IMAGE)
	@for attempt in $$(seq 1 30); do $(DOCKER) exec $(TEST_DB_CONTAINER) pg_isready -h 127.0.0.1 -U postgres -d location_test >/dev/null && exit 0; sleep 1; done; exit 1

test-db-down:
	@if [ "$$($(DOCKER) inspect -f '{{index .Config.Labels "io.mralexandrov.location.test"}}' $(TEST_DB_CONTAINER) 2>/dev/null)" = 1 ]; then $(DOCKER) stop $(TEST_DB_CONTAINER); fi

test-integration: export TEST_DATABASE_URL ?= postgres://postgres@127.0.0.1:$(TEST_DB_PORT)/location_test?sslmode=disable
test-integration:
	go test -race -count=1 ./internal/repository
