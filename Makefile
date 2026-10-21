.PHONY: dev check check-api check-web build

BIN ?= bin/kcal-bot

# API on :8080 with the fake estimator (or a real one if a key is exported);
# run `npm --prefix web run dev` alongside — Vite proxies /api to it.
dev:
	go -C api run . -db ./kcal.db -dev

check: check-api check-web

check-api:
	go -C api test ./...
	go -C api vet ./...

check-web:
	npm --prefix web test
	npm --prefix web run build

# One binary plus web/dist is the whole deployable.
build:
	npm --prefix web run build
	go -C api build -o ../$(BIN) .
