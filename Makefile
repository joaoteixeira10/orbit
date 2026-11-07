# Orbit — dev tasks. Nothing needs Go or Node on the host: every target
# runs in a pinned container (PKI Management's web-* targets used host npm;
# this one doesn't). Tool versions are pinned here on purpose.

GO_IMAGE      ?= golang:1.26.10
NODE_IMAGE    ?= node:22-alpine
AIR_VERSION   ?= v1.67.1
SWAG_VERSION  ?= v1.16.6
LINT_VERSION  ?= v2.12.2
VULN_VERSION  ?= v1.1.4

DOCKER_GO = docker run --rm \
	-v "$(CURDIR)":/src \
	-v orbit-go-cache:/root/.cache/go-build \
	-v orbit-go-modcache:/go/pkg/mod \
	-v orbit-go-bin:/go/bin \
	-w /src/api \
	-e GOFLAGS=-buildvcs=false \
	$(GO_IMAGE)

# node_modules live in a named volume so the container's Linux binaries never
# mix with the host's.
DOCKER_NODE = docker run --rm \
	-v "$(CURDIR)":/src \
	-v orbit-web-node-modules:/src/web/node_modules \
	-w /src/web \
	$(NODE_IMAGE)

.PHONY: help go tidy build test vet lint fmt vuln swag crests web-install web-lint web-typecheck web-test web-build check dev image release-image pull up up-build down set-version

help:            ## list the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/'

## --- Go ---
go:              ## make go ARGS="version"
	$(DOCKER_GO) go $(ARGS)
tidy:            ## go mod tidy
	$(DOCKER_GO) go mod tidy
build:           ## go build ./...
	$(DOCKER_GO) go build ./...
test:            ## go test ./...
	$(DOCKER_GO) go test ./...
test-pg:         ## the API's tests on PostgreSQL (a throwaway postgres container)
	docker network create orbit-test-pg >/dev/null 2>&1 || true
	docker rm -f orbit-test-pg >/dev/null 2>&1 || true
	docker run -d --name orbit-test-pg --network orbit-test-pg -e POSTGRES_PASSWORD=pg -e POSTGRES_DB=orbit postgres:17-alpine >/dev/null
	for i in $$(seq 1 30); do docker exec orbit-test-pg pg_isready -U postgres >/dev/null 2>&1 && break; sleep 1; done
	docker run --rm --network orbit-test-pg -v "$(CURDIR)/api":/src -w /src -v orbit-go-modcache:/go/pkg/mod \
		-e ORBIT_TEST_POSTGRES='postgres://postgres:pg@orbit-test-pg:5432/orbit?sslmode=disable' $(GO_IMAGE) go test ./internal/api/ -count=1; \
		s=$$?; docker rm -f orbit-test-pg >/dev/null; exit $$s
vet:             ## go vet ./...
	$(DOCKER_GO) go vet ./...
fmt:             ## gofmt -w
	$(DOCKER_GO) gofmt -l -w .
lint:            ## golangci-lint (pinned)
	$(DOCKER_GO) sh -c 'command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION); golangci-lint run ./...'
vuln:            ## govulncheck (pinned)
	$(DOCKER_GO) sh -c 'go install golang.org/x/vuln/cmd/govulncheck@$(VULN_VERSION) && govulncheck ./...'
swag:            ## regenerate the OpenAPI spec (api/docs, committed)
	$(DOCKER_GO) sh -c 'go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) && swag init -g cmd/server/main.go -o docs'

CRESTS ?= ../assets/orbit/ubonito-orbit
crests:          ## bake the developer-theme crests from the owner's pack (../assets)
	docker run --rm -v "$(CURDIR)/..":/w -w /w/$(notdir $(CURDIR)) python:3.12-slim sh -c 'pip -q install fonttools brotli && python scripts/bake-crests.py $(CRESTS)'

## --- Frontend ---
web-install:     ## npm ci
	$(DOCKER_NODE) npm ci --no-audit --no-fund
web-lint:        ## eslint
	$(DOCKER_NODE) npm run lint
web-typecheck:   ## vue-tsc
	$(DOCKER_NODE) npm run typecheck
web-test:        ## vitest
	$(DOCKER_NODE) npm test
web-build:       ## vite build
	$(DOCKER_NODE) npm run build

check: vet test web-lint web-typecheck web-test  ## vet + tests + web lint/typecheck/tests (run with lint before merging)

## --- Dev loop ---
dev:             ## API (air) + SPA (Vite) with live reload → http://localhost:5180
	docker compose -f docker-compose.dev.yml up

## --- Image / compose ---
VERSION    ?= $(shell git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# Override with REGISTRY=… on the command line to push elsewhere.
REGISTRY   ?= ghcr.io/joaoteixeira10/orbit
PLATFORMS  ?= linux/amd64,linux/arm64
VNUM       := $(patsubst v%,%,$(VERSION))

image:           ## build orbit:dev locally
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) -t orbit:dev .

release-image:   ## multi-arch build + push (local fallback for the Build & Push workflow)
	docker buildx inspect orbit-builder >/dev/null 2>&1 || docker buildx create --name orbit-builder
	docker buildx build --builder orbit-builder --platform $(PLATFORMS) \
		--build-arg VERSION=$(VNUM) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(REGISTRY):v$(VNUM) -t $(REGISTRY):latest --push .

pull:            ## pull the published image
	docker compose pull
up:              ## start the production stack (docker-compose.yml)
	docker compose up -d
up-build:        ## build from source and start (stamped from git)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) docker compose -f docker-compose-build.yml up -d --build
down:            ## stop the stack
	docker compose down

set-version:     ## bump every version marker: make set-version VERSION=0.2.0
	./scripts/set-version.sh $(VNUM)
