# Releasing

How to cut a versioned release of Orbit: bump the version, build and push
the image, then tag and publish the release. Read it before your first
release and every time you're unsure. Every step is **manual**: nothing builds,
pushes, tags or publishes on a push or a merge.

> **Status:** no release yet. The first one will be **v0.1.0**: the version
> markers already say `0.1.0`, so the first run starts at step 2 once the
> repository is on Gitea and the Harbor secrets are set.

## TL;DR

```sh
# 0. main is green; pick a BARE semver, e.g. 0.2.0 (no leading "v").
git checkout main && git pull
git checkout -b release/v0.2.0
make set-version VERSION=0.2.0      # rewrites every version marker (runs on the host: bash + perl)
#    edit CHANGELOG.md: "## [Unreleased]" → "## [0.2.0] - YYYY-MM-DD",
#    and add a fresh, empty "## [Unreleased]" above it
git commit -am "release: 0.2.0"
#    push, open a PR, merge it to main once Test / E2E / Security are green
# 2. Gitea → Actions → "Build & Push" → branch main, version 0.2.0   (image :v0.2.0 + :latest)
# 3. Gitea → Actions → "Release"      → branch main, version 0.2.0   (tag v0.2.0 + release notes)
# 4. Check both registries, the release page, and /api/v1/version on a deployed instance.
```

## Why the order matters

**Bump → build → release**, always in that order.

- **Build & Push builds whatever the chosen branch holds.** The Go binary gets
  its version from the workflow's `version` input (`-ldflags`), so `/version`
  and the page's sidebar are right either way. But the **API reference**
  (`/api/docs`, `/api/v1/openapi.json`) is the committed `api/docs/` files,
  embedded at build time: without the bump, the image ships with the old
  version there.
- **Release tags the commit it runs on** (`main` HEAD) and lifts the release
  notes from `CHANGELOG.md` on that commit. The tag must be the commit the
  image was built from, with the markers bumped and the changelog rolled.
- **Release does not build.** Run it after Build & Push is green. Otherwise
  the release page (and the update check of every running instance, which
  reads the latest Gitea release every 6 hours) announces a version whose
  image doesn't exist yet.
- **`.env.example` pins `ORBIT_VERSION`.** New deployments copy it and pull that
  tag, so it should name an image that exists.

## Workflows

All in `.gitea/workflows/`.

| Workflow | File | Runs on | Does |
|---|---|---|---|
| Test | `test.yml` | pull requests, by hand | Go `gofmt`, `vet`, `golangci-lint`, build, tests with coverage; web lint, typecheck, Vitest |
| E2E | `e2e.yml` | pull requests, by hand | Builds the binary with the SPA embedded, boots it, runs Playwright |
| Security scan | `security.yml` | pull requests, by hand, Mondays 06:00 UTC | `govulncheck`, `npm audit --omit=dev --audit-level=high`, `gitleaks` over the full history |
| Build & Push | `build.yml` | **by hand only** | One multi-arch build, pushed to Harbor **and** the Gitea registry |
| Release | `release.yml` | **by hand only** | Tags the commit and creates the Gitea release with notes from `CHANGELOG.md` |

Secrets (repo → Settings → Actions → Secrets):

| Secret | Used by | Required |
|---|---|---|
| `HARBOR_REGISTRY_HOST` | Build & Push | yes — the Harbor host to log in to |
| `HARBOR_IMAGE` | Build & Push | yes — the full image path on Harbor, without a tag (e.g. `orbit/orbit`; match `ORBIT_IMAGE` in `.env.example` and `REGISTRY` in the Makefile, which are placeholders until you choose it) |
| `HARBOR_USERNAME` | Build & Push | yes |
| `HARBOR_PASSWORD` | Build & Push | yes |
| `PACKAGES_TOKEN` | Build & Push | no — a dedicated credential for the Gitea registry; without it the built-in Actions token is used (needs package write) |
| `RELEASE_TOKEN` | Release | no — a token with `write:repository`, if the built-in Actions token lacks the scope |

## 0. Pre-flight

- `main` is green and at the commit you want to ship.
- [`CHANGELOG.md`](../CHANGELOG.md) `[Unreleased]` says what's in it (it's filled as changes
  merge, not at release time).
- Pick the **bare** semver: patch (`1.0.1`) for fixes, minor (`1.1.0`) for
  features, major for breaking changes.
- The runner builds amd64 and arm64 and needs a few GB of free disk. (The
  workflow builds once and pushes all four tags because building once per
  registry ran the runner out of disk.)

## 1. Bump the version

```sh
make set-version VERSION=0.2.0
```

