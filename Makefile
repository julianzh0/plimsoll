NAME := plimsoll
GO_VERSION := $(shell sed -n 's/^go //p' go.mod)
GOLANGCI_LINT_VERSION := 2.15.0
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.9.0
# The zig that builds the grammar modules; internal/parse/grammars/pins.txt records its archives and every module.
ZIG_VERSION := 0.17.1
# Space-separated project-relative paths of agent files that may be committed.
AGENT_FILES_ALLOWED ?= CLAUDE.md
COUNT ?= 20
TAGS := -tags integration,e2e
# One test package at a time: a test binary that parses compiles grammars under the race detector, up to 1.2 GB for one
# binary (measured 2026-11-16, Swift's grammar alone about 720 MB); at Go's default count the whole suite passed 3.7 GB.
GO_TEST := go test -count=1 -race -shuffle=on -p 1
# Coverage of the three test levels, merged: the packages measured leave out the test helpers.
COVERDIR := $(CURDIR)/bin/cover
COVERPKG = $(shell go list ./... | grep -v -e /snapshottest -e /gitrepotest -e /parsetest | paste -sd, -)
GO_COVER = -cover -coverpkg=$(COVERPKG)
BINDIR ?= $(shell go env GOPATH)/bin

# Succeeds only when this directory is itself a Git top level, never a folder inside a parent repository.
IS_OWN_REPO = top=$$(git rev-parse --show-toplevel 2>/dev/null) && [ "$$(cd "$$top" && pwd -P)" = "$$(pwd -P)" ]

export GOTOOLCHAIN := local
export GOFLAGS := -mod=readonly

.NOTPARALLEL:
.PHONY: fix qa unit integration e2e cover cover-clean cover-report flake lint build run hooks check-tools fmt-check vet tidy-check fix-check vuln agent-files install-golangci-lint grammars grammars-check zig-check

qa: check-tools fmt-check vet vet-windows lint tidy-check fix-check agent-files cover vuln

fix:
	golangci-lint fmt
	golangci-lint run --fix
	go fix $(TAGS) ./...
	go mod tidy

unit:
	@mkdir -p $(COVERDIR)
	$(GO_TEST) $(GO_COVER) ./... -args -test.gocoverdir=$(COVERDIR)

integration:
	@mkdir -p $(COVERDIR)
	$(GO_TEST) $(GO_COVER) -tags integration -run '^TestIntegration' ./... -args -test.gocoverdir=$(COVERDIR)

# The end-to-end tests build the program with coverage when PLIMSOLL_COVERDIR is set, and the program writes there.
e2e:
	@mkdir -p $(COVERDIR)
	PLIMSOLL_COVERDIR=$(COVERDIR) $(GO_TEST) $(GO_COVER) -tags e2e -run '^TestE2E' ./... -args -test.gocoverdir=$(COVERDIR)

# cover runs the three levels on fresh coverage data and prints it; qa runs it, so the tests run once.
cover: cover-clean unit integration e2e cover-report

cover-clean:
	rm -rf $(COVERDIR)

# Coverage is information, never a gate: the percentage per package, then the total.
cover-report:
	@go tool covdata percent -i=$(COVERDIR) | sed 's/^[[:space:]]*github.com\/julianzh0\/plimsoll\///' | sort
	@go tool covdata textfmt -i=$(COVERDIR) -o=$(COVERDIR)/profile.txt
	@go tool cover -func=$(COVERDIR)/profile.txt | tail -1

flake:
	@if [ -z "$(TESTS)" ]; then echo "usage: make flake TESTS='<regexp>' [COUNT=20]"; exit 2; fi
	$(GO_TEST) -count=$(COUNT) $(TAGS) -run '$(TESTS)' ./...

lint: check-tools
	golangci-lint run

# grammars builds the grammar modules from the sources pins.txt names and records their hashes; grammars-check
# rebuilds them and fails when one differs from the pins or from the committed file. Neither is part of qa: they need
# zig and the network.
grammars: zig-check
	sh internal/parse/grammars/build.sh record

grammars-check: zig-check
	sh internal/parse/grammars/build.sh check

zig-check:
	@[ "$$(zig version 2>/dev/null)" = "$(ZIG_VERSION)" ] || { echo "the grammar modules need zig $(ZIG_VERSION)"; exit 1; }

