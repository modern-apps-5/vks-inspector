BINARY      := vksinspect
PKG         := github.com/modern-apps-5/vks-inspector
CMD         := ./cmd/vksinspect
DIST        := dist

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/buildinfo.Version=$(VERSION) \
	-X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(PKG)/internal/buildinfo.Date=$(DATE)

# CGO_ENABLED=0 is load-bearing: the binary must run on a jump host or a
# customer laptop with no runtime dependencies. See docs/ADR/0001-go-single-binary.md
GOFLAGS_STATIC := CGO_ENABLED=0

.PHONY: all
all: build

.PHONY: build
build:
	$(GOFLAGS_STATIC) go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) $(CMD)

.PHONY: install
install:
	$(GOFLAGS_STATIC) go install -trimpath -ldflags '$(LDFLAGS)' $(CMD)

# Cross-compile the platforms a field engineer actually runs this from.
.PHONY: release
release:
	@mkdir -p $(DIST)
	@for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo ">> $$os/$$arch"; \
		$(GOFLAGS_STATIC) GOOS=$$os GOARCH=$$arch go build -trimpath \
			-ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-$$os-$$arch$$ext $(CMD) || exit 1; \
	done

# Default test run. Excludes anything build-tagged `integration` — those need a
# live lab and never run in CI. See docs/unit-test-coverage.md.
.PHONY: test
test:
	go test ./...

.PHONY: test-race
test-race:
	go test -race ./...

.PHONY: cover
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "wrote coverage.html"

# Requires a real vSphere/NSX/ALB lab plus credentials in the environment.
# Never wired into CI.
.PHONY: test-integration
test-integration:
	go test -tags=integration -count=1 -timeout=30m ./...

# Regenerate renderer golden files after an intentional output change.
# Review the diff before committing — that diff IS the output contract.
.PHONY: golden
golden:
	go test ./internal/renderers/... -update

# Regenerate the per-section summary tables in docs/REQUIREMENTS-MATRIX.md after
# adding or retargeting a check. `make test` fails if they are stale, so this is
# not optional bookkeeping. Review the diff — it is the coverage claim we make.
.PHONY: matrix
matrix:
	go test ./internal/docs/... -update

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed; skipping"; exit 0; }
	golangci-lint run ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: check
check: fmt vet test

.PHONY: clean
clean:
	rm -rf $(DIST) $(BINARY) coverage.out coverage.html

# Smoke: prove the skeleton runs end to end against the example config.
.PHONY: smoke
smoke: build
	./$(BINARY) check --config config/example.yaml --format terminal || true
	./$(BINARY) check --config config/example.yaml --format json || true