It runs `scripts/set-version.sh 0.2.0` (a leading `v` is stripped; anything
that isn't `X.Y.Z` is refused). It rewrites:

| File | Marker | Why |
|---|---|---|
| `web/package.json` | `"version"` | The SPA package |
| `web/package-lock.json` | the `orbit-web` entry's `"version"` | Kept in sync with `package.json` |
| `api/internal/version/version.go` | `Version = "…"` | Fallback for builds without `-ldflags` |
| `.env.example` | `ORBIT_VERSION=v…` | The image tag compose pulls by default |
| `api/cmd/server/main.go` | `@version` | The OpenAPI source annotation |
| `api/docs/docs.go`, `swagger.json`, `swagger.yaml` | the version field only | The committed, generated OpenAPI served at `/api/docs` |

It does **not** touch `CHANGELOG.md` and does not run `make swag`. If you
changed API annotations, run `make swag` first and commit the result, then bump.

## 1b. Roll the changelog

In the same branch:

1. Rename `## [Unreleased]` to `## [0.2.0] - YYYY-MM-DD`.
2. Add a new, empty `## [Unreleased]` above it.

Commit, push, open a PR, and merge it to `main` when the pull-request
workflows are green.

## 2. Build & Push the image

Gitea → **Actions → Build & Push → Run workflow** → branch **`main`**,
version **`0.2.0`**.

- Builds once with `docker/build-push-action`: `linux/amd64` and `linux/arm64`,
  cross-compiled (pure Go, CGO off), no QEMU.
- Stamps `VERSION` (the bare input), `COMMIT` (short SHA) and `BUILD_TIME`.
- Pushes four tags: `<HARBOR_IMAGE>:v0.2.0`, `<HARBOR_IMAGE>:latest`, and the
  same two on this Gitea's registry, at `<gitea host>/<owner>/<repo>` in lower
  case (the repo's **Packages**).
- Wait for it to be green before step 3. If it fails for lack of disk, free
  space on the runner host and run it again; nothing in the repo changes.

`make release-image` is a local fallback: the same multi-arch build with
`docker buildx`, pushed to `REGISTRY` (default
`harbor.ubonito.org/orbit/orbit`, a placeholder) as `:v<version>` and `:latest`.
It pushes to Harbor only, not to the Gitea registry, and needs a
`docker login` first. Run it from the bumped `main` with
`make release-image VERSION=0.2.0`.

## 3. Create the release

Gitea → **Actions → Release → Run workflow** → branch **`main`**,
version **`0.2.0`**.

- Creates tag and release **`v0.2.0`** on the commit the workflow runs on,
  through the Gitea API.
- The notes are a **"What's new"** block with the `## [0.2.0]` section of
  `CHANGELOG.md` (or, if that is empty, the `## [Unreleased]` section), then
  **"All changes since vX.Y.Z"**: every non-merge commit since the previous
  `vX.Y.Z` tag, so the first release lists **all changes** in the history.
- It fails unless Gitea answers `201`. A `409` means the tag or release exists.

## 4. Verify

- Harbor and the repo's **Packages** show `:v0.2.0` and `:latest`, both
  architectures.
- The repo's **Releases** shows `v0.2.0` with the notes.
- Deploy it (`ORBIT_VERSION=v0.2.0`, then `docker compose pull && docker compose up -d`)
  and check `/api/v1/version`, the version in the page's sidebar, and the
  version on `/api/docs`.

## Rules that bite

- **Bump before build.** Otherwise the API reference in the image shows the old
  version, and the tag doesn't match the markers.
- **Build before release.** Release never builds.
- **Bare version everywhere** (`0.2.0`). The workflows add the `v` to the tags.
- **Run both on `main`.** Both use the branch picked in the *Run workflow*
  dialog; Release tags exactly that commit.
- **`:latest` moves on every build.** Anyone running with `ORBIT_VERSION` unset
  gets the new image on their next pull. Don't use Build & Push to try things
  out; build locally with `make image` (tags `orbit:dev`, pushes nothing).
- **Nothing checks the input against the markers.** Type the same version you
  bumped to, in both workflows.
- **Start each workflow once.** Re-run a failed run (*Re-run*, or
  `POST …/actions/runs/<run>/rerun-failed-jobs`) *or* start a new one, not
  both: there's no API to cancel a run, and a second Build & Push pushes
  `:v0.2.0` and `:latest` again with a new digest.
- **The runner runs one job at a time** and its disk fills with Docker build
  cache: jobs late in a round can fail with `no space left on device`. Free it
  on the runner host with `docker builder prune -f`, then re-run only the
  failed jobs. Run `make check lint` locally before pushing to the release PR.
- **Roll the changelog heading.** If you forget, the notes still use
  `[Unreleased]`, but `CHANGELOG.md` itself is wrong for the next release.
- **Triggering by API** instead of the page (each is a `workflow_dispatch`):

  ```sh
  curl -s -X POST -H "Authorization: token <TOKEN>" -H 'Content-Type: application/json' \
    https://gitea.ubonito.org/api/v1/repos/jteixeira/Orbit/actions/workflows/build.yml/dispatches \
    -d '{"ref":"main","inputs":{"version":"0.2.0"}}'
  ```

  then the same with `release.yml`.

## Redoing a botched run

| What went wrong | Fix |
|---|---|
| Build & Push failed | Fix the cause (often disk space on the runner) and run it again with the same version. Nothing was tagged. |
| Built without the bump | Merge the bump to `main`, run Build & Push again with the same version: it overwrites `:v0.2.0` and `:latest`. |
| Release failed with `409` | The tag or release already exists. Delete them (below) or pick the next version. |
| Released the wrong commit or wrong notes | Delete the release, then the tag; fix `main`; run Build & Push, then Release, with the same version. |

Delete through the page (repo → Releases, repo → Tags) or the Gitea API:

```sh
# find the release id
curl -s -H "Authorization: token <TOKEN>" \
  https://gitea.ubonito.org/api/v1/repos/jteixeira/Orbit/releases/tags/v0.2.0 | jq .id
curl -s -X DELETE -H "Authorization: token <TOKEN>" \
  https://gitea.ubonito.org/api/v1/repos/jteixeira/Orbit/releases/<id>
curl -s -X DELETE -H "Authorization: token <TOKEN>" \
  https://gitea.ubonito.org/api/v1/repos/jteixeira/Orbit/tags/v0.2.0
```

> Redoing a version is safe only while nobody has pulled the bad image. Once
> someone has, ship the fix as the next patch version instead.
