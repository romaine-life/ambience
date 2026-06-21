# Ambience Glimmung run-harness (Go)

Ambience's Glimmung run-harness is a typed Go module at `glimmung-harness/`,
built on the Glimmung run-harness SDK `github.com/romaine-life/glimmung`
(pinned to `v0.2.0`, tag pin, **no `replace`**). It replaces the retired
`scripts/glimmung-native/*.sh` shell fork end to end (no parallel path).

The SDK holds Glimmung's run contract as types so a producing step physically
cannot emit an untyped error, skip the verdict, or mislabel a harness crash as
a model failure. See `glimmung/docs/run-harness-sdk.md`.

## Venue: in-cluster, single-faced

Unlike spirelens (a remote Windows gaming-laptop venue over ssh/tailscale),
ambience runs **in-cluster**:

- There is **no host face** and **no remote venue** — no `remotehost`, no ssh,
  no Windows cross-compile. The binary is one Linux face.
- The model does **not** run as an in-pod child of the harness step (so
  `harness/agent.Invoke` does not apply). It runs in an **isolated child k8s
  Job** rendered by the `mcp/ambience_preview` Python venue tooling, which the
  Go handlers shell out to (`apply-agent-job` / `wait-agent-job`). The child
  Job never mounts the real provider OAuth secret; its usage lines are priced
  through Glimmung's inner-Job observation, primed by the
  `===GLIMMUNG-INNER-JOB===` marker the harness emits.
- The per-attempt GitHub token comes from `harness/runcallbacks`
  (`GLIMMUNG_GITHUB_TOKEN_URL` + `X-Glimmung-Attempt-Token`), never a mounted
  OAuth secret.

## Phase shape (identical to the retired shell)

Slugs and phase shape are preserved exactly. Ambience reuses slugs across
phases (`clone`/`prepare`/`finalize`/`emit` recur with different behavior), so
dispatch is two-level: the binary's first argument selects the **phase**
registry; `GLIMMUNG_STEP_SLUG` selects the step within it.

| Glimmung phase | binary phase arg | step slugs (in order) |
| --- | --- | --- |
| prepare (env-prep) | `env-prep` | `clone-repo`, `build-validation-image`, `push-validation-image`, `reap-slot-conflicts`, `deploy-validation-env`, `check-validation-env`, `emit-env-outputs` |
| llm-work / test-plan | `test-plan` | `clone`, `prepare`, `run-test-plan`, `collect`, `finalize`, `emit` |
| llm-work / implement | `implement` | `clone`, `prepare`, `prepare-draft-pr-branch`, `ensure-draft-pr`, `run-implementation`, `collect`, `push-branch`, `wait-pr-checks`, `rebuild-env`, `finalize`, `emit` |
| cleanup (env-destroy) | `env-destroy` | `describe-pre-teardown`, `uninstall-helm-release`, `delete-namespace`, `cleanup-issue-branches`, `emit` |

The **verify** phase is unchanged and is NOT part of this Go harness: it is
rendered and executed entirely as a Python inner Job by
`mcp/ambience_preview/ops.py` (`_VERIFY_BASH`). The native phases above never
write `artifacts/verification.json` — the verification verdict is the verify
inner-Job agent's output, consumed by Glimmung's evidence gate. (See
"Evidence/verification model" below.)

## Step-slug → `run` invocation (for workflow re-registration)

Every step runs the same command; only the phase + slug differ. The harness is
built from the run's checkout at re-registration time (git_ref-controlled,
exactly like spirelens):

```sh
cd glimmung-harness \
  && go build -o /tmp/glimmung-ambience ./cmd/glimmung-ambience \
  && exec /tmp/glimmung-ambience <phase> <slug>
```

For example, env-prep's deploy step:

```sh
cd glimmung-harness && go build -o /tmp/glimmung-ambience ./cmd/glimmung-ambience \
  && exec /tmp/glimmung-ambience env-prep deploy-validation-env
```

