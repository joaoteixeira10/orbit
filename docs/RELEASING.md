# Releasing

How to cut a versioned release of Orbit. The same code lives in two places: a
**Gitea** repo (where the tests run, and the images for our own servers are
built) and **GitHub** (`github.com/joaoteixeira10/orbit`, the public one, its
image on `ghcr.io`). Every commit goes to both; a release is made on **Gitea
first, then on GitHub**, on the same commit. Every step is **manual**: nothing
builds, pushes, tags or publishes on a push or a merge.

> **Status:** the latest release is v0.6.1 (2026-11-13; the first was v0.1.0,
> 2026-10-31). The next one starts at step 0 with the next version.

## TL;DR

```sh
# 0. main is green (locally and on Gitea); pick a BARE semver, e.g. 0.2.0.
make set-version VERSION=0.2.0      # rewrites every version marker (runs on the host: bash + perl)
#    edit CHANGELOG.md: "## [Unreleased]" → "## [0.2.0] - YYYY-MM-DD",
#    and add a fresh, empty "## [Unreleased]" above it
git commit -am "release: 0.2.0" && git push          # one push: Gitea and GitHub
# 2. Gitea  → Actions → "Build & Push" → main, 0.2.0   (Harbor + Gitea registry :v0.2.0 + :latest)
# 3. Gitea  → Actions → "Release"      → main, 0.2.0   (tag v0.2.0 + release notes)
# 4. GitHub → Actions → "Build & Push" → main, 0.2.0   (ghcr.io :v0.2.0 + :latest)
# 5. GitHub → Actions → "Release"      → main, 0.2.0   (tag v0.2.0 + release notes)
# 6. Check the registries, both release pages, and /api/v1/version on a deployed instance.
```

## One repo, two remotes

The local clone pushes to both forges at once:

```sh
git remote set-url --add --push origin <gitea repo's ssh URL>
git remote set-url --add --push origin git@github.com:joaoteixeira10/orbit.git
git remote -v    # origin: fetch from Gitea, push to both
```

Each forge reads only its own workflows: Gitea runs `.gitea/workflows/` (and
ignores `.github/` while that folder exists), GitHub runs `.github/workflows/`.

## Why the order matters

**Bump → build → release**, always in that order, on each forge.

- **Build & Push builds whatever the chosen branch holds.** The Go binary gets
  its version from the workflow's `version` input (`-ldflags`), so `/version`
  and the page's sidebar are right either way. But the **API reference**
  (`/api/docs`, `/api/v1/openapi.json`) is the committed `api/docs/` files,
  embedded at build time: without the bump, the image ships with the old
  version there.
- **Release tags the commit it runs on** (`main` HEAD) and takes the release
  notes from `CHANGELOG.md` on that commit. The tag must be the commit the
  image was built from, with the markers bumped and the changelog rolled —
  and the same commit on both forges.
- **Release does not build.** Run it after Build & Push is green. Otherwise
  the release page (and the update check of every running instance, which
  reads the latest release every 6 hours) announces a version whose image
  doesn't exist yet.
- **Gitea before GitHub.** The tests run on Gitea; GitHub only builds and
  releases what Gitea already released.
- **`.env.example` pins `ORBIT_VERSION`** (and `ORBIT_IMAGE`, the ghcr one).
  New deployments copy it and pull that tag, so it should name an image that
  exists — on ghcr, which is released last.

## Which releases an image checks

Each Build & Push stamps the repo it runs in (`REPO_URL` build argument →
`version.RepositoryURL`): the About link and the "newer release?" check of
images from **Gitea** read that Gitea's releases; images from **GitHub** read
GitHub's. A build without the stamp (`make image`, a build from source) reads
GitHub's. `REPO_URL` in the environment overrides it at run time.

## Workflows

| Forge | Workflow | File | Runs on | Does |
|---|---|---|---|---|
| Gitea | Test | `.gitea/workflows/test.yml` | pull requests, by hand | Go `gofmt`, `vet`, `golangci-lint`, build, tests with coverage; web lint, typecheck, Vitest |
| Gitea | E2E | `.gitea/workflows/e2e.yml` | pull requests, by hand | Builds the binary with the SPA embedded, boots it, runs Playwright |
| Gitea | Security scan | `.gitea/workflows/security.yml` | pull requests, by hand, Mondays 06:00 UTC | `govulncheck`, `npm audit --omit=dev --audit-level=high`, `gitleaks` over the full history |
| Gitea | Build & Push | `.gitea/workflows/build.yml` | **by hand only** | One multi-arch build, pushed to Harbor **and** the Gitea registry |
| Gitea | Release | `.gitea/workflows/release.yml` | **by hand only** | Tags the commit and creates the Gitea release with notes from `CHANGELOG.md` |
| GitHub | Build & Push | `.github/workflows/build.yml` | **by hand only** | One multi-arch build, pushed to `ghcr.io/<owner>/<repo>` |
| GitHub | Release | `.github/workflows/release.yml` | **by hand only** | Tags the commit and creates the GitHub release with notes from `CHANGELOG.md` |

