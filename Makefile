.PHONY: api worker scheduler manage migrate-up migrate-fresh seed-demo test fmt web-install web-dev web-check hooks dev-up dev-down

api:
	go run ./apps/api

worker:
	go run ./apps/worker

scheduler:
	go run ./apps/scheduler

manage:
	go run ./apps/manage

migrate-up:
	go run ./apps/manage migrate up

migrate-fresh:
	go run ./apps/manage migrate fresh

seed-demo:
	go run ./apps/manage seed demo

test:
	go test ./...

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