build:
	go build -trimpath -buildvcs=false -o bin/$(NAME) ./cmd/$(NAME)

run: build
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; exec bin/$(NAME)

hooks:
	@$(IS_OWN_REPO) || { echo "make hooks needs this project to be its own Git repository"; exit 1; }
	git config core.hooksPath .githooks

check-tools:
	@test "$$(go env GOVERSION)" = "go$(GO_VERSION)" || { echo "Go $(GO_VERSION) is required, found $$(go env GOVERSION)"; exit 1; }
	@test "$$(golangci-lint version --short)" = "$(GOLANGCI_LINT_VERSION)" || { echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required, found $$(golangci-lint version --short)"; exit 1; }

fmt-check:
	golangci-lint fmt --diff

vet:
	go vet $(TAGS) ./...

# vet-windows checks the Windows build from any machine: no Windows runner tests it.
vet-windows:
	GOOS=windows GOARCH=amd64 go vet $(TAGS) ./...
	GOOS=windows GOARCH=arm64 go build -o /dev/null ./cmd/plimsoll

tidy-check:
	go mod tidy -diff

fix-check:
	go fix -diff $(TAGS) ./...

vuln:
	go run $(GOVULNCHECK) ./...

agent-files:
	@$(IS_OWN_REPO) && mode=git || mode=scan; \
	if [ "$$mode" = git ]; then \
		found=$$(git ls-files -- ':(top,glob)**/CLAUDE.md' ':(top,glob)**/AGENTS.md' ':(top,glob)**/.claude' ':(top,glob)**/.claude/**' ':(top,glob)**/.cursor' ':(top,glob)**/.cursor/**' ':(top,glob)**/.vscode/tasks.json') || { echo "agent-files: could not list tracked files"; exit 1; }; \
	else \
		found=$$(find . -name .git -prune -o ! -type d \( -name CLAUDE.md -o -name AGENTS.md -o -name .claude -o -name .cursor -o -path '*/.claude/*' -o -path '*/.cursor/*' -o -path '*/.vscode/tasks.json' \) -print) || { echo "agent-files: could not scan files"; exit 1; }; \
		found=$$(printf '%s\n' "$$found" | sed 's|^\./||'); \
	fi; \
	status=0; set -f; IFS=$$(printf '\n_'); IFS=$${IFS%_}; \
	for file in $$found; do \
		allowed=no; \
		for entry in $(AGENT_FILES_ALLOWED); do [ "$$file" = "$$entry" ] && allowed=yes; done; \
		if [ "$$allowed" = no ]; then echo "unlisted agent file: $$file"; status=1; fi; \
	done; \
	exit $$status

install-golangci-lint:
	@command -v go >/dev/null 2>&1 || { echo "Go is required"; exit 1; }; \
	if [ -d "$(BINDIR)/golangci-lint" ]; then echo "$(BINDIR)/golangci-lint is a directory"; exit 1; fi; \
	name=golangci-lint-$(GOLANGCI_LINT_VERSION)-$$(go env GOHOSTOS)-$$(go env GOHOSTARCH); \
	base=https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION); \
	work="$(BINDIR)/.golangci-lint-install.$$$$"; \
	mkdir -p "$(BINDIR)" && trap 'rm -f "$$work.tar.gz" "$$work.sum" "$$work.bin"' EXIT && \
	curl --fail --silent --show-error --location --max-time 300 -o "$$work.tar.gz" "$$base/$$name.tar.gz" && \
	hash=$$(curl --fail --silent --show-error --location --max-time 60 "$$base/golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt" | grep " $$name.tar.gz\$$" | cut -d' ' -f1) && \
	test -n "$$hash" && \
	printf '%s  %s\n' "$$hash" "$$work.tar.gz" > "$$work.sum" && \
	if command -v sha256sum >/dev/null 2>&1; then sha256sum --check --status "$$work.sum"; else shasum -a 256 --check --status "$$work.sum"; fi && \
	tar -xzOf "$$work.tar.gz" "$$name/golangci-lint" > "$$work.bin" && \
	chmod 755 "$$work.bin" && \
	mv "$$work.bin" "$(BINDIR)/golangci-lint"
