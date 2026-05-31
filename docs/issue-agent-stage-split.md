# Issue-agent stage split (design)

**Status:** active design
**Driver:** Issue 172's single-LLM observation — the prior phase
ran ~14 min spanning code + build + browser evidence + verify, with the
last ~5 min lost to playwright environment-fighting that should have
been a separate, narrower context.

## Goal

Restructure the native Ambience agent flow so the LLM work is split across
focused invocations rather than one monolithic run. Per the platform principle in
`tank-operator/docs/agent-llm-task-splitting.md` and the canonical
example in [`nelsong6/spirelens/.github/workflows/issue-agent.yaml`](https://github.com/nelsong6/spirelens/blob/main/.github/workflows/issue-agent.yaml).

## Current shape

The live workflow has a `prepare` phase followed by parallel planning and
implementation:

```
prepare
  ├─ env-prep          (validation image/env)
  └─ issue-contract   (LLM, no code edits, no evidence plan)
       ↓
llm-work
  ├─ run-test-plan      (LLM, no code edits, no kubectl)
  └─ run-implementation (LLM, no GitHub, no kubectl, no Playwright)
       ↓
llm-verify              (LLM, no code edits, no GitHub-write)
       ↓
evidence-gate
```

`issue-contract` exists so the test-plan and implementation jobs stay
independent but share canonical target names, public routes, and trigger events.
Implementation still solves the issue, not the test plan.

## Stage contracts

### Stage 0 — `run-issue-contract`

**Goal:** Read the issue and repo conventions, then settle canonical target
names and public surface before the parallel LLM jobs run.

**Input context:** issue body, `.github/agent/prompt-issue-contract.md`,
`AGENTS.md`, `CLAUDE.md`, `docs/effects-cookbook.md`,
`docs/dev-endpoints.md`.

**Tools:** Read, Grep, ToolSearch, optional WebFetch. **No** Edit,
Write, or Bash-state-mutating tools.

**Output:** `/workspace/evidence/issue-agent-contract.json` and `.md`.

### Stage 1 — `run-test-plan`

**Goal:** Read the issue, decide the change shape, list the evidence
that would prove the change works.

**Input context:** issue body, issue-contract JSON,
`.github/agent/prompt-test-plan.md`, `AGENTS.md`, `CLAUDE.md`,
`docs/effects-cookbook.md`, `docs/dev-endpoints.md`.

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
      "kind": "video",
      "url_path": "/dev/distant-storm",
      "must_show": "horizon line + cloud bank, no flash",
      "duration_seconds": 6,
      "expected_text": null
    },
    {
      "id": "dev-distant-storm-flash",
      "kind": "video",
      "url_path": "/dev/distant-storm?session=test1",
      "trigger_event": "lightning-flash",
      "must_show": "cloud interior brightened by flash",
      "duration_seconds": 6,
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

**Goal:** Edit code only. Implement what the issue calls for while respecting
the issue contract's public names.

**Input context:** issue body, issue-contract JSON,
`.github/agent/prompt-implementation.md`, `AGENTS.md`, `CLAUDE.md`, the
cookbook docs. The implementation stage does **not** read the test-plan
artifact.

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
language matches what the captured artifact actually shows.

**Input context:** issue-contract JSON, test-plan JSON, implementation JSON,
`.github/agent/prompt-verification.md`, the rebuilt validation URL.

**Tools:** Read, Grep, Bash (curl, node, playwright), Write (only to
`/workspace/evidence/`). **No** Edit on `/workspace/repo/`. **No**
GitHub-write tools.

**Output:** `/workspace/evidence/issue-agent-verification.json` and
`.md`, plus WebMs in `/workspace/evidence/videos/` and optional PNGs
in `/workspace/evidence/screenshots/`. JSON shape:

```json
{
  "schema_version": 1,
  "status": "pass",
  "abort_reason": "",
  "evidence": [
    {"kind": "video", "ref": "videos/dev-distant-storm.webm", "content_type": "video/webm", "duration_ms": 6000}
  ],
  "evidence_results": [
    {"id": "dev-distant-storm-default", "status": "pass", "video": "videos/dev-distant-storm.webm", "observed_text": null},
    {"id": "dev-distant-storm-flash", "status": "pass", "video": "videos/dev-distant-storm-flash.webm", "observed_text": null},
    {"id": "tests-distant-storm", "status": "pass", "stdout_excerpt": "PASS: TestDistantStormFlashFlow"}
  ]
}
```

Allowed `abort_reason` values: `video_missing`, `screenshot_missing`,
`claimed_result_not_observed`, `target_evidence_missing`,
`validation_env_unreachable`, `unit_tests_failed`.

The wrapper first enforces the issue contract's public surface against the
rebuilt validation environment: declared dev/schema routes must exist, declared
trigger events must be accepted, and forbidden public names must not resolve.
It then recomputes pass/fail by walking the test-plan's
`required_evidence` list and confirming every required item has a
matching `evidence_results` entry with `status: pass` and the expected
artifact path (`video` for video requirements, `screenshot` for
screenshot requirements). A verifier-claimed pass with a missing
required item flips to `fail` with `abort_reason:
target_evidence_missing`.

## Wrapper changes

The live phase scripts expose these job step boundaries:

```bash
scripts/glimmung-native/issue-contract.sh: run-issue-contract
scripts/glimmung-native/test-plan.sh:      run-test-plan
scripts/glimmung-native/implement.sh:      run-implementation, push-branch, rebuild-env
scripts/glimmung-native/verify.sh:         run-verification, finalize, upload-screenshots
```

Each `run_*` function calls a per-stage helper that drives a fresh
claude-code invocation with the stage prompt and tool restrictions.
Stage prompts live at `.github/agent/prompt-issue-contract.md`,
`.github/agent/prompt-test-plan.md`, `.github/agent/prompt-implementation.md`,
and `.github/agent/prompt-verification.md`.

## Registration checklist

1. Land the stage prompts, native scripts, and `ambience_preview` stage
   helpers on the workflow checkout ref.
2. Register the live Glimmung workflow with `prepare` containing both
   `env-prep` and `issue-contract`, then wire `issue_contract` into
   `llm-work` and `llm-verify`.
3. Run a real issue through the shape. Watch for stages reaching into adjacent
   stage surfaces; tighten tool permissions per stage if they do.

## Open questions

- Should the test-plan stage have read access to the validation env
  (already-deployed `main` build) so it can sanity-check `must_show`
  language against what already renders? Probably yes — read-only
  curl + video or screenshot evidence of the *baseline* gives the
  planner a real reference, and the implementation stage still can't
  reach it.
- How do we surface per-stage cost in the run summary? The current
  `verification_cost` jq filter sums all `result` events; with three
  invocations it should report each separately so cost regressions
  are attributable.
- Should `push-branch` block on the impl-stage local build passing,
  or also on a fresh CI build of the pushed branch? Today it only
  checks that `go build ./...` passes locally before push.
