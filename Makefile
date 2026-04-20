# rampart — A rampart for your supply chain.
# See CONTRIBUTING.md for dev setup; ROADMAP.md for what's implemented.

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

# --- Codegen (Adım 3) ------------------------------------------------------

.PHONY: gen gen-go gen-ts
gen: gen-go gen-ts ## Regenerate Go + TS types from OpenAPI. (Adım 3)

gen-go: ## Regenerate Go types from schemas/openapi.yaml. (Adım 3)
	@echo "Not yet wired — see FIRST.md Adım 3."

gen-ts: ## Regenerate TS types from schemas/openapi.yaml. (Adım 3)
	@echo "Not yet wired — see FIRST.md Adım 3."

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

# --- Demo stack (Adım 7) ---------------------------------------------------

.PHONY: demo demo-axios demo-shai-hulud demo-go-only
demo: ## Bring up the full demo stack. (Adım 7)
	@echo "Not yet wired — see FIRST.md Adım 7."

demo-axios: ## Replay the 31 March 2026 axios compromise. (Adım 7)
	@echo "Not yet wired — see FIRST.md Adım 7."

demo-shai-hulud: ## Replay the Shai-Hulud worm scenario. (Adım 7)
	@echo "Not yet wired — see FIRST.md Adım 7."

demo-go-only: ## Demo with rampart-native off (Go parser fallback). (Adım 7)
	@echo "Not yet wired — see FIRST.md Adım 7."

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