GitHub runs no tests: the same commits are tested locally and on Gitea.

**Gitea:** repo → Settings → Actions → **Variables** or **Secrets**. The
Harbor host, image and username can be either (a variable wins when both are
set); passwords and tokens are secrets only.

| Name | Used by | Required |
|---|---|---|
| `HARBOR_REGISTRY_HOST` | Build & Push | yes — the Harbor host to log in to |
| `HARBOR_IMAGE` | Build & Push | yes — the full image path on Harbor, host included, without a tag (what your servers set as `ORBIT_IMAGE`). Without the host the push goes to Docker Hub |
| `HARBOR_USERNAME` | Build & Push | yes |
| `HARBOR_PASSWORD` | Build & Push | yes — a secret |
| `PACKAGES_TOKEN` | Build & Push | no — a dedicated credential for the Gitea registry; without it the built-in Actions token is used (needs package write) |
| `RELEASE_TOKEN` | Release | no — a token with `write:repository`, if the built-in Actions token lacks the scope |

**GitHub:** nothing to set — both workflows use the built-in `GITHUB_TOKEN`.

## 0. Pre-flight

- `main` is green (locally, and Test / E2E / Security on Gitea) and at the
  commit you want to ship.
- [`CHANGELOG.md`](../CHANGELOG.md) `[Unreleased]` says what's in it (it's filled as changes
  merge, not at release time).
- Pick the **bare** semver: patch (`1.0.1`) for fixes, minor (`1.1.0`) for
  features, major for breaking changes.
- The Gitea runner builds amd64 and arm64 and needs a few GB of free disk.

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

1. Rename `## [Unreleased]` to `## [0.2.0] - YYYY-MM-DD`.
2. Add a new, empty `## [Unreleased]` above it.

Commit (`release: 0.2.0`) and push — to both remotes.

## 2. Gitea: Build & Push

Gitea → **Actions → Build & Push → Run workflow** → branch **`main`**,
version **`0.2.0`**.

- Builds once with `docker/build-push-action`: `linux/amd64` and `linux/arm64`,
  cross-compiled (pure Go, CGO off), no QEMU.
- Stamps `VERSION` (the bare input), `COMMIT` (short SHA), `BUILD_TIME` and
  `REPO_URL` (this Gitea repo).
