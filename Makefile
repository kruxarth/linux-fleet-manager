.PHONY: build test run-server run-agent web web-dev compose demo demo-down

GO ?= go

build:
	mkdir -p bin
	$(GO) build -o bin/server ./cmd/server
	$(GO) build -o bin/agent ./cmd/agent
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o bin/agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -o bin/agent-linux-arm64 ./cmd/agent

test:
	$(GO) test ./...

web:
	cd web && npm install && npm run build

web-dev:
	cd web && npm run dev

run-server: build
	FLEET_ADMIN_TOKEN=$${FLEET_ADMIN_TOKEN:-dev-admin-token} \
	FLEET_AGENT_BIN_DIR=./bin \
	./bin/server

run-agent:
	FLEET_MANAGER_URL=$${FLEET_MANAGER_URL:-http://127.0.0.1:8080} \
	./bin/agent -config $${FLEET_AGENT_CONFIG:-./agent.yaml}

compose:
	docker compose -f deploy/docker-compose.yml up --build

# Same as `docker compose up --build` from the repo root, detached.
demo:
	docker compose up --build -d
	@echo "Open http://localhost:8080 and click 'Sign in to the demo'."

demo-down:
	docker compose down
