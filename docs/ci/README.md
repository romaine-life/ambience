# Parked CI workflows — INSTALL REQUIRED (hub action)

The restricted-git session token that produced the Go run-harness migration
**cannot create or modify `.github/workflows/*`** (no `workflows` GitHub App
permission), exactly as spirelens hit. The workflow changes the migration needs
are parked here for an actor with `workflows: write` (the hub / a human) to
install with `git mv` into `.github/workflows/`.

Until these are installed, the existing `.github/workflows/docker-build-check.yaml`
and `.github/workflows/build-native-runner-image.yml` still reference the
**deleted** `scripts/glimmung-native/` paths and the removed
`scripts/test-glimmung-native-contract.sh`, so those two workflows will fail on
this branch. Install the corrected copies in the SAME merge that lands the
harness so `main` never carries the broken combination.

## Install

```sh
git mv docs/ci/go-harness-quality.yml        .github/workflows/go-harness-quality.yml
git mv docs/ci/docker-build-check.yaml        .github/workflows/docker-build-check.yaml
git mv docs/ci/build-native-runner-image.yml  .github/workflows/build-native-runner-image.yml
git rm -r docs/ci   # optional, once installed
```

## What changed and why

- **`go-harness-quality.yml` (NEW)** — build + vet + test for the
  `glimmung-harness/` Go module. The repo-root `go test ./...` in
  docker-build-check covers only the ROOT module and does not descend into the
  nested harness module, so the harness needs its own gate. No Windows
  cross-compile: ambience's harness is single-faced and in-cluster (no host
  face, unlike spirelens).

- **`docker-build-check.yaml` (EDITED)**
  - native-runner `fingerprint-paths`: `.github/runner/Dockerfile scripts/glimmung-native`
    → `.github/runner/Dockerfile`. The harness is no longer baked into the
    image; it is built from the run checkout at re-registration time, so only
    the Dockerfile changes the image.
  - Removed the `Check native runner contract` step (it ran the deleted
    `scripts/test-glimmung-native-contract.sh`). Harness quality is now
    `go-harness-quality.yml`.

- **`build-native-runner-image.yml` (EDITED)**
  - Dropped the `scripts/glimmung-native/**` and
    `scripts/test-glimmung-native-contract.sh` trigger paths.
  - Removed the `Check native runner contract` step.
  - Fingerprint `--paths` `.github/runner/Dockerfile scripts/glimmung-native`
    → `.github/runner/Dockerfile`.
