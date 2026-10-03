GO       ?= $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
REGISTRY ?= docker.io/nskforward/mikwg
IMAGE    ?= $(REGISTRY)
VERSION  ?= 1.0.1
TAR      ?= awg-converter-arm64.tar
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build build-host test lint fmt vet image tar tar-nodocker push push-release clean bench race

all: test build

build: ## Cross-compile the converter for linux/arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" \
		-o bin/awg-converter ./cmd/awg-converter

build-host: ## Build for the host (local bench + tools)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/awg-converter ./cmd/awg-converter
	$(GO) build -trimpath -o bin/rscgen ./cmd/rscgen
	$(GO) build -trimpath -o bin/imagetool ./cmd/imagetool

test: ## Run unit tests
	$(GO) test ./...

race: ## Run tests with the race detector
	$(GO) test -race ./...

bench: ## Run transform benchmarks
	$(GO) test -run '^$$' -bench . -benchmem ./internal/awg

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

lint: vet ## gofmt check + vet
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

image: ## Build the container image for linux/arm64 (Docker daemon)
	docker buildx build --platform linux/arm64 -t $(IMAGE):$(VERSION) --load .

tar: image ## Save the container image as a tar for upload to RouterOS
	docker save $(IMAGE):$(VERSION) -o $(TAR)
	@echo "==> $(TAR) ready (upload via Winbox/Files)"

tar-nodocker: ## Build the image tar without a Docker daemon
	VERSION=$(VERSION) TAR=$(TAR) ./build-nodocker.sh

push: build ## Cross-compile and push :$(VERSION) to $(REGISTRY) (needs DOCKER_USERNAME/DOCKER_PASSWORD or docker login)
	$(GO) run ./cmd/imagetool \
		-binary bin/awg-converter \
		-image $(REGISTRY) \
		-version $(VERSION) \
		-out $(TAR) \
		-push

push-release: build ## Push both :$(VERSION) and :latest to $(REGISTRY)
	$(GO) run ./cmd/imagetool \
		-binary bin/awg-converter \
		-image $(REGISTRY) \
		-version $(VERSION) \
		-tags latest \
		-out $(TAR) \
		-push

clean:
	rm -rf bin dist $(TAR)
