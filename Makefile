.PHONY: api worker scheduler manage migrate-up migrate-down migrate-fresh seed-demo test fmt web-install web-dev web-check hooks dev-up dev-down

api:
	GOCACHE=$${GOCACHE:-/tmp/onixggr-go-build} go run ./apps/api

worker:
	GOCACHE=$${GOCACHE:-/tmp/onixggr-go-build} go run ./apps/worker

scheduler:
	GOCACHE=$${GOCACHE:-/tmp/onixggr-go-build} go run ./apps/scheduler

manage:
	./appctl

migrate-up:
	./scripts/migrate-up.sh

migrate-down:
	./scripts/migrate-down.sh

migrate-fresh:
	./scripts/migrate-fresh.sh

seed-demo:
	./scripts/seed-demo.sh

test:
	GOCACHE=$${GOCACHE:-/tmp/onixggr-go-build} go test ./...

fmt:
	gofmt -w $(shell find apps internal -name '*.go' -type f)

web-install:
	npm install

web-dev:
	npm run dev:web

web-check:
	npm run check:web

hooks:
	git config core.hooksPath .githooks

dev-up:
	./scripts/podman-up.sh

dev-down:
	./scripts/podman-down.sh
