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
   `validation_url`, `namespace`, `image_tag`, and `claude_namespace`.
5. Glimmung substitutes those phase outputs into `agent-execute`.
6. The `agent-execute` native phase runs
   `scripts/glimmung-native/agent-execute.sh`.
7. `agent-execute` clones this repo, prepares the Claude agent Job in the
   validation namespace, collects logs/evidence, rebuilds the validation
   environment from the pushed agent branch, captures screenshots, posts a
   typed verification result, and lets Glimmung drive retry/report decisions.

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
