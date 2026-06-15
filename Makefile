BINARY := llm-gateway
BINDIR := bin
SRCDIR := ./cmd/llm-gateway
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

export CGO_ENABLED := 0
GOFILES := $(shell find . -path './vendor' -prune -o -path './.git' -prune -o -name '*.go' -print)
GOIMPORTS_VERSION := v0.43.0

.PHONY: build build-all install uninstall clean test cover format format-check lint install-format-tools check-format-tools ci

build: format
	@mkdir -p $(BINDIR)
	go build $(LDFLAGS) -o $(BINDIR)/$(BINARY) $(SRCDIR)

build-all: format
	go build ./...

install: build
	@mkdir -p $(HOME)/.local/bin
	@rm -f $(HOME)/.local/bin/$(BINARY)
	cp $(BINDIR)/$(BINARY) $(HOME)/.local/bin/

uninstall:
	rm -f $(HOME)/.local/bin/$(BINARY)

install-format-tools:
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

check-format-tools:
	@command -v goimports >/dev/null || go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

format: check-format-tools
	@gofmt -w $(GOFILES)
	@goimports -w $(GOFILES)

format-check: check-format-tools
	@files="$$(gofmt -l $(GOFILES))"; \
	if [ -n "$$files" ]; then \
		echo "Go files are not formatted. Run: make format"; \
		echo "$$files"; \
		exit 1; \
	fi
	@files="$$(goimports -l $(GOFILES))"; \
	if [ -n "$$files" ]; then \
		echo "Go imports are not formatted. Run: make format"; \
		echo "$$files"; \
		exit 1; \
	fi

lint:
	go vet ./...
	@$(MAKE) format-check

test:
	go test ./...

cover:
	CGO_ENABLED=1 go test -race -covermode=atomic -timeout 120s \
		-coverpkg=./internal/... -coverprofile=coverage.out \
		./...

ci: format-check build-all lint
	$(MAKE) cover

clean:
	rm -rf $(BINDIR) coverage.out
