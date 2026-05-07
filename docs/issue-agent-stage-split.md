# Issue-agent stage split (design)

**Status:** draft (2026-05-07)
**Driver:** Issue 172's run-agent observation — the single LLM phase
ran ~14 min spanning code + build + screenshot + verify, with the
last ~5 min lost to playwright environment-fighting that should have
been a separate, narrower context.

## Goal

Restructure `scripts/glimmung-native/agent-execute.sh` so the LLM
work is split across three discrete claude-code invocations rather
than one monolithic run. Per the platform principle in
`tank-operator/docs/agent-llm-task-splitting.md` and the canonical
example in [`nelsong6/spirelens/.github/workflows/issue-agent.yaml`](https://github.com/nelsong6/spirelens/blob/main/.github/workflows/issue-agent.yaml).

## Current shape

Today the `agent-execute` glimmung phase runs:

```
clone-repo → prepare-agent-context → run-agent (single LLM)
            → collect-evidence → summarize-agent
            → verify-result → push-branch → emit-agent-outputs
```

`run-agent` is one claude-code invocation given the full
`.github/agent/prompt.md` + issue body + validation env URL. It
authors code, builds, captures screenshots, and writes notes.md in
one context.

## Proposed shape

Replace the single `run-agent` step with three LLM steps, each with
its own prompt and its own per-step tool/permission profile:

```
clone-repo → prepare-agent-context
          → run-test-plan      (LLM, no code edits, no kubectl)
          → run-implementation (LLM, no GitHub, no kubectl, no Playwright)
          → push-branch
          → rebuild-validation
          → run-verification   (LLM, no code edits, no GitHub-write)
          → collect-evidence
          → summarize-agent
          → verify-result
          → emit-agent-outputs
```

`push-branch` and `rebuild-validation` move earlier — between
implementation and verification — so the verification phase has a
real branch + a rebuilt validation env to look at, mirroring
spirelens.

## Stage contracts

### Stage 1 — `run-test-plan`

**Goal:** Read the issue, decide the change shape, list the evidence
that would prove the change works.

**Input context:** issue body, `.github/agent/prompt-test-plan.md`,
`AGENTS.md`, `CLAUDE.md`, `docs/effects-cookbook.md`,
`docs/dev-endpoints.md`, `screenshot-pages.json`.

**Tools:** Read, Grep, ToolSearch, optional WebFetch. **No** Edit,
Write, or Bash-state-mutating tools.

**Output:** `/workspace/evidence/issue-agent-test-plan.json` and
`.md`. JSON shape:

```json
{
  "schema_version": 1,
  "status": "pass",
  "abort_reason": "",
  "summary": "one paragraph",
  "target_files": ["sim/distant_storm.go", "..."],
  "required_evidence": [
    {
      "id": "dev-distant-storm-default",
      "kind": "screenshot",
      "url_path": "/dev/distant-storm",
      "must_show": "horizon line + cloud bank, no flash",
      "expected_text": null
    },
    {
      "id": "dev-distant-storm-flash",
      "kind": "screenshot",
      "url_path": "/dev/distant-storm?session=test1",
      "trigger_event": "lightning-flash",
      "must_show": "cloud interior brightened by flash",
      "expected_text": null
    },
    {
      "id": "tests-distant-storm",
      "kind": "go-test",
      "command": "go test ./sim/ -run DistantStorm"
    }
  ],
  "validation_path": "/dev/distant-storm",
  "open_questions": []
}
```

Allowed `abort_reason` values: `issue_unclear`,
`no_repo_pattern_for_request`, `out_of_scope_for_agent`,
`requires_human_judgment`.

### Stage 2 — `run-implementation`

**Goal:** Edit code only. Implement what the test plan calls for.

**Input context:** issue body, `.github/agent/prompt-implementation.md`,
the test-plan JSON, `AGENTS.md`, `CLAUDE.md`, the cookbook docs.

**Tools:** Read, Edit, Write, Grep, Bash (build/test only —
`go build`, `go test`, language toolchains; no kubectl, no curl to
the validation env, no playwright). **No** GitHub-write tools.

**Output:** `/workspace/evidence/issue-agent-implementation.json` and
`.md`. JSON shape:

```json
{
  "schema_version": 1,
  "status": "pass",
  "abort_reason": "",
  "summary": "one paragraph",
  "changed_files": ["sim/distant_storm.go", "..."],
  "build_results": {
    "go_build": "pass",
    "go_test": "pass: 4 distant-storm tests"
  }
}
```

Allowed `abort_reason` values: `change_too_large`,
`requires_new_library`, `requires_architecture_change`,
`unsafe_refactor`, `missing_code_context`, `conflicting_requirements`,
`cannot_implement_without_guessing`.

The wrapper builds + pushes the branch from the staged tree at this
point, *before* verification runs. If the build fails on the staged
tree the run aborts with `implementation_build_failed`.

### Stage 3 — `run-verification`

**Goal:** Validate the change against the rebuilt validation env.
Capture the evidence the test plan called for. Confirm `must_show`
language matches what the screenshot actually shows.

**Input context:** test-plan JSON, implementation JSON,
`.github/agent/prompt-verification.md`, the rebuilt validation URL.

**Tools:** Read, Grep, Bash (curl, node, playwright), Write (only to
`/workspace/evidence/`). **No** Edit on `/workspace/repo/`. **No**
GitHub-write tools.

**Output:** `/workspace/evidence/issue-agent-verification.json` and
`.md`, plus PNGs in `/workspace/evidence/screenshots/`. JSON shape:

```json
{
  "schema_version": 1,
  "status": "pass",
  "abort_reason": "",
  "evidence_results": [
    {"id": "dev-distant-storm-default", "status": "pass", "screenshot": "screenshots/dev-distant-storm.png", "observed_text": null},
    {"id": "dev-distant-storm-flash", "status": "pass", "screenshot": "screenshots/dev-distant-storm-flash.png", "observed_text": null},
    {"id": "tests-distant-storm", "status": "pass", "stdout_excerpt": "PASS: TestDistantStormFlashFlow"}
  ]
}
```

Allowed `abort_reason` values: `screenshot_missing`,
`claimed_result_not_observed`, `target_evidence_missing`,
`validation_env_unreachable`, `unit_tests_failed`.

The wrapper recomputes pass/fail by walking the test-plan's
`required_evidence` list and confirming every required item has a
matching `evidence_results` entry with `status: pass`. A verifier-
claimed pass with a missing required item flips to `fail` with
`abort_reason: target_evidence_missing`.

## Wrapper changes

`scripts/glimmung-native/agent-execute.sh` grows three native_step
calls (plus rebuild-validation moved earlier):

```bash
native_step "run-test-plan"      run_test_plan
native_step "run-implementation" run_implementation
native_step "push-branch"        push_branch
native_step "rebuild-validation" rebuild_validation_env
native_step "run-verification"   run_verification
native_step "collect-evidence"   collect_evidence
# ... existing finalize/emit steps ...
```

Each `run_*` function calls a per-stage helper that drives a fresh
claude-code invocation with the stage prompt and tool restrictions.
Stage prompts live at `.github/agent/prompt-test-plan.md`,
`.github/agent/prompt-implementation.md`,
`.github/agent/prompt-verification.md`; the existing
`.github/agent/prompt.md` is removed (or kept temporarily as a
deprecated landmark for one release).

The glimmung workflow registration grows three new step slugs in the
`agent-execute` job. PR retry policy stays attached to the
`agent-execute` phase as a whole.

## Migration plan

1. Land this design doc (this PR) for review.
2. Land a follow-up PR adding the three stage prompts under
   `.github/agent/`, the wrapper changes in
   `scripts/glimmung-native/agent-execute.sh`, and a glimmung
   workflow patch (separate change in the glimmung workflow
   registration). Keep the old `prompt.md` in tree under a
   `.deprecated.md` rename for one release for traceability.
3. Run a few real issues through the new shape. Watch for stages
   reaching into adjacent stage surfaces; tighten tool permissions
   per stage if they do.

## Open questions

- Should the test-plan stage have read access to the validation env
  (already-deployed `main` build) so it can sanity-check `must_show`
  language against what already renders? Probably yes — read-only
  curl + screenshot of the *baseline* gives the planner a real
  reference, and the implementation stage still can't reach it.
- How do we surface per-stage cost in the run summary? The current
  `verification_cost` jq filter sums all `result` events; with three
  invocations it should report each separately so cost regressions
  are attributable.
- Should `push-branch` block on the impl-stage local build passing,
  or also on a fresh CI build of the pushed branch? Today it only
  checks that `go build ./...` passes locally before push.
