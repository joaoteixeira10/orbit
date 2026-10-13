#!/usr/bin/env bash
# Set the project version across every marker, so a release shows one consistent
# version everywhere. Usage: scripts/set-version.sh 1.20.0  (bare semver).
#
# Run this BEFORE "Build & Push" and commit the result, so the image is built
# from a tree that says the same version everywhere.
# Uses perl for portable in-place edits (consistent on macOS + Linux).
set -euo pipefail

V="${1:?usage: set-version.sh X.Y.Z (bare semver, no leading v)}"
V="${V#v}" # tolerate a leading "v"
if ! printf '%s' "$V" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "error: '$V' is not a bare semver like 1.20.0" >&2
  exit 1
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# SPA package (and its lockfile's own entry).
perl -0pi -e 's/("version":\s*")[^"]+(")/${1}'"$V"'${2}/' web/package.json
perl -0pi -e 's/("name":\s*"orbit-web",\s*"version":\s*")[^"]+(")/${1}'"$V"'${2}/g' web/package-lock.json
# Go fallback — overridden by -ldflags at build, kept in sync for un-stamped builds.
perl -pi -e 's/(Version\s*=\s*")[^"]+(")/${1}'"$V"'${2}/' api/internal/version/version.go
# Compose image-tag default.
perl -pi -e 's/^(ORBIT_VERSION=v).*/${1}'"$V"'/' .env.example
# Swagger API-doc version: the source annotation…
perl -pi -e 's/(\@version\s+).*/${1}'"$V"'/' api/cmd/server/main.go
# …and the committed generated docs (version field only).
perl -pi -e 's/(Version:\s*")[^"]+(",)/${1}'"$V"'${2}/' api/docs/docs.go
perl -0pi -e 's/("version":\s*")[^"]+(")/${1}'"$V"'${2}/' api/docs/swagger.json
perl -pi -e 's/^(  version:\s*).*/${1}'"$V"'/' api/docs/swagger.yaml

echo "Version set to $V across: package.json (+ lock), version.go, .env.example, swagger (annotation + docs)."
