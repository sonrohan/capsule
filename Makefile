.PHONY: build install test

GO ?= go

build:
	$(GO) build -o capsule .

install:
	@$(GO) install .
	@bin_dir="$$($(GO) env GOBIN)"; \
	if [ -z "$$bin_dir" ]; then bin_dir="$$($(GO) env GOPATH)/bin"; fi; \
	echo "Installed capsule to $$bin_dir/capsule"; \
	case ":$$PATH:" in \
		*":$$bin_dir:"*) echo "Run: capsule --help" ;; \
		*) echo "Add $$bin_dir to PATH to run capsule from anywhere:"; \
		   echo "  export PATH=\"$$bin_dir:$$PATH\"" ;; \
	esac

test:
	$(GO) test ./...