- Pushes four tags: `<HARBOR_IMAGE>:v0.2.0`, `<HARBOR_IMAGE>:latest`, and the
  same two on this Gitea's registry, at `<gitea host>/<owner>/<repo>` in lower
  case (the repo's **Packages**).
- Wait for it to be green before step 3. If it fails for lack of disk, free
  space on the runner host and run it again; nothing in the repo changes.

## 3. Gitea: Release

Gitea → **Actions → Release → Run workflow** → branch **`main`**,
version **`0.2.0`**.

- Creates tag and release **`v0.2.0`** on the commit the workflow runs on,
  through the Gitea API.
- The notes are a **"What's new"** block with the `## [0.2.0]` section of
  `CHANGELOG.md` (or, if that is empty, the `## [Unreleased]` section), then
  **"All changes since vX.Y.Z"**: every non-merge commit since the previous
  `vX.Y.Z` tag.
- It fails unless Gitea answers `201`. A `409` means the tag or release exists.

The tag comes back to your clone with `git fetch --tags`; it isn't pushed to
GitHub by hand — GitHub's Release makes its own on the same commit.

## 4. GitHub: Build & Push

GitHub → **Actions → Build & Push → Run workflow** → branch **`main`**,
version **`0.2.0`** (a leading `v` is accepted and stripped).

- The same build, stamped with `REPO_URL` = this GitHub repo.
- Pushes `ghcr.io/<owner>/<repo>:v0.2.0` and `:latest`, in lower case (the
  repo's **Packages**). A pre-release such as `0.2.0-rc.1` gets only
  `:v0.2.0-rc.1`, never `:latest`.

## 5. GitHub: Release

GitHub → **Actions → Release → Run workflow** → branch **`main`**,
version **`0.2.0`**.

- Creates tag and release **`v0.2.0`** on the commit the workflow runs on, with
  the same notes as Gitea's.
- A pre-release is marked as such and never becomes **Latest**, so the update
  check doesn't offer it.
- It fails early if tag `v0.2.0` already exists — it does when you've pushed
  your local tags (`git push --tags`) to GitHub: don't.

## 6. Verify

- Harbor and the Gitea repo's **Packages** show `:v0.2.0` and `:latest`, both
  architectures; so does the GitHub repo's **Packages** (ghcr).
- Both **Releases** pages show `v0.2.0` with the notes, on the same commit.
- Deploy it (`ORBIT_VERSION=v0.2.0`, then `docker compose pull && docker compose up -d`)
  and check `/api/v1/version`, the version in the page's sidebar, the version
  on `/api/docs`, and that About links to the repo the image came from.

## Rules that bite

- **Bump before build.** Otherwise the API reference in the image shows the old
  version, and the tag doesn't match the markers.
- **Build before release.** Release never builds.
- **Gitea, then GitHub, same commit.** Don't push anything to `main` between
  the two.
- **Bare version everywhere** (`0.2.0`). The workflows add the `v` to the tags.
- **Run all four on `main`.** They use the branch picked in the *Run workflow*
  dialog; Release tags exactly that commit.
- **`:latest` moves on every build.** Anyone running with `ORBIT_VERSION` unset
  gets the new image on their next pull. Don't use Build & Push to try things
  out; build locally with `make image` (tags `orbit:dev`, pushes nothing).
- **Nothing checks the input against the markers.** Type the same version you
  bumped to, in every workflow.
- **Start each workflow once.** Re-run a failed run *or* start a new one, not
  both: a second Build & Push pushes `:v0.2.0` and `:latest` again with a new
  digest (on Gitea there's no API to cancel a run).
- **The Gitea runner runs one job at a time** and its disk fills with Docker
  build cache: jobs late in a round can fail with `no space left on device`.
  Free it on the runner host with `docker builder prune -f`, then re-run only
  the failed jobs. Run `make check lint` locally before pushing.
- **Roll the changelog heading.** If you forget, the notes still use
  `[Unreleased]`, but `CHANGELOG.md` itself is wrong for the next release.
- **Triggering from the terminal** instead of the page (each is a
  `workflow_dispatch`):

  ```sh
  # Gitea (a token with write:repository)
  curl -s -X POST -H "Authorization: token <TOKEN>" -H 'Content-Type: application/json' \
    https://<gitea host>/api/v1/repos/<owner>/<repo>/actions/workflows/build.yml/dispatches \
    -d '{"ref":"main","inputs":{"version":"0.2.0"}}'
  #   then the same with release.yml
  # GitHub
  gh workflow run build.yml --ref main -f version=0.2.0
  gh workflow run release.yml --ref main -f version=0.2.0   # once the build is green
  ```

## Redoing a botched run

| What went wrong | Fix |
|---|---|
| Build & Push failed | Fix the cause (on Gitea often disk space on the runner) and run it again with the same version. Nothing was tagged. |
| Built without the bump | Push the bump to `main`, run Build & Push again with the same version: it overwrites `:v0.2.0` and `:latest`. |
| Release failed: the tag exists (Gitea `409`, GitHub "already exists") | Delete the release and tag (below) or pick the next version. |
| Released the wrong commit or wrong notes | Delete the release, then the tag; fix `main`; run Build & Push, then Release, with the same version — on each forge that has it. |

Delete through the pages (repo → Releases, repo → Tags), or:

```sh
# Gitea: find the release id, then delete the release and the tag
curl -s -H "Authorization: token <TOKEN>" \
  https://<gitea host>/api/v1/repos/<owner>/<repo>/releases/tags/v0.2.0 | jq .id
curl -s -X DELETE -H "Authorization: token <TOKEN>" \
  https://<gitea host>/api/v1/repos/<owner>/<repo>/releases/<id>
curl -s -X DELETE -H "Authorization: token <TOKEN>" \
  https://<gitea host>/api/v1/repos/<owner>/<repo>/tags/v0.2.0
# GitHub
gh release delete v0.2.0 --cleanup-tag --yes
git fetch --prune --prune-tags
```

> Redoing a version is safe only while nobody has pulled the bad image. Once
> someone has, ship the fix as the next patch version instead.
