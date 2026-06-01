# Agentic Issue Flow

Ambience issue automation is moving off GitHub Actions as the runner source.
Glimmung is the queue, run graph, callback, retry, and report owner.
Ambience owns the app-specific work that happens inside native Kubernetes
jobs.

GitHub Actions can still exist for repository CI and image builds. It is not
the source of issue-run execution for the native Ambience flow.

## Native Flow

1. A Glimmung issue for the `ambience` project is dispatched to the registered
   `default` workflow.
2. Glimmung creates a Run, acquires native runner capacity, creates the
   per-attempt callback token, and launches a Kubernetes Job in
   `glimmung-runs`.
3. The `prepare` native phase runs two parallel jobs:
   `scripts/glimmung-native/env-prep.sh` and
   `scripts/glimmung-native/issue-contract.sh`.
4. `env-prep` clones this repo, builds and verifies an Ambience validation
   image, deploys a public validation environment, checks it, and emits
   `validation_url`, `namespace`, `image_tag`, `claude_namespace`, and
   `claude_ca_namespace`.
5. `issue-contract` reads the issue and emits `issue_contract`, the canonical
   target/public-surface contract consumed by later LLM jobs.
6. Glimmung substitutes those phase outputs into `llm-work`.
7. `llm-work` runs `test-plan` and `implement` in parallel. Both consume
   `issue_contract`; neither consumes the other's output.
8. `llm-verify` receives `issue_contract`, `test_plan`, `implementation`, and
   the rebuilt validation URL, captures evidence, posts a typed verification
   result, and lets Glimmung drive retry/report decisions.

Glimmung-native issue bodies are passed into the native Kubernetes job as the
`GLIMMUNG_ISSUE_BODY` environment variable and included verbatim in the agent
prompt, so the agent works from the Glimmung-owned issue text rather than
re-fetching it from GitHub Issues.
Glimmung also passes the run-scoped `GLIMMUNG_AGENT_RUNTIME_JSON` snapshot.
Ambience LLM stages use the `issue_contract`, `test_plan`, `implementation`,
and `verification` slots from that snapshot; model/provider selection is not
owned by shell script defaults.

## Ownership

- Glimmung owns clone-token minting, run/session authentication, native
  callback endpoints, graph state, log archival, retry decisions, and report
  creation.
- Ambience owns image build, Helm deploy, validation checks, stage prompts,
  evidence capture, and verification semantics.
- Managed `type: agent` steps are orchestrated by Glimmung. Ambience scripts
  still emit observational boundaries around app-specific preparation,
  collection, and enforcement work.
- Native job success and failure both terminate through Glimmung's
  `/native/completed` callback. Ambience runner images must not require the
  retired `GLIMMUNG_FAILED_URL` environment variable.

## Workflow Registration

The live `default` workflow is the Postgres-backed workflow row registered in
Glimmung. Dispatch reads that database row, not a workflow file from this repo.
Ambience contributes the per-phase runner scripts under
`scripts/glimmung-native/`; managed `type: agent` steps provide the concrete
provider/model selection through Glimmung's agent runtime policy.

Video evidence can be required per issue with Glimmung labels such as
`evidence:video`, or made the Ambience baseline by registering
`default_requirements.required_evidence` on the live `ambience.default`
workflow. Do that live registration change only after the repo version that can
capture and upload WebM evidence is on the workflow checkout ref.

The terminal review surface is the Glimmung Report primitive. The current
Glimmung registration schema still exposes that knob as `pr.enabled` until the
remaining Report API rename lands.

The native runner registration was first smoke-tested end-to-end through
Glimmung on 2026-05-04 with the earlier two-phase native runner. The current
workflow shape is `prepare` → `llm-work` → `llm-verify` → `evidence-gate`; use
the Glimmung run graph as the source of truth for the active registered shape.

The native runner confirms the pushed agent branch directly through GitHub.
It does not mutate validation namespace metadata for branch discovery.

## Runner Image

The native runner image is built from `.github/runner/Dockerfile` by
`.github/workflows/build-native-runner-image.yml` and pushed to:

```text
romainecr.azurecr.io/ambience-agent-runner:native-runner-<fingerprint>
```

That image includes Azure CLI, `kubectl`, Helm, Python, Node, Playwright,
Codex, and the native Ambience scripts under `/opt/ambience-native/scripts`.

## Retry And Resume

`llm-verify` plus the evidence gate form the verification boundary with a
recycle policy on `verify_fail` and `verify_malformed`. The app-owned step
boundaries are the
resume surface for future MCP/API dispatches; a caller that wants to resume
from a particular point should pass `GLIMMUNG_RESUME_FROM_STEP=<step-slug>`
when creating the next native attempt.

## Evidence

`llm-verify` uploads WebM videos and optional screenshots to Glimmung-owned
private artifact storage under `runs/<project>/<run-id>/videos/` and
`runs/<project>/<run-id>/screenshots/`. It emits typed evidence metadata in the
completion callback so Glimmung can render videos directly on the Touchpoint;
the markdown summary still links through
`https://glimmung.romaine.life/v1/artifacts/...`. Public reviewers do not
access the storage account directly.

The Glimmung-artifact upload path is exercised by `llm-verify`, which captures
browser evidence from the validation environment and pushes it to
`runs/ambience/<run-id>/...` on `romaineglimmungartifacts`, with the proxy link
rendering inline in the resulting report body.
