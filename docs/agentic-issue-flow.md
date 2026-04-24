# Agentic Issue Flow

This repo now has a first-pass GitHub-to-Codex-to-k8s issue flow aimed at bounded backlog slices.

## What it does

1. A GitHub issue gets a trigger label.
2. GitHub Actions becomes the queue manager.
3. The job runs on an ARC-backed self-hosted runner scale set capped at five runners.
4. Codex implements the issue in the checked-out repo, runs tests, uses the repo-local `ambience_preview` MCP server for the exact build/deploy/screenshot commands, and hands a structured result back to the workflow.
5. The workflow opens a pull request with the screenshots and posts the result back to the issue.
6. A separate pull-request workflow builds and publishes a long-lived preview at `pr-<number>.ambience.dev.romaine.life`.
7. The scratch validation namespace is deleted at the end of the issue run, while the PR preview stays up until the PR closes.

Because each job owns one runner and one namespace, a `maxRunners: 5` scale set also caps us at five concurrent ephemeral environments.

## Files added for this flow

- `.github/workflows/codex-issue-agent.yml`
- `.github/workflows/codex-pr-preview.yml`
- `.github/workflows/build-agent-runner-image.yml`
- `.github/codex/prompts/issue-implementation.md`
- `.github/codex/issue-result.schema.json`
- `.github/runner/Dockerfile`
- `mcp/`
- `scripts/agent/*.sh`
- `scripts/agent/capture-screenshot.mjs`
- `k8s/arc/values-issue-agents.yaml`

## Trigger label

Default trigger label:

- `codex:run`

Optional repo variable:

- `CODEX_TRIGGER_LABELS_JSON`

Example:

```json
["codex:run", "codex:retry"]
```

If the variable is unset, the workflow falls back to `["codex:run"]`. The workflow also tolerates a simpler non-JSON value such as `codex:run` if you set it through the GitHub CLI and the shell mangles the quoting.

## Required GitHub secrets and variables

Secrets:

- `OPENAI_API_KEY` or `CODEX_API_KEY`: optional fallback OpenAI API key used by `codex exec`
- `CODEX_GH_TOKEN`: optional PAT or GitHub App token used when opening PRs; if omitted the workflow falls back to `GITHUB_TOKEN`

Variables:

- `ARM_CLIENT_ID`
- `ARM_TENANT_ID`
- `ARM_SUBSCRIPTION_ID`
- `AZURE_AKS_RESOURCE_GROUP`
- `AZURE_AKS_CLUSTER_NAME`
- `KEY_VAULT_NAME`

The workflow loads the API key from Azure Key Vault secret `openai-api-key` after OIDC login. If that lookup fails or you need an override, it falls back to the GitHub Actions secrets `OPENAI_API_KEY` and then `CODEX_API_KEY`.

## Runner image

The ARC runners for this workflow should use the Dockerfile at `.github/runner/Dockerfile`.

Build and push it with the manual workflow:

- `Build Agent Runner Image`

That image now preinstalls the heavy runtime tooling so issue jobs do less work on every ephemeral runner start. It provides:

- Azure CLI
- Node.js 22
- `@openai/codex`
- Playwright plus Chromium
- the standard GitHub runner base

The runner scale-set values now also declare `ephemeral-storage` requests and limits so runner pods are less likely to be evicted under disk pressure.

## ARC scale set

Use `k8s/arc/values-issue-agents.yaml` with the official `gha-runner-scale-set` chart.

Recommended install shape:

1. Put ARC controller pods in a controller namespace such as `arc-system`.
2. Put the runner scale set in a different namespace such as `arc-runners`.
3. Use a GitHub App secret named `arc-github-app` in that runner namespace.
4. Keep `minRunners: 0` and `maxRunners: 5` for the initial rollout.

Example install:

```bash
helm upgrade --install ambience-issue-agents \
  --namespace arc-runners \
  --create-namespace \
  -f k8s/arc/values-issue-agents.yaml \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set
```

This setup targets the runner scale set by name (`ambience-issue-agents`) instead of extra ARC labels. That keeps it compatible with the controller/chart version currently running in the cluster.

## Agent MCP server

The issue automation now installs a small repo-local Python MCP server from `mcp/` before it launches `codex exec`.

That server gives Codex exact, typed tools for the fixed preview operations:

- `build_preview_image`
- `deploy_validation_preview`
- `capture_validation_screenshot`

The point is to move the exact build/deploy/screenshot command lines behind a stable tool surface instead of making the model reconstruct them from prompt text on every run.

## Ephemeral validation behavior

Each run uses:

- a unique namespace: `ambience-issue-<issue>-<run-id>`
- a fixed Helm release name inside that namespace
- a unique image tag built from the current branch contents

The validation deploy stays internal-only. Codex validates through the MCP server, which deploys the chart without public gateway resources and captures screenshots through a temporary `kubectl port-forward`.

## PR preview behavior

When the issue workflow opens a pull request, `Codex PR Preview` takes over the long-lived review environment:

- build an image for the PR head SHA
- deploy it to `ambience-pr-<number>`
- expose `https://pr-<number>.ambience.dev.romaine.life`
- let external-dns publish the record from the `HTTPRoute`
- update the preview on `pull_request.synchronize`
- delete the namespace on `pull_request.closed`

Preview cleanup is intentionally non-agentic and deterministic.

## Security notes

This workflow intentionally gives Codex broad access inside an isolated automation runner:

- `codex exec --sandbox danger-full-access`
- Azure login
- Kubernetes namespace create/delete
- image builds

Treat that runner pool as privileged automation infrastructure. GitHub's ARC guidance explicitly recommends isolating these workloads because GitHub Actions jobs execute arbitrary code.

Strongly recommended:

- dedicate a node pool or cluster to ARC runners
- keep runner pods in a different namespace from the ARC controller
- scope the GitHub App and Azure identity as tightly as practical
- keep production workloads and secrets out of the runner namespace

## Expected operator loop

1. Label a bounded issue with `codex:run`.
2. Wait for the ARC runner to scale up and pick up the job.
3. Review the PR that the workflow opens and use the PR preview URL that the preview workflow comments back.
4. Close the PR when you are done; the preview namespace will be removed automatically.
5. Re-label or manually dispatch if you want another pass.

If the run is blocked, the workflow comments back on the issue instead of guessing.
