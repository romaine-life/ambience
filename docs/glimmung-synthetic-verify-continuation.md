# Glimmung Synthetic Verify Continuation

This document is a concrete recovery brief for continuing the aborted
Ambience run `ambience#168/runs/17.1`. It exists to prevent agents from
re-litigating known facts, over-hedging about image availability, or spending
time rediscovering the workflow shape.

## Known Facts

- Source PR branch: `codex/lifecycle-observe`
- Ambience PR: `https://github.com/romaine-life/ambience/pull/289`
- PR head observed during this investigation:
  `1fb347493dafb0d9a27a23d1499d25f97a2007fe`
- Glimmung run: `ambience#168/runs/17.1`
- Run id: `b4b67043-978e-42be-85f6-0c568144bc05`
- Workflow used: `branch-input-test`
- Workflow input used: `git_ref=codex/lifecycle-observe`
- Implementation branch created by `llm-work`:
  `glimmung/b4b67043-978e-42be-85f6-0c568144bc05`
- Implementation commit:
  `fbba553bcd7c60670ccaaa8ce871df5455204e2c`
- Validation image tag:
  `git-fbba553bcd7c60670ccaaa8ce871df5455204e2c`
- Validation image:
  `romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c`

The native run events for `llm-work/rebuild-env` showed:

- `skipped_build: false`
- image:
  `romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c`
- source revision:
  `fbba553bcd7c60670ccaaa8ce871df5455204e2c`
- branch:
  `glimmung/b4b67043-978e-42be-85f6-0c568144bc05`
- namespace at the time: `ambience-slot-3`
- public health check returned `ready: true`, `status: 204`
- `rebuild-env` exited successfully

Treat that as proof that the image was built, pushed, deployed once, and
observed healthy. Do not treat image existence as an open mystery unless a
fresh registry check proves the tag has since been deleted.

## What The Workflow Does

The workflow path is in `scripts/glimmung-native/implement.sh`:

1. Resolve the implementation branch revision with
   `native_remote_branch_revision`.
2. Compute `git-<40-character-sha>` with `native_image_tag_for_revision`.
3. Call:

   ```sh
   python3 -m ambience_preview.cli rebuild-validation-image \
     --namespace "$NAMESPACE" \
     --branch "$BRANCH_NAME" \
     --image-tag "$rebuild_tag" \
     --source-revision "$branch_revision" \
     --repo-slug "$REPO_SLUG"
   ```

4. Wait for public preview health with:

   ```sh
   python3 -m ambience_preview.cli wait-public-preview --url "$VALIDATION_URL"
   ```

`ambience_preview.ops.rebuild_validation_image` checks ACR for the exact tag.
If the tag is absent, it runs `az acr build` against the exact GitHub revision.
Then it rolls both workloads in the namespace:

- `deployment/ambience-edge`
- `statefulset/ambience-authority`

This means recreating the workflow action is straightforward. It is not a
reason to pause for extended speculation.

## Correct Continuation Shape

The useful next test is not another full Ambience run. The previous run already
completed `prepare` and `llm-work`. Dashboard/native inspection later showed
that the visual magic-portal cases, including `dev-magic-portal-ending`,
completed. The terminal failure was the focused Go test case:
`tests-magic-portal`.

Use this shape:

1. Claim an Ambience test slot with `checkout_test_slot`.
2. Put the slot on the known implementation image:

   ```sh
   kubectl -n <slot-namespace> set image \
     deployment/ambience-edge \
     ambience=romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c

   kubectl -n <slot-namespace> set image \
     statefulset/ambience-authority \
     ambience=romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c

   kubectl -n <slot-namespace> rollout status deployment/ambience-edge --timeout=10m
   kubectl -n <slot-namespace> rollout status statefulset/ambience-authority --timeout=10m
   ```

3. Wait for the claimed slot URL health endpoint to return the expected ready
   response.
4. Synthetic-dispatch from `llm-verify`, not from `prepare` or `llm-work`.
5. Supply the successful `prepare` and `llm-work` phase outputs from
   `ambience#168/runs/17.1`.
6. Override `prepare.namespace` and `prepare.validation_url` in the supplied
   output to match the newly claimed slot if the new slot is not
   `ambience-slot-3`.
7. Narrow the supplied test plan to `tests-magic-portal` unless new evidence
   proves a different case failed.

