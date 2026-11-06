.PHONY: build test release dev
build:
	bash scripts/build.sh
test:
	cd frontend && npm ci && npm run typecheck && npm run lint && npm test
	cd backend && go test -race ./...
	python3 tests/installer_test.py
release: build
	bash scripts/package.sh $(VERSION)
# Example: make dev CONFIG=/absolute/config.json SINGBOX=/absolute/sing-box
# Local mode permits inspection/validation and never restarts a service.
dev:
	@test -n "$(CONFIG)" -a -n "$(SINGBOX)" || (echo 'Usage: make dev CONFIG=/absolute/config.json SINGBOX=/absolute/sing-box'; exit 2)
	cd backend && go run ./cmd/manager serve --local --state-dir ../.local/manager --config "$(CONFIG)" --sing-box "$(SINGBOX)"
