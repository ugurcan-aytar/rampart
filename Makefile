# rampart — A rampart for your supply chain.
# See CONTRIBUTING.md for dev setup.

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# --- Help ------------------------------------------------------------------

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_.-]+:.*?## / {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# --- Bootstrap -------------------------------------------------------------

.PHONY: bootstrap
bootstrap: ## Install JS deps + sync Go workspace. Fails if supply-chain gates are off.
	@command -v node     >/dev/null || { echo "MISSING: node (need 20+)";              exit 1; }
	@command -v go       >/dev/null || { echo "MISSING: go (need 1.22+)";              exit 1; }
	@command -v corepack >/dev/null || { echo "MISSING: corepack (bundled with node)"; exit 1; }
	@echo "==> node      $$(node --version)"
	@echo "==> go        $$(go version)"
	@echo "==> corepack  $$(corepack --version)"
	@echo "==> corepack enable  (activate packageManager from package.json)"
	corepack enable
	@echo "==> yarn install"
	yarn install
	@echo "==> supply-chain gate: enableScripts must be false"
	@ACTUAL="$$(yarn config get enableScripts)"; \
	if [ "$$ACTUAL" != "false" ]; then \
	  echo "FAIL: yarn config enableScripts=$$ACTUAL (expected false)"; \
	  echo "      Refusing to continue. Fix .yarnrc.yml and retry."; \
	  exit 1; \
	fi; \
	echo "PASS: enableScripts=false"
	@echo "==> supply-chain gate: exact versions (no caret/tilde)"
	@ACTUAL="$$(yarn config get defaultSemverRangePrefix)"; \
	if [ "$$ACTUAL" != "" ] && [ "$$ACTUAL" != '""' ]; then \
	  echo "FAIL: defaultSemverRangePrefix='$$ACTUAL' (expected empty)"; \
	  exit 1; \
	fi; \
	echo "PASS: defaultSemverRangePrefix=''"
	@echo "==> go work sync"
	go work sync
	@echo "bootstrap complete."

# --- Codegen ---------------------------------------------------------------

OAPI_CODEGEN_VERSION := v2.4.1

.PHONY: gen gen-go gen-ts gen-check
gen: gen-go gen-ts ## Regenerate Go + TS types from OpenAPI.

gen-go: ## Regenerate engine Go types from schemas/openapi.yaml.
	cd engine && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) \
	  -config oapi-codegen.yaml ../schemas/openapi.yaml

gen-ts: ## Regenerate Backstage plugin TS types from schemas/openapi.yaml.
	@test -x node_modules/.bin/openapi-typescript || yarn install
	cd backstage/plugins/rampart && yarn run gen:api

gen-check: gen ## CI gate — regen then assert zero diff. Fails if a contract edit forgot to run make gen.
	@if ! git diff --exit-code --quiet; then \
	  echo "FAIL: generated artefacts drift from schemas/openapi.yaml."; \
	  echo "      Run 'make gen' and commit the result."; \
	  git --no-pager diff --stat; \
	  exit 1; \
	fi
	@echo "PASS: generated artefacts in sync with schemas/openapi.yaml"

# --- Build / Test / Lint (filled in Adım 2+) -------------------------------

.PHONY: build
build: ## Build engine + cli + backstage plugins. (Adım 2+)
	@echo "Not yet wired — see FIRST.md Adım 2."

.PHONY: test
test: ## Run all tests (Go, Rust, TS). (Adım 2+)
	@echo "Not yet wired — see FIRST.md Adım 2."

.PHONY: lint
lint: ## Run all linters (golangci-lint, eslint, clippy). (Adım 2+)
	@echo "Not yet wired — see FIRST.md Adım 2."

# --- Demo stack (Adım 7.5) -------------------------------------------------
#
# Default profile brings up engine + mock-npm-registry + slack-notifier.
# Backstage is not in the compose stack (see ADR deferral note in
# docker-compose.yml) — run `yarn workspace rampart dev` in a second
# terminal and open http://localhost:3000 to see the IncidentDashboard.

.PHONY: demo demo-axios demo-shai-hulud demo-vercel demo-native demo-down
demo: ## Bring up the demo stack + seed the catalog, leave it running.
	docker compose up -d --build
	./scripts/seed-catalog.sh
	@echo ""
	@echo "demo is up."
	@echo "  engine:            http://localhost:8080"
	@echo "  mock-npm-registry: http://localhost:8081"
	@echo "  slack-notifier:    docker compose logs -f slack-notifier"
	@echo ""
	@echo "Next: pick a scenario —"
	@echo "  make demo-axios       2026-03-31 axios compromise"
	@echo "  make demo-shai-hulud  2026-04-18 rampage-* worm"
	@echo "  make demo-vercel      2026-04-19 Vercel OAuth leak"
	@echo ""
	@echo "  For the Backstage UI, run in a second terminal:"
	@echo "    yarn workspace rampart dev"
	@echo "  then open http://localhost:3000."

demo-axios: ## Replay the 2026-03-31 axios compromise against the running stack.
	docker compose up -d --build
	./scripts/demo-scenarios/axios-compromise.sh

demo-shai-hulud: ## Replay the 2026-04-18 rampage-* worm against the running stack.
	docker compose up -d --build
	./scripts/demo-scenarios/shai-hulud.sh

demo-vercel: ## Replay the 2026-04-19 Vercel OAuth leak against the running stack.
	docker compose up -d --build
	./scripts/demo-scenarios/vercel-oauth.sh

demo-native: ## Same as demo-axios but routes parsing through the Rust sidecar (--profile native).
	RAMPART_PARSER_STRATEGY=native docker compose --profile native up -d --build
	./scripts/demo-scenarios/axios-compromise.sh

demo-down: ## Tear the demo stack down (all profiles) and prune its volumes.
	docker compose --profile native --profile full down -v --remove-orphans

# --- CI umbrella (Adım 8) --------------------------------------------------

.PHONY: ci
ci: lint test ## Run everything CI runs, locally.
	@echo "CI checks passed."

# --- Misc ------------------------------------------------------------------

.PHONY: clean
clean: ## Remove build artifacts (keeps node_modules and .yarn/cache).
	rm -rf dist build coverage
	rm -f engine/engine cli/rampart
	-cd native 2>/dev/null && cargo clean || true
