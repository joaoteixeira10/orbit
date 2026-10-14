# syntax=docker/dockerfile:1
# Orbit — one image: the Go API with the Vue SPA embedded.
# Multi-arch by cross-compiling (pure Go, CGO off), so amd64 + arm64 build
# natively on one runner, no QEMU. Same structure as PKI Management.

# ---------- Stage 1: build the Vue SPA ----------
# Pinned to the build platform: the SPA is arch-independent JS, so build it once
# natively (never under emulation) regardless of the target image arch.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
# npm ci: reproducible install from the lockfile (PKI used npm install).
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- Stage 2: build the Go binary (SPA embedded) ----------
FROM --platform=$BUILDPLATFORM golang:1.26.9 AS api
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
# Drop the built SPA into the embed directory so go:embed picks it up.
COPY --from=web /web/dist/ ./internal/webui/dist/

# Version stamping. .git is not in the build context, so these are passed as
# build args (Makefile `image` target, build.yml) and injected via -ldflags.
ARG VERSION=""
ARG COMMIT=""
ARG BUILD_TIME=""
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOFLAGS=-buildvcs=false
RUN VP=gitea.ubonito.org/jteixeira/orbit/api/internal/version; \
    LDFLAGS="-s -w"; \
    [ -n "$VERSION" ]    && LDFLAGS="$LDFLAGS -X $VP.Version=$VERSION"; \
    [ -n "$COMMIT" ]     && LDFLAGS="$LDFLAGS -X $VP.Commit=$COMMIT"; \
    [ -n "$BUILD_TIME" ] && LDFLAGS="$LDFLAGS -X $VP.BuildTime=$BUILD_TIME"; \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags "$LDFLAGS" -o /out/orbit ./cmd/server
# An empty /data for the runtime image: distroless has no shell to mkdir, and a
# volume mounted where the image has no directory is created root-owned, which
# the nonroot user can't write (SQLite then fails with "unable to open database
# file"). Docker seeds a new volume with this directory's ownership.
RUN mkdir -p /out/data

# ---------- Stage 3: minimal runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/orbit /orbit
COPY --from=api --chown=65532:65532 /out/data /data
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
# Liveness probe via the binary's own subcommand (distroless has no shell/curl).
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/orbit", "healthcheck"]
ENTRYPOINT ["/orbit"]
