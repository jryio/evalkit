.PHONY: build install test smoke release

# Static, reproducible builds: no cgo, no local paths in the binary.
STATIC = CGO_ENABLED=0
LDFLAGS = -s -w
PLATFORMS = darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

build:
	$(STATIC) go build -trimpath -ldflags="$(LDFLAGS)" -o bin/evalkit ./cmd/evalkit

install:
	$(STATIC) go install -trimpath -ldflags="$(LDFLAGS)" ./cmd/evalkit

# Unit tests: Go (with the race detector) and the framework-free JS core.
test:
	go vet ./...
	go test -race ./...
	node --test 'web/kit/core/*.test.js'

# Render every item of a running server: make smoke PORT=8790
smoke:
	tests/e2e/smoke.sh $(PORT)

# Static binaries for the common platforms, in dist/.
release: $(PLATFORMS)

$(PLATFORMS):
	$(STATIC) GOOS=$(word 1,$(subst /, ,$@)) GOARCH=$(word 2,$(subst /, ,$@)) \
		go build -trimpath -ldflags="$(LDFLAGS)" \
		-o dist/evalkit-$(word 1,$(subst /, ,$@))-$(word 2,$(subst /, ,$@))$(if $(findstring windows,$@),.exe) ./cmd/evalkit
