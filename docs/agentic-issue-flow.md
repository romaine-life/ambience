# Agentic Issue Flow

This repo now has a first-pass GitHub-to-Codex-to-k8s issue flow aimed at bounded backlog slices.

## What it does

1. A GitHub issue gets a trigger label.
2. GitHub Actions becomes the queue manager.
3. The job runs on an ARC-backed self-hosted runner scale set capped at five runners.
4. Codex implements the issue in the checked-out repo, runs tests, deploys an ephemeral ambience environment in Kubernetes, validates the change, captures screenshots, and hands a structured result back to the workflow.
5. The workflow wrapper opens a pull request with the screenshots and posts the result back to the issue.
6. The namespace is deleted at the end of the run.

Because each job owns one runner and one namespace, a `maxRunners: 5` scale set also caps us at five concurrent ephemeral environments.

## Files added for this flow

- `.github/workflows/codex-issue-agent.yml`
- `.github/workflows/build-agent-runner-image.yml`
- `.github/codex/prompts/issue-implementation.md`
- `.github/codex/issue-result.schema.json`
- `.github/runner/Dockerfile`
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

- `CODEX_API_KEY`: OpenAI API key used by `codex exec`
- `CODEX_GH_TOKEN`: optional PAT or GitHub App token used when opening PRs; if omitted the workflow falls back to `GITHUB_TOKEN`

Variables:

- `ARM_CLIENT_ID`
- `ARM_TENANT_ID`
- `ARM_SUBSCRIPTION_ID`
- `AZURE_AKS_RESOURCE_GROUP`
- `AZURE_AKS_CLUSTER_NAME`

## Runner image

The ARC runners for this workflow should use the Dockerfile at `.github/runner/Dockerfile`.

Build and push it with the manual workflow:

- `Build Agent Runner Image`

That image intentionally stays small. The job still installs `@openai/codex` and `playwright` at runtime, but the image provides:

- Azure CLI
- `sudo` so `playwright install --with-deps chromium` can succeed on Linux runners
- the standard GitHub runner base

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

## Ephemeral environment behavior

Each run uses:

- a unique namespace: `ambience-issue-<issue>-<run-id>`
- a fixed Helm release name inside that namespace
- a unique image tag built from the current branch contents

The deploy helper disables public gateway resources by default and keeps the validation environment internal-only. The workflow validates through `kubectl port-forward`, which avoids burning public DNS names for short-lived runs.

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
3. Review the PR that the workflow opens.
4. Re-label or manually dispatch if you want another pass.

If the run is blocked, the workflow comments back on the issue instead of guessing.