The runner pod sets `GLIMMUNG_STEP_SLUG`; the explicit `<slug>` arg is
belt-and-suspenders (it overrides the env, mirroring spirelens's `pod <slug>`).

The agent-runner image (`.github/runner/Dockerfile`) carries the Go toolchain
so this build succeeds in-pod; `go mod download github.com/romaine-life/glimmung@v0.2.0`
resolves via `GOPRIVATE=github.com/romaine-life/*`.

## Honest attribution

Every handler returns a typed `step.LayeredError`:

- `Layer: harness` — the harness glue failed (missing input, bad encode).
- `Layer: host` — the in-cluster venue failed (kubectl/helm/az/python CLI/git
  remote/GitHub API unreachable or misconfigured; the agent Job could not be
  rendered).
- `Layer: model` — the agent run did not deliver (the inner agent Job exited
  nonzero, the model's plan failed the deterministic plan-lint gates, the
  implementation failed contract/ui_hint/PR-CI, or the agent never pushed).

This is an honesty improvement over the shell: a failure to even render the
agent Job is attributed `host`, not laundered through the agent exit code; only
a genuine agent-run failure is recorded and attributed `model`.

## Deterministic pre-verify-spend gates

Ambience's deterministic gates run on the **model's output** and fail the phase
named, BEFORE any verify spend (the in-cluster analogue of spirelens's
pre-agent unit-test gate):

- **test-plan**: `planlint` normalizes and validates the generated plan (closed
  claim vocabulary — no digits/comparators/camelCase in `must_show`; media-kind
  allow-list; case caps; judged-case cap; trigger baseline; terminal-lifecycle
  enum; numeric session_config). A rejected plan fails the phase, emitting the
  named verdict.
- **implement**: the implementation-contract validation (the model's changed
  files must match the effect touchpoints and avoid forbidden paths) and the
  `ui_hint` gate (a passing effect implementation must declare
  `{menu_label, route:/dev/<effect>}`). Both fail the phase named, before
  verify binds the standing case.

## Evidence/verification model and how it flows through the SDK

What the native phases prove (carried as **phase outputs**, not a verification
verdict):

- **env-prep** proves the validation env is built, deployed, and healthy →
  `validation_url`, `validation_slot_index`, `namespace`, `image_tag`,
  `base_revision`, `claude_namespace`, `claude_ca_namespace`.
- **test-plan** proves a well-formed evidence spec → `test_plan` (JSON string),
  `test_cases_count`.
- **implement** proves the agent produced an implementation, pushed the branch,
  passed deterministic PR-CI, rebuilt the env, and declared a ui_hint →
  `implementation` (JSON string), `branch_name`, `pr_number`, `pr_url`,
  `ui_hint`.

The actual **verification verdict** (`artifacts/verification.json`,
screenshots/videos, `evidence_results`) is produced by the **verify inner-Job
agent** (Python venue), not by these native Go phases. Consequently the Go
harness uses the SDK's `step` and `runcallbacks` packages fully, but does **not**
call `harness/verification.WriteFinalizable`/`Gate` or `harness/agent.Invoke` —
ambience has no native step that writes the verdict or runs the agent in-pod.
This is a deliberate venue divergence from spirelens (flagged to the hub as an
SDK gap: an "inner-job agent venue" the SDK could own so model-layer attribution
and usage-line pricing are typed across the pod boundary for in-cluster
consumers).

## Tests

`go test ./...` in `glimmung-harness/` covers:

- `naming` — image tag, branch-name preference order, issue prefix, workflow
  ref, agent image (ported from the shell contract test).
- `planlint` — every plan-validation gate and normalization, guard order, and
  the standing-case lint (ported from the contract test's test-plan cases).
- `contract` — effect contract generation, implementation-contract validation
  (touchpoints/forbidden paths), and the ui_hint gate (ported cases 16/17).
- `venue` — git auth header, inner-Job marker shape, evidence-tar roundtrip,
  PR-checks classification.
- per-phase registry-completeness tests (slugs match the retired scripts).
- `migrationguard` — fails if `scripts/glimmung-native/` or the contract test
  reappear, or any retired `native_*` sentinel lands in a live shell/CI file.
