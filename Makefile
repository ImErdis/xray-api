VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/ImErdis/xray-api/internal/version.Version=$(VERSION) \
           -X github.com/ImErdis/xray-api/internal/version.Commit=$(COMMIT)

.PHONY: build test vet fmt run integration docker tidy

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/xray-api ./cmd/xray-api

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w internal cmd

tidy:
	go mod tidy

run: build
	./bin/xray-api serve -config examples/config.example.yaml

# Requires Docker; spins up a real Xray node. See test/integration.
integration:
	go test -tags integration -count=1 ./test/integration/...

docker:
	docker build -f deploy/Dockerfile -t xray-api:$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .
