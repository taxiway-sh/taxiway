.PHONY: build snapshot completion completion-bash completion-fish completion-zsh test test-unit test-e2e test-e2e-only test-e2e-up test-e2e-prepare-run test-e2e-phase-by-phase test-e2e-codex test-e2e-codex-up test-e2e-codex-prepare-run test-e2e-codex-phase-by-phase test-e2e-claude-code test-e2e-claude-code-up test-e2e-claude-code-prepare-run test-e2e-claude-code-phase-by-phase test-e2e-gastown test-e2e-gastown-up test-e2e-gastown-prepare-run test-e2e-gastown-phase-by-phase lint lint-scripts test-scripts

build:
	go build ./...

E2E_TIMEOUT ?= 1200s

snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
	@os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	arch=$$(uname -m); \
	case "$$arch" in \
		arm64|aarch64) arch=arm64 ;; \
		x86_64|amd64) arch=amd64 ;; \
		*) echo "unsupported architecture: $$arch"; exit 1 ;; \
	esac; \
	bin=$$(find dist -path "*/taxiway_$${os}_$${arch}_*/taxiway" -type f -perm -111 | head -n 1); \
	if [ -z "$$bin" ]; then \
		echo "snapshot binary not found for $$os/$$arch"; \
		exit 1; \
	fi; \
	install -m 0755 "$$bin" ./taxiway; \
	./taxiway version

completion:
	@shell_name=$$(basename "$${SHELL:-}"); \
	case "$$shell_name" in \
		bash) $(MAKE) completion-bash ;; \
		fish) $(MAKE) completion-fish ;; \
		zsh) $(MAKE) completion-zsh ;; \
		*) echo "unsupported shell: $${shell_name:-unknown}"; echo "Use make completion-bash, completion-fish, or completion-zsh."; exit 1 ;; \
	esac

completion-bash:
	@mkdir -p "$$HOME/.bash_completion.d"
	./taxiway completion bash > "$$HOME/.bash_completion.d/taxiway"
	@echo "wrote $$HOME/.bash_completion.d/taxiway"

completion-fish:
	@mkdir -p "$$HOME/.config/fish/completions"
	./taxiway completion fish > "$$HOME/.config/fish/completions/taxiway.fish"
	@echo "wrote $$HOME/.config/fish/completions/taxiway.fish"

completion-zsh:
	@mkdir -p "$$HOME/.zsh/completions"
	./taxiway completion zsh > "$$HOME/.zsh/completions/_taxiway"
	@echo "wrote $$HOME/.zsh/completions/_taxiway"

test: test-unit

test-unit:
	go test ./...

## test-e2e: run end-to-end tests (requires a running Docker daemon)
## Usage: make test-e2e
test-e2e:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) ./...

## test-e2e-only: run only end-to-end tests (requires relevant local services)
## Usage: make test-e2e-only
test-e2e-only:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_' ./...

## test-e2e-up: run only up end-to-end tests
## Usage: make test-e2e-up
test-e2e-up:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_Orchestrator.*_Up$$' ./internal/cli

## test-e2e-prepare-run: run only prepare+run end-to-end tests
## Usage: make test-e2e-prepare-run
test-e2e-prepare-run:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_Orchestrator.*_PrepareRun$$' ./internal/cli

## test-e2e-phase-by-phase: run only phase-by-phase end-to-end tests
## Usage: make test-e2e-phase-by-phase
test-e2e-phase-by-phase:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_Orchestrator.*_PhaseByPhase$$' ./internal/cli

## test-e2e-codex: run Codex orchestrator end-to-end tests
## Usage: make test-e2e-codex
test-e2e-codex: test-e2e-codex-up test-e2e-codex-prepare-run test-e2e-codex-phase-by-phase

## test-e2e-codex-up: run Codex up end-to-end test
## Usage: make test-e2e-codex-up
test-e2e-codex-up:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorCodex_Up$$' ./internal/cli

## test-e2e-codex-prepare-run: run Codex prepare+run end-to-end test
## Usage: make test-e2e-codex-prepare-run
test-e2e-codex-prepare-run:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorCodex_PrepareRun$$' ./internal/cli

## test-e2e-codex-phase-by-phase: run Codex phase-by-phase end-to-end test
## Usage: make test-e2e-codex-phase-by-phase
test-e2e-codex-phase-by-phase:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorCodex_PhaseByPhase$$' ./internal/cli

## test-e2e-claude-code: run Claude Code orchestrator end-to-end tests
## Usage: make test-e2e-claude-code
test-e2e-claude-code: test-e2e-claude-code-up test-e2e-claude-code-prepare-run test-e2e-claude-code-phase-by-phase

## test-e2e-claude-code-up: run Claude Code up end-to-end test
## Usage: make test-e2e-claude-code-up
test-e2e-claude-code-up:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorClaudeCode_Up$$' ./internal/cli

## test-e2e-claude-code-prepare-run: run Claude Code prepare+run end-to-end test
## Usage: make test-e2e-claude-code-prepare-run
test-e2e-claude-code-prepare-run:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorClaudeCode_PrepareRun$$' ./internal/cli

## test-e2e-claude-code-phase-by-phase: run Claude Code phase-by-phase end-to-end test
## Usage: make test-e2e-claude-code-phase-by-phase
test-e2e-claude-code-phase-by-phase:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorClaudeCode_PhaseByPhase$$' ./internal/cli

## test-e2e-gastown: run Gastown orchestrator end-to-end tests
## Usage: make test-e2e-gastown
test-e2e-gastown: test-e2e-gastown-up test-e2e-gastown-prepare-run test-e2e-gastown-phase-by-phase

## test-e2e-gastown-up: run Gastown up end-to-end test
## Usage: make test-e2e-gastown-up
test-e2e-gastown-up:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorGastown_Up$$' ./internal/cli

## test-e2e-gastown-prepare-run: run Gastown prepare+run end-to-end test
## Usage: make test-e2e-gastown-prepare-run
test-e2e-gastown-prepare-run:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorGastown_PrepareRun$$' ./internal/cli

## test-e2e-gastown-phase-by-phase: run Gastown phase-by-phase end-to-end test
## Usage: make test-e2e-gastown-phase-by-phase
test-e2e-gastown-phase-by-phase:
	go test -v -tags=e2e -count=1 -timeout=$(E2E_TIMEOUT) -run '^TestE2E_OrchestratorGastown_PhaseByPhase$$' ./internal/cli

lint: lint-scripts
	@out=$$(find . -name '*.go' -not -path './vendor/*' \
		| xargs gofmt -l); \
	if [ -n "$$out" ]; then \
		echo "gofmt: the following files need formatting:"; \
		echo "$$out"; \
		exit 1; \
	fi
	@echo "gofmt: all files are properly formatted"

lint-scripts:
	@bad=$$(grep -rn \
		-E '^\s*(export\s+)?(ANTHROPIC_API_KEY|OPENAI_API_KEY|DOCKER_TOKEN)=' \
		orchestrators/*/install.sh orchestrators/*/verify.sh 2>/dev/null || true); \
	if [ -n "$$bad" ]; then \
		echo "$$bad"; \
		echo "FAIL: install/verify scripts must not assign credentials."; \
		exit 1; \
	fi
	@echo "lint-scripts: OK"

test-scripts: ## Run shell script tests
	@set -e; \
	find tests/scripts -type f -print | sort | grep -E '/test_[^/]+\.sh$$' | while IFS= read -r script; do \
		echo "==> $$script"; \
		bash "$$script"; \
	done
