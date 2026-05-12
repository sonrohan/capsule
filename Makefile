.PHONY: build install test

GO ?= go
VERSION ?= $(shell tr -d '[:space:]' < VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o capsule .

install:
	@$(GO) install -ldflags "$(LDFLAGS)" .
	@bin_dir="$$($(GO) env GOBIN)"; \
	if [ -z "$$bin_dir" ]; then bin_dir="$$($(GO) env GOPATH)/bin"; fi; \
	echo "Installed capsule to $$bin_dir/capsule"; \
	case ":$$PATH:" in \
		*":$$bin_dir:"*) echo "Run: capsule --help" ;; \
		*) echo "Add $$bin_dir to PATH to run capsule from anywhere:"; \
		   echo "  echo 'export PATH=\"$$bin_dir:\$$PATH\"' >> ~/.zshrc"; \
		   echo "Then reload your shell:"; \
		   echo "  exec zsh" ;; \
	esac

test:
	$(GO) test ./...
