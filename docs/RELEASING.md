# Releasing

How to cut a versioned release of Orbit: bump the version, build and push
the image, then tag and publish the release. Read it before your first
release and every time you're unsure. Every step is **manual**: nothing builds,
pushes, tags or publishes on a push or a merge.

> **Status:** the latest release is v0.5.0 (2026-11-07; the first was v0.1.0,
> 2026-10-31). The next one starts at step 0 with the next version.

## TL;DR

```sh
# 0. main is green; pick a BARE semver, e.g. 0.2.0 (no leading "v").
git checkout main && git pull
git checkout -b release/v0.2.0
make set-version VERSION=0.2.0      # rewrites every version marker (runs on the host: bash + perl)
#    edit CHANGELOG.md: "## [Unreleased]" → "## [0.2.0] - YYYY-MM-DD",
#    and add a fresh, empty "## [Unreleased]" above it
git commit -am "release: 0.2.0"
#    run `make check lint` locally, push, open a PR, merge it to main
# 2. GitHub → Actions → "Build & Push" → branch main, version 0.2.0   (image :v0.2.0 + :latest)
# 3. gh release create v0.2.0 --target main …  (tag v0.2.0 + release notes, see step 3)
# 4. Check the package, the release page, and /api/v1/version on a deployed instance.
```

## Why the order matters

**Bump → build → release**, always in that order.

- **Build & Push builds whatever the chosen branch holds.** The Go binary gets
  its version from the workflow's `version` input (`-ldflags`), so `/version`
  and the page's sidebar are right either way. But the **API reference**
  (`/api/docs`, `/api/v1/openapi.json`) is the committed `api/docs/` files,
  embedded at build time: without the bump, the image ships with the old
  version there.
- **The release tags the commit you target** (`main` HEAD) and takes its
  notes from `CHANGELOG.md` on that commit. The tag must be the commit the
  image was built from, with the markers bumped and the changelog rolled.
- **The release does not build.** Create it after Build & Push is green.
  Otherwise the release page announces a version whose image doesn't exist
  yet.
- **`.env.example` pins `ORBIT_VERSION`.** New deployments copy it and pull that
  tag, so it should name an image that exists.

## Workflows

One workflow, in `.github/workflows/`:

| Workflow | File | Runs on | Does |
|---|---|---|---|
| Build & Push | `docker-image.yml` | **by hand only** | One multi-arch build, pushed to this repo's GitHub Container Registry (`ghcr.io/<owner>/<repo>`) |

It logs in with the built-in `GITHUB_TOKEN`, so no secrets or variables are
needed. There is no CI on pull requests: run `make check lint` locally before
merging.

## 0. Pre-flight

- `main` is green and at the commit you want to ship.
- [`CHANGELOG.md`](../CHANGELOG.md) `[Unreleased]` says what's in it (it's filled as changes
  merge, not at release time).
- Pick the **bare** semver: patch (`1.0.1`) for fixes, minor (`1.1.0`) for
  features, major for breaking changes.

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

Commit, run `make check lint`, push, open a PR, and merge it to `main`.

## 2. Build & Push the image

GitHub → **Actions → Build & Push → Run workflow** → branch **`main`**,
version **`0.2.0`** (a leading `v` is accepted and stripped).

- Builds once with `docker/build-push-action`: `linux/amd64` and `linux/arm64`,
  cross-compiled (pure Go, CGO off), no QEMU.
- Stamps `VERSION` (the bare input), `COMMIT` (short SHA) and `BUILD_TIME`.
- Pushes `ghcr.io/<owner>/<repo>:v0.2.0` and `:latest`, in lower case (the
  repo's **Packages**). A pre-release such as `0.2.0-rc.1` gets only
  `:v0.2.0-rc.1`, never `:latest`.
- Wait for it to be green before step 3. If it fails, fix the cause and run it
  again; nothing in the repo changes.

`make release-image` is a local fallback: the same multi-arch build with
`docker buildx`, pushed to `REGISTRY` (default
`ghcr.io/joaoteixeira10/orbit`) as `:v<version>` and `:latest`.
It needs a `docker login ghcr.io` first (a token with `write:packages`). Run it
from the bumped `main` with `make release-image VERSION=0.2.0`.

## 3. Create the release

From an up-to-date `main`, with the [GitHub CLI](https://cli.github.com/):

```sh
git checkout main && git pull
awk '/^## \[0\.2\.0\]/{f=1;next} /^## /{f=0} f' CHANGELOG.md > release-notes.md
gh release create v0.2.0 --target main --title v0.2.0 \
  --notes-file release-notes.md --generate-notes
rm release-notes.md
```

- Creates tag and release **`v0.2.0`** on `main` HEAD.
- The notes are the `## [0.2.0]` section of `CHANGELOG.md`, followed by
  GitHub's generated list of changes since the previous release.
- It fails if the tag or release already exists.

## 4. Verify

- The repo's **Packages** shows `:v0.2.0` and `:latest`, both architectures.
- The repo's **Releases** shows `v0.2.0` with the notes.
- Deploy it (`ORBIT_VERSION=v0.2.0`, then `docker compose pull && docker compose up -d`)
  and check `/api/v1/version`, the version in the page's sidebar, and the
  version on `/api/docs`.

## Rules that bite

- **Bump before build.** Otherwise the API reference in the image shows the old
  version, and the tag doesn't match the markers.
- **Build before release.** The release never builds.
- **Bare version in the workflow** (`0.2.0`). It adds the `v` to the image tags;
  the git tag is `v0.2.0`.
- **Build and release `main`.** Build & Push uses the branch picked in the
  *Run workflow* dialog; the release tags the `--target` commit.
- **`:latest` moves on every build.** Anyone running with `ORBIT_VERSION` unset
  gets the new image on their next pull. Don't use Build & Push to try things
  out; build locally with `make image` (tags `orbit:dev`, pushes nothing).
- **Nothing checks the input against the markers.** Use the same version you
  bumped to, in the workflow and in the release.
- **Start Build & Push once.** Re-run a failed run *or* start a new one, not
  both: a second run pushes `:v0.2.0` and `:latest` again with a new digest.
- **Roll the changelog heading.** If you forget, the notes still use
  `[Unreleased]`, but `CHANGELOG.md` itself is wrong for the next release.
- **Triggering from the terminal** instead of the page:

  ```sh
  gh workflow run docker-image.yml --ref main -f version=0.2.0
  ```

## Redoing a botched run

| What went wrong | Fix |
|---|---|
| Build & Push failed | Fix the cause and run it again with the same version. Nothing was tagged. |
| Built without the bump | Merge the bump to `main`, run Build & Push again with the same version: it overwrites `:v0.2.0` and `:latest`. |
| `gh release create` says the release exists | The tag or release already exists. Delete them (below) or pick the next version. |
| Released the wrong commit or wrong notes | Delete the release and the tag; fix `main`; run Build & Push, then create the release, with the same version. |

Delete through the page (repo → Releases) or with the GitHub CLI:

```sh
gh release delete v0.2.0 --cleanup-tag --yes
git fetch --prune --prune-tags
```

> Redoing a version is safe only while nobody has pulled the bad image. Once
> someone has, ship the fix as the next patch version instead.
