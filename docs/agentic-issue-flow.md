# Agentic Issue Flow

Ambience issue automation is moving off GitHub Actions as the runner source.
Glimmung is the queue, run graph, callback, retry, and report owner.
Ambience owns the app-specific work that happens inside native Kubernetes
jobs.

GitHub Actions can still exist for repository CI and image builds. It is not
the source of issue-run execution for the native Ambience flow.

## Native Flow

1. A Glimmung issue for the `ambience` project is dispatched to the registered
   `agent-run` workflow.
2. Glimmung creates a Run, acquires native runner capacity, creates the
   per-attempt callback token, and launches a Kubernetes Job in
   `glimmung-runs`.
3. The `env-prep` native phase runs
   `scripts/glimmung-native/env-prep.sh`.
4. `env-prep` clones this repo, builds and verifies an Ambience validation
   image, deploys a public validation environment, checks it, and emits
   `validation_url`, `namespace`, `image_tag`, `claude_namespace`, and
   `claude_ca_namespace`.
5. Glimmung substitutes those phase outputs into `agent-execute`.
6. The `agent-execute` native phase runs
   `scripts/glimmung-native/agent-execute.sh`.
7. `agent-execute` clones this repo, prepares the Claude agent Job in the
   validation namespace, collects logs/evidence, rebuilds the validation
   environment from the pushed agent branch, captures screenshots, posts a
   typed verification result, and lets Glimmung drive retry/report decisions.

Glimmung-native issue bodies are passed into the native Kubernetes job as the
`GLIMMUNG_ISSUE_BODY` environment variable and included verbatim in the agent
prompt, so the agent works from the Glimmung-owned issue text rather than
re-fetching it from GitHub Issues.

## Ownership

- Glimmung owns clone-token minting, run/session authentication, native
  callback endpoints, graph state, log archival, retry decisions, and report
  creation.
- Ambience owns image build, Helm deploy, validation checks, agent prompt/job
  creation, screenshot selection, and verification semantics.
- Steps are observational boundaries emitted by Ambience scripts. Glimmung
  records them but does not orchestrate inside a step.

## Workflow Registration

After a native runner image is built, register Ambience with:

```bash
AMBIENCE_NATIVE_RUNNER_IMAGE=romainecr.azurecr.io/ambience-agent-runner:native-<sha> \
  scripts/glimmung-native/register-workflow.sh
```

The registration currently creates two native phases:

- `env-prep`
- `agent-execute`

The terminal review surface is the Glimmung Report primitive. The current
Glimmung registration schema still exposes that knob as `pr.enabled` until the
remaining Report API rename lands.

The native runner registration has been smoke-tested end-to-end through
Glimmung against this repo: a Glimmung-dispatched issue successfully
ran `env-prep` and `agent-execute` against the registered Ambience
workflow and produced a documentation-only commit on an agent branch.
Most recently re-exercised on 2026-05-04 as a production dogfood pass:
both phases ran cleanly under the registered native runner image and the
attempt produced a reviewable agent branch via the Report primitive.

The native runner confirms the pushed agent branch directly through GitHub.
It does not mutate validation namespace metadata for branch discovery.

## Runner Image

The native runner image is built from `.github/runner/Dockerfile` by
`.github/workflows/build-native-runner-image.yml` and pushed to:

```text
romainecr.azurecr.io/ambience-agent-runner:native-<sha>
```

That image includes Azure CLI, `kubectl`, Helm, Python, Node, Playwright,
Codex, and the native Ambience scripts under `/opt/ambience-native/scripts`.

## Retry And Resume

`agent-execute` is a verification phase with a recycle policy on
`verify_fail` and `verify_malformed`. The app-owned step boundaries are the
resume surface for future MCP/API dispatches; a caller that wants to resume
from a particular point should pass `GLIMMUNG_RESUME_FROM_STEP=<step-slug>`
when creating the next native attempt.

## Evidence

`agent-execute` uploads screenshots to Glimmung-owned private artifact storage
under `runs/<project>/<run-id>/screenshots/`. PR and Report markdown link
through `https://glimmung.romaine.life/v1/artifacts/...`; public reviewers do
not access the storage account directly.

The Glimmung-artifact upload path has been smoke-tested end-to-end through a
Glimmung-dispatched native run against this repo: `agent-execute` captured a
screenshot of the validation environment and pushed it to
`runs/ambience/<run-id>/screenshots/` on `romaineglimmungartifacts`, with the
proxy link rendering inline in the resulting PR body.