Synthetic dispatch is intentionally strict. It will not fetch old outputs,
claim a slot, infer a namespace, or deploy the app. The caller must provide
those facts.

Important discovered gap: the synthetic-dispatch API currently has no run
`inputs` field. Workflows whose checkout refs contain `${{ inputs.git_ref }}`
cannot be synthetically dispatched directly. The practical workaround used here
was to register a temporary workflow named `synthetic-168-verify-fbba553` with
concrete checkout refs:

- workflow checkout ref:
  `glimmung/b4b67043-978e-42be-85f6-0c568144bc05`
- implementation branch:
  `glimmung/b4b67043-978e-42be-85f6-0c568144bc05`

Do not spin on "how do I pass `git_ref` to synthetic dispatch"; that support is
missing. Either add it to Glimmung or use a concrete-ref temporary workflow.

## MCP/API Shape

The MCP tool is `synthetic_dispatch_run`. The required shape is:

```json
{
  "project": "ambience",
  "issue_number": 168,
  "workflow": "synthetic-168-verify-fbba553",
  "start_at_phase": "llm-verify",
  "slot_lease_ref": "<lease returned by checkout_test_slot>",
  "namespace": "<claimed slot namespace>",
  "validation_url": "https://<claimed-slot-host>",
  "reason": "continue aborted ambience#168/runs/17.1 from completed prepare and llm-work outputs",
  "supplied_phase_outputs": [
    {
      "phase": "prepare",
      "phase_outputs": {
        "...": "copy from ambience#168/runs/17.1 prepare output, with namespace and validation_url adjusted"
      }
    },
    {
      "phase": "llm-work",
      "phase_outputs": {
        "...": "copy from ambience#168/runs/17.1 llm-work output"
      }
    }
  ]
}
```

Use `get_native_run_events` or the dashboard resource to recover the exact
phase outputs from `ambience#168/runs/17.1`. Do not substitute guesses for
phase outputs.

## Verified Continuation Result

The continuation was actually driven in this session:

- Checked out `ambience-slot-5`.
- Deployed
  `romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c`
  onto both `deployment/ambience-edge` and `statefulset/ambience-authority`.
- Confirmed `https://ambience-slot-5.ambience.dev.romaine.life/healthz`
  returned `204`.
- Synthetic run:
  `https://glimmung.romaine.life/projects/ambience/issues/168/runs/20/cycles/1`
- `llm-verify` result: `verification.status=pass`.
- Selected case: `tests-magic-portal`.
- Verifier evidence excerpt:

  ```text
  go version go1.26.2 linux/amd64
  ok  	github.com/romaine-life/ambience/sim	0.010s
  ```

The top-level run `20.1` is marked `aborted` only because the temporary
workflow's no-op cleanup job had no container image, so Kubernetes rejected the
cleanup dispatch with `spec.template.spec.containers[0].image: Required value`.
Do not misread that as a verification failure. The `llm-verify` phase itself
completed successfully and emitted a pass verdict.

Why the cleanup job had no image:

- The temporary synthetic workflow registered the no-op cleanup jobs with
  `image: ""` and without `managed: true`.
- Glimmung's native launcher only falls back to `settings.NativeRunnerImage`
  when `NativeJobSpec.Managed` is true. Unmanaged jobs use `job.Image`
  literally.
- Kubernetes therefore received a Job manifest whose main container had an
  empty image field and rejected it before the no-op step could run.

Right way next time:

- For no-op synthetic k8s jobs, use `managed: true` with a normal `run` step.
  That lets Glimmung use its native runner image and step harness.
- Alternatively, specify an explicit image and command/args, but do not mix
  unmanaged jobs with managed `steps` and an empty image.
- Keep in mind that Glimmung workflows now require PR touchpoint/review-gate
  phases. There is no `pr.enabled=false` escape hatch. A truly top-level
  `passed` workflow normally goes through PR touchpoint, human gate, PR merge,
  and final cleanup. For synthetic evidence-only continuation, treat
  `llm-verify verification.status=pass` as the meaningful result unless the
  operator intentionally wants to create and approve a touchpoint PR.

A corrected temporary workflow should therefore use managed no-op cleanup jobs,
for example:

