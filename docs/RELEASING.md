---
summary: "Release checklist for the Robben Media gogcli fork"
---

# Releasing `gogcli`

Robben Media maintains this fork independently from the original project. Releases come only from `Robben-Media/gogcli`; upstream repositories, taps, and install methods are not part of this process.

Shortcut scripts:

```sh
scripts/release.sh X.Y.Z
scripts/verify-release.sh X.Y.Z
```

## 0) Prerequisites

- Repo: `Robben-Media/gogcli`
- Clean working tree on `main`
- Go toolchain from `go.mod`
- GitHub CLI authenticated for the Robben Media repository
- `make ci` succeeds locally

## 1) Verify the build

```sh
make ci
make build
./bin/gog --version
```

Confirm GitHub Actions `ci` is green for the commit being tagged:

```sh
gh run list -L 5 --repo Robben-Media/gogcli --branch main
```

## 2) Update the changelog

Update `CHANGELOG.md` with a dated section for the release.

Example:

```text
## 0.10.0 - 2026-08-04
```

## 3) Commit, tag, and push

```sh
git switch main
git pull --ff-only origin main

git commit -am "release: vX.Y.Z"
git tag -a vX.Y.Z -m "Release X.Y.Z"
git push origin main --tags
```

## 4) Verify the Robben Media GitHub release

The tag triggers `.github/workflows/release.yml`. Confirm the workflow succeeds, release notes are non-empty, and the expected assets and checksums are present:

```sh
gh run list -L 5 --repo Robben-Media/gogcli --workflow release.yml
gh release view vX.Y.Z --repo Robben-Media/gogcli
```

The companion skill pack ships in two forms from the same source under `skills/pack/`:

- the `SKILL.md` files are present in the tagged Git tree;
- the same files are embedded in the `gog` binary with `go:embed`.

No separate skill-pack release artifact is required.

If the workflow needs a rerun:
```sh
gh workflow run release.yml --repo Robben-Media/gogcli -f tag=vX.Y.Z
```

## 5) Verify from Robben source

Before further commits land on `main`, run the repository verification script from the clean checkout whose `HEAD` is tagged `vX.Y.Z`:

```sh
scripts/verify-release.sh X.Y.Z
```

The script validates the changelog, CI, GitHub release, local test suite, and a fresh local build from the Robben Media checkout.

For an operator installation, build from the verified Robben checkout and install the resulting binary:

```sh
make build
install -m 755 bin/gog ~/.local/bin/gog
gog --version
```

Do not use an upstream Homebrew tap or `go install ...@main`. The retained Go module namespace is an internal compatibility detail, not an installation source.
