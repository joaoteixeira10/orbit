# Orbit v1 — dev tasks. Nothing needs Go or Node on the host: every target
# runs in a pinned container (PKI Management's web-* targets used host npm;
# this one doesn't). Tool versions are pinned here on purpose.

GO_IMAGE      ?= golang:1.26.9
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

.PHONY: help go tidy build test vet lint lab-seed e2e-lab fmt vuln swag web-install web-lint web-typecheck web-test web-build check dev image release-image pull up up-build down set-version lbctl lb-agent

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
vet:             ## go vet ./...
	$(DOCKER_GO) go vet ./...
fmt:             ## gofmt -w
	$(DOCKER_GO) gofmt -l -w .
lbctl:           ## build lbctl for linux / macOS (amd64 + arm64) into dist/lbctl, with SHA256SUMS
	$(DOCKER_GO) sh -c 'set -e; VP=gitea.ubonito.org/jteixeira/orbit/api/internal/version; \
		for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		  CGO_ENABLED=0 GOOS=$${t%/*} GOARCH=$${t#*/} go build -ldflags "-s -w -X $$VP.Version=$(or $(VNUM),dev) -X $$VP.Commit=$(COMMIT)" \
		    -o ../dist/lbctl/lbctl-$${t%/*}-$${t#*/} ./cmd/lbctl; \
		done; cd ../dist/lbctl && sha256sum lbctl-* > SHA256SUMS && ls -l'
lb-agent:        ## build lb-agent for linux (amd64 + arm64) into dist/lb-agent, with SHA256SUMS
	$(DOCKER_GO) sh -c 'set -e; VP=gitea.ubonito.org/jteixeira/orbit/api/internal/version; \
		for t in linux/amd64 linux/arm64; do \
		  CGO_ENABLED=0 GOOS=$${t%/*} GOARCH=$${t#*/} go build -trimpath -ldflags "-s -w -X $$VP.Version=$(or $(VNUM),dev) -X $$VP.Commit=$(COMMIT)" \
		    -o ../dist/lb-agent/lb-agent-$${t%/*}-$${t#*/} ./cmd/lb-agent; \
		done; cd ../dist/lb-agent && sha256sum lb-agent-* > SHA256SUMS && ls -l'
lab-seed:        ## add the lab proxies and test users to the running dev instance (make dev)
	docker compose -f docker-compose.dev.yml exec -w /src/api api go run ./cmd/labseed /src/lab

e2e-lab:         ## Playwright against Orbit + a real nginx/lb-agent lab, all in compose (docker-compose.e2e.yml; removes its images after)
	docker compose -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e; \
	  rc=$$?; docker compose -f docker-compose.e2e.yml down -v --remove-orphans --rmi local >/dev/null 2>&1; exit $$rc

lint:            ## golangci-lint (pinned, same as CI)
	$(DOCKER_GO) sh -c 'command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION); golangci-lint run ./...'
vuln:            ## govulncheck (pinned)
	$(DOCKER_GO) sh -c 'go install golang.org/x/vuln/cmd/govulncheck@$(VULN_VERSION) && govulncheck ./...'
swag:            ## regenerate the OpenAPI spec (api/docs, committed)
	$(DOCKER_GO) sh -c 'go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) && swag init -g cmd/server/main.go -o docs'

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

check: vet test web-lint web-typecheck web-test  ## what CI's Test workflow runs (minus lint)

## --- Dev loop ---
dev:             ## API (air) + SPA (Vite) with live reload → http://localhost:5173
	docker compose -f docker-compose.dev.yml up

## --- Image / compose ---
VERSION    ?= $(shell git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
REGISTRY   ?= harbor.ubonito.org/proxy_manager/orbit
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