```json
{
  "name": "cleanup_early",
  "kind": "k8s_job",
  "run_on": "always",
  "purpose": "teardown",
  "depends_on": ["llm-verify"],
  "jobs": [
    {
      "id": "env-destroy",
      "name": "No-op synthetic cleanup",
      "managed": true,
      "steps": [
        {
          "slug": "noop",
          "title": "No-op synthetic cleanup",
          "type": "run",
          "run": "echo synthetic continuation cleanup is intentionally no-op"
        }
      ],
      "checkout": {
        "ref": "glimmung/b4b67043-978e-42be-85f6-0c568144bc05",
        "path": "/workspace/ambience"
      },
      "working_directory": "/workspace"
    }
  ]
}
```

That corrected shape was registered as:

```text
synthetic-168-verify-fbba553-managed-cleanup
```

Use this workflow instead of `synthetic-168-verify-fbba553` for any repeat
synthetic continuation. It fixes the cleanup image problem.

This corrected cleanup shape was tested in `ambience#168/runs/21.1`:

- Checked out `ambience-slot-1`.
- Deployed
  `romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c`
  onto both `deployment/ambience-edge` and `statefulset/ambience-authority`.
- Confirmed `https://ambience-slot-1.ambience.dev.romaine.life/healthz`
  returned `204`.
- `llm-verify` passed the selected `tests-magic-portal` case again:

  ```text
  go version go1.26.2 linux/amd64
  ok  	github.com/romaine-life/ambience/sim	0.006s
  ```

- The managed no-op `cleanup_early` job succeeded:

  ```text
  synthetic continuation cleanup is intentionally no-op
  ```

That confirms the no-container-image cleanup problem was fixed by using managed
native jobs.

`runs/21.1` did not terminally pass, but for a different reason: the
`pr_touchpoint` phase failed with HTTP `422`:

```text
required artifact evidence was not recorded
```

This is the next real finish-out gap. Glimmung's touchpoint finalizer validates
that required typed evidence artifacts exist before making the Touchpoint ready.
The narrowed synthetic verifier emitted unit-test evidence with empty
`evidence_refs` and `evidence`, so there was no screenshot/video/evidence
artifact for Touchpoint to promote. Do not conflate this with the old cleanup
image failure.

Important finish-out constraint: Glimmung's current workflow contract requires
PR touchpoint and review-gate phases. There is no `pr.enabled=false` opt-out.
After a successful `llm-verify` and successful cleanup, the workflow will try
to ensure a PR touchpoint, then park at the review gate until approved. The
touchpoint is the human-intervention boundary; it does not merge the PR. Do not
claim an evidence-only synthetic run can terminally pass without either:

- intentionally creating the touchpoint PR and satisfying the human gate, or
- changing Glimmung to support a synthetic/evidence-only terminal mode.

Also do not claim a unit-test-only synthetic verifier can finish the current
Touchpoint path unless it records at least one acceptable artifact ref. For this
case, the viable next shapes are:

- rerun verification with a small durable evidence artifact, for example an
  uploaded text/JSON report under an accepted `evidence/` artifact path, if the
  Touchpoint requirement accepts generic evidence for this issue;
- rerun the richer visual/browser evidence path so the verifier emits the
  required screenshot/video refs; or
- change Glimmung to support an explicit synthetic/evidence-only terminal mode
  that does not require PR Touchpoint artifact publication.

`runs/23.1` confirmed the same touchpoint artifact problem after the verifier
finalization bug was fixed:

- Branch: `codex/168-touchpoint-continuation`
- Head: `9047653 agent: preserve verifier pass after wait timeout`
- Slot: `ambience-slot-3`
- Run:
  `https://glimmung.romaine.life/projects/ambience/issues/168/runs/23/cycles/1`
- `llm-verify` succeeded for all continuation cases:
  `dev-magic-portal-power-surge`, `dev-magic-portal-ember-burst`,
  `dev-magic-portal-rune-shift`, `dev-magic-portal-quiet-gate`, and
  `tests-magic-portal`.
- `touchpoint` failed with:

  ```text
  required artifact evidence was not recorded
  ```

The cause was exact: the selected `tests-magic-portal` required evidence had
kind `go-test`. Glimmung normalizes unknown evidence kinds to `artifact`, but
the child verifier reported the passing Go test only in `evidence_results` with
an empty `evidence` array. The native verifier must materialize non-visual pass
results as durable JSON observation artifacts before touchpoint runs.

The branch now includes that fix in `scripts/glimmung-native/verify.sh`: for a
non-video/non-screenshot selected evidence case with a passing
`evidence_results` entry, it writes
`observations/<evidence-id>-verification.json`. The normal upload path promotes
that observation as an `artifact`, which satisfies touchpoint's required
artifact count.

`runs/24.1` proved that artifact materialization alone was not quite enough:

- Branch head: `cf6a279 agent: emit artifact evidence for nonvisual verification`
- Slot: `ambience-slot-4`
- Run:
  `https://glimmung.romaine.life/projects/ambience/issues/168/runs/24/cycles/1`
- `llm-verify` succeeded and emitted:

  ```json
  {
    "kind": "artifact",
    "ref": "observations/tests-magic-portal-verification.json"
  }
  ```

- The observation upload succeeded, but touchpoint still returned:

  ```text
  required artifact evidence was not recorded
  ```

The cause was Glimmung's touchpoint resolver. It auto-prefixes relative
`screenshots/`, `videos/`, `evidence/`, and `inspections/` refs with
`runs/<project>/<run-id>/`, but not `observations/`. The verifier upload path
stores observation JSON at `runs/<project>/<run-id>/observations/...`, so the
reported artifact ref must already be run-scoped. The branch now normalizes
`observations/...` refs to `runs/<project>/<run-id>/observations/...` in
`write_evidence_artifacts`.

One more discovered runtime gap: the verifier agent container did not have
`go` on `PATH`, so the first synthetic retry, `ambience#168/runs/19.1`, failed
with:

```text
/bin/bash: line 1: go: command not found
```

For run `20.1`, the supplied test-plan command bootstrapped Go before running
the same focused test:

```sh
set -Eeuo pipefail
tmp="$(mktemp -d)"
curl -fsSL https://go.dev/dl/go1.26.2.linux-amd64.tar.gz -o "$tmp/go.tgz"
tar -C "$tmp" -xzf "$tmp/go.tgz"
PATH="$tmp/go/bin:$PATH"
go version
go test ./sim/ -run MagicPortal
```

The durable fix is to make the verifier runtime capable of running the
language toolchains that test-plan evidence is allowed to require. Do not treat
`unit_tests_failed` from a verifier image with no `go` binary as an
implementation failure.

## If The Image Tag Is Somehow Absent

Do not stop at "the image might be missing." Verify the tag first.

If the tag is truly absent, recreate the workflow action:

```sh
python3 -m ambience_preview.cli rebuild-validation-image \
  --namespace "<claimed slot namespace>" \
  --branch "glimmung/b4b67043-978e-42be-85f6-0c568144bc05" \
  --image-tag "git-fbba553bcd7c60670ccaaa8ce871df5455204e2c" \
  --source-revision "fbba553bcd7c60670ccaaa8ce871df5455204e2c" \
  --repo-slug "romaine-life/ambience"
```

That command expects an environment with Azure CLI and registry permissions.
The native Ambience runner image has those tools. A normal session pod may not.
Lack of local `az` in a session pod is not, by itself, a blocker; use the
native runner path or an equivalent GitHub Actions/ACR builder path.

## Do Not Waste Time On These Detours

- Do not confuse PR proof images with the implementation validation image.
  PR #289 CI can prove branch images, but the continuation needs
  `romainecr.azurecr.io/ambience:git-fbba553bcd7c60670ccaaa8ce871df5455204e2c`.
- Do not rerun `prepare` and `llm-work` unless the supplied outputs are
  unavailable or proven invalid.
- Do not treat the slot lease as a hard conceptual blocker. Claiming a slot is
  routine. The important runtime requirement is that the claimed slot serves
  the known implementation image before `llm-verify` starts.
- Do not rely on backend/static hot-swap for this continuation. The
  implementation touched WASM-facing code, so the full validation image is the
  faithful test surface.
- Do not cancel a running verification job merely because it consumes model
  tokens. If a case is already underway and is the thing being tested, assess
  progress before aborting.

## Issue Being Tested

The crux is not whether the effect works. The investigation found that the
effect can work, but the old evidence capture could record the last video
frame after the effect finished. The intended fix is machine-observable effect
lifecycle state:

- `/dev/observe` exposes effect/debug lifecycle information.
- Verification can locate the authoritative in-effect moment.
- `/dev/frame` can capture the matching frame.
- Video remains supporting evidence rather than the sole source of truth.

The continuation run should validate that shape against the magic portal ending
case without spending another full implementation cycle.
