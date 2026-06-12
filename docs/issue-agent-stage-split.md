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
example in [`romaine-life/spirelens/.github/workflows/issue-agent.yaml`](https://github.com/romaine-life/spirelens/blob/main/.github/workflows/issue-agent.yaml).

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
verify-case-01..10     (bounded LLM evidence cases, no code edits, no GitHub-write)
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

**Agent runtime slot:** `issue_contract`.

**Input context:** issue body, `.github/agent/prompt-issue-contract.md`,
`AGENTS.md`, `CLAUDE.md`, `docs/effects-cookbook.md`,
`docs/dev-endpoints.md`.

**Tools:** Read, Grep, ToolSearch, optional WebFetch. **No** Edit,
Write, or Bash-state-mutating tools.

**Output:** `/workspace/evidence/issue-agent-contract.json` and `.md`.

### Stage 1 — `run-test-plan`

**Goal:** Read the issue, decide the change shape, list the evidence
that would prove the change works.

**Agent runtime slot:** `test_plan`.

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
      "session_config": {"flash_brightness": 0.9},
      "expected_text": null
    },
    {
      "id": "dev-distant-storm-ending",
      "kind": "screenshot",
      "url_path": "/dev/distant-storm?session=ending1",
      "trigger_event": "ending",
      "terminal_lifecycle": "ended",
      "hold_ticks": 12,
      "session_config": {"flash_p": 0}
    }
  ],
  "validation_path": "/dev/distant-storm",
  "open_questions": []
}
```

Allowed `abort_reason` values: `issue_unclear`,
`no_repo_pattern_for_request`, `out_of_scope_for_agent`,
`requires_human_judgment`.

`required_evidence` is media-only. The test-plan wrapper normalizes common
media aliases to `video` or `screenshot`, then fails any passing plan that has
zero media cases or any deterministic/non-media evidence kind such as
`go-test`, `unit-test`, `lint`, `build`, `ci`, `note`, or `artifact`.
Deterministic checks are PR CI, not LLM verification cases.

**Closed claim vocabulary.** Cases split into **judged** (non-empty
`must_show`: one gestalt sentence an LLM verifier judges against the
artifact) and **mechanical-only** (no `must_show`: the verify wrapper itself
produces and checks the evidence; no verification LLM runs). The test-plan
wrapper enforces the vocabulary on every passing plan:

- `unverifiable_must_show` — a `must_show` containing a digit, a comparator
  phrase ("at least", "at most", "exactly", "within", "more than",
  "less than", "no more than", "between", `>=`, `<=`), or a camelCase
  identifier token (`[a-z]+[A-Z][A-Za-z]*` — internal state reaching;
  kebab-case trigger names are fine) fails the plan with the case id and
  offending fragment named. Judged claims describe a look; decidable
  expectations live in `session_config` / `trigger_event` /
  `terminal_lifecycle`, which the wrapper enforces mechanically.
  Motivated by ambience#167 runs 9.1/10.1, where measured prose claims
  were adjudicated as undecidable by judgment.
- `too_many_judged_cases` — at most `MAX_JUDGED_CASES=3` judged cases per
  plan. Mechanical-only cases are uncapped up to the 10-case total.
- `video_requires_judgment` — `kind: video` requires a non-empty
  `must_show`; video exists to be judged over time. Mechanical cases use
  screenshot/observation evidence.
- `empty_case` — a case without `must_show` must declare at least one of
  `trigger_event`, `terminal_lifecycle`, or `session_config`.
- `trigger_case_without_baseline` — every case declaring `trigger_event`
  must declare a non-empty `session_config`. A trigger judged against the
  default background cadence is undecidable (ambience#167 run 9.1: a
  triggered single-lantern claim drowned in default
  `emit_every`/`release_pulse_p` activity); trigger cases pin a quiescent
  baseline that suppresses the competing cadence knobs (emission rates near
  zero, pulse probabilities 0) so the triggered behavior is isolated.

All vocabulary guards fail closed: a jq evaluation error rejects the plan
with a `jq_error:*` sentinel rather than letting an uncheckable plan pass.

**Pinned session contract.** Dev sessions are created with randomized knob
values (a `/dev` product feature), so a `must_show` written against knob
defaults is undecidable on an unpinned session — ambience#167 run 5 failed
exactly this way: a "5-10 lantern cluster" claim judged against a session
whose randomized `cluster_min`/`cluster_max` was 2/23. The settled contract:

- Every `/dev/<effect>` case runs in a named session; the wrapper injects
  `session=<case id>` into `url_path` when the plan omits it.
- Verification pins each case's session to schema defaults plus the case's
  optional `session_config` (a flat object of knob-key → numeric overrides)
  **before** capture, and the verify wrapper independently re-checks the pin
  (`enforce_session_config_pinned`) — an unpinned or drifted session fails
  the case.
- `must_show` claims are therefore written against the pinned config:
  schema defaults unless `session_config` overrides them. The numbers
  themselves never appear in prose — the closed claim vocabulary rejects
  digits/comparators in `must_show`; the knobs carry the numbers and the
  pin enforcement carries the proof. Non-`/dev` surfaces cannot be pinned
  and take config-agnostic claims only.
- The test-plan wrapper fails a passing plan whose `session_config` is not a
  flat numeric object (`invalid_session_config`); unknown or out-of-range
  knob keys fail at verify time with the knob named.

**Lifecycle observer contract.** Terminal claims assert the effect-generic
lifecycle enum, never effect-internal state fields: cases declare
`terminal_lifecycle` (`intro | running | ending | ended`) + `hold_ticks`,
and `/dev/observe` proves it (`lifecycle=` predicate). The retired
`terminal_state_path` / `terminal_state_equals` fields fail the plan
(`invalid_terminal_lifecycle`) — ambience#167 run 8.1 failed because a plan
guessed `introTicks` semantics that a correct implementation did not share.
`terminal_lifecycle: "ended"` is valid only for effects whose schema
declares `ending_terminal: true`; non-terminal effects resume after the
outro, so their post-outro claim is `running`. Every intro/ending-capable
effect publishes the field (enforced by
`cmd/ambience/effects_lifecycle_test.go`).

### Stage 2 — `run-implementation`

**Goal:** Edit code only. Implement what the issue calls for while respecting
the issue contract's public names.

**Agent runtime slot:** `implementation`.

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

The wrapper creates and pushes the issue-scoped implementation branch
`glimmung/issue-<issue-number>/<run-id>`, opens or updates a draft PR, then
runs the implementation agent. The agent can publish its current work and read
the draft PR CI state through
`scripts/glimmung-native/agent-ci-feedback.sh publish-and-wait`; it should use
that deterministic feedback instead of inventing a build/test proof. After the
agent exits, the wrapper confirms the branch, waits for the PR's GitHub checks
to pass, then rebuilds the validation environment from the checked branch before
verification runs. If the implementation pod fails, the branch is missing, the
draft PR cannot be opened, or PR checks fail or time out, the run aborts before
LLM verification spends tokens.

Final cleanup removes implementation branches under the same issue prefix only
after GitHub reports a merged PR under that prefix. That preserves open review
branches on aborts and unapproved touchpoints while still cleaning abandoned
branches from earlier runs after the issue's touchpoint path has merged.

### Stage 3 — `run-verification`

**Goal:** Validate one ordered evidence case against the rebuilt validation
env. Pin the case's dev session to its declared config
(`scripts/agent/pin-session-config.mjs`: schema defaults + the case's
`session_config`), capture the selected item from the test plan, and — for
judged cases — confirm the `must_show` gestalt matches what the captured
artifact actually shows. The verifier's judgment is gestalt-only: it never
counts, measures, or infers internal state; every mechanical fact is
wrapper-enforced.

**Agentless mechanical cases.** A selected case without a `must_show` never
launches the verification LLM. The wrapper (`run_mechanical_case` in
`verify.sh`) performs the case itself: re-asserts the session pin, fires a
declared `trigger_event` through the documented flow
(`POST /dev/trigger/<session>/<event>?effect=<effect>`, then requires the
event to register in `snapshot.appliedEvents` — accepted is not applied),
proves a declared `terminal_lifecycle` with
`scripts/agent/capture-observation.mjs`, freezes a frame for `screenshot`
cases (via `/dev/observe`, echoing the session's current lifecycle as the
predicate when none is declared), and synthesizes
`issue-agent-verification.{json,md}` in the exact verifier-output schema
(`schema_version: 1`, `status`, `evidence`, `evidence_results` with an
`observed_text` such as "mechanical: pin verified; trigger accepted;
lifecycle ended held N ticks"). finalize / upload-screenshots / emit run
unchanged on that synthesized output, and every enforcement gate below —
pin re-check, evidence/artifact checks, failure-block synthesis — applies
to mechanical cases identically. Every mechanical step fails closed: a
step that cannot be completed writes a `fail` verdict carrying the literal
failure as `observed_text` and a structured `failure` block
(`where: wrapper-mechanical`).

**Agent runtime slot:** `verification`.

**Input context:** issue-contract JSON, test-plan JSON, implementation JSON,
`verification-case.json`, `.github/agent/prompt-verification.md`, the rebuilt
validation URL.

**Tools:** Read, Grep, Bash (curl, node, playwright), Write (only to
`/workspace/evidence/`). **No** Edit on `/workspace/repo/`. **No**
GitHub-write tools.

**Output:** `/workspace/evidence/issue-agent-verification.json` and
`.md`, plus WebMs in `/workspace/evidence/videos/` and optional PNGs
in `/workspace/evidence/screenshots/`. Video cases must run
`scripts/agent/inspect-video.mjs` against the captured WebM before claiming
pass; ad hoc playback servers are outside the contract. JSON shape:

```json
{
  "schema_version": 1,
  "status": "pass",
  "abort_reason": "",
  "failure": null,
  "evidence": [
    {"kind": "video", "ref": "videos/dev-distant-storm.webm", "content_type": "video/webm", "duration_ms": 6000}
  ],
  "evidence_results": [
    {"id": "dev-distant-storm-default", "status": "pass", "video": "videos/dev-distant-storm.webm", "observed_text": "steady horizon line with layered cloud bank; no flash fired"}
  ]
}
```

Allowed `abort_reason` values: `video_missing`, `screenshot_missing`,
`claimed_result_not_observed`, `target_evidence_missing`,
`validation_env_unreachable`, `session_pin_failed`.

**Failure contract.** `observed_text` is required on the selected case's
result (pass or fail) — what the artifact literally shows. On any non-pass
verdict the verifier must also write a structured top-level `failure` block:
`{expected, observed, where, suspected_cause, cause_detail}` with
`suspected_cause` one of `code_bug | test_expectation_mismatch |
environment_config | harness_flake`. The wrapper:

- copies `failure`, the verifier's `evidence_results`, and the selected case
  definition (`id`, `must_show`, `url_path`, `trigger_event`, `session`,
  `session_config`) into the emitted per-case verification JSON
  (`prompt_version: ambience-native-staged-v2`) — glimmung stores and
  renders expected-vs-observed instead of a bare enum;
- folds `expected` / `observed` / `suspected cause` lines into `reasons`,
  so the one-line step failure message answers *why*;
- synthesizes a `failure` block from the case definition and the first
  recorded reason when the failure was detected by wrapper enforcement
  (or the verifier omitted the block — which is itself flagged);
- writes the run summary from `issue-agent-verification.md` on **every**
  outcome, not only pass, and uploads
  `issue-agent-verification.{json,md}` + `verification-case.json` as
  durable `reports/` artifacts prefixed by the case slot.

`verify-case-01` also enforces the issue contract's public surface against the
rebuilt validation environment: declared dev/schema routes must exist, declared
trigger events must be accepted, and forbidden public names must not resolve.
Each active case then recomputes pass/fail for its selected
`verification-case.required_evidence` item, confirming it has a matching
`evidence_results` entry with `status: pass` and the expected artifact path
(`video` for video requirements, `screenshot` for screenshot requirements).
For `/dev/<effect>` cases the wrapper also re-checks the pinned session
contract (`enforce_session_config_pinned`): the live session config must
equal schema defaults + the case's `session_config`, knob for knob. Dev
sessions are reaped after 60s with no listeners and lazily recreated
randomized, so the wrapper holds a background SSE listener on the case's
session from prepare through emit teardown (and pins at prepare); without
it the pinned session would die between capture and enforcement and the
check would false-fail against a freshly randomized session. A
verifier-claimed pass with a missing selected item flips to `fail` with
`abort_reason: target_evidence_missing`. For selected video requirements, the
wrapper also opens the reported local WebM with `inspect-video.mjs`, enforces
the planned duration with a small capture tolerance, and writes a sampled-frame
PNG. A verifier-claimed pass over an unopenable, remote-only, empty, or too
short video flips to `fail` before evidence upload.

## Wrapper changes

The live phase scripts expose these job step boundaries:

```bash
scripts/glimmung-native/issue-contract.sh: run-issue-contract
scripts/glimmung-native/test-plan.sh:      run-test-plan
scripts/glimmung-native/implement.sh:      prepare-draft-pr-branch, ensure-draft-pr, run-implementation, push-branch, wait-pr-checks, rebuild-env
scripts/glimmung-native/verify.sh:         run-verification, finalize, upload-screenshots
```

Each `run_*` function calls a per-stage helper that drives a fresh
agent invocation with the stage prompt and tool restrictions. Glimmung
snapshots the resolved agent runtime on the Run and passes it as
`GLIMMUNG_AGENT_RUNTIME_JSON`; Ambience maps stages to the stable slots above
and renders the selected provider/model/reasoning into the actual agent command.
If that snapshot is missing or selects an unsupported provider, the stage fails
before launching an agent pod rather than using a hidden default.
Stage prompts live at `.github/agent/prompt-issue-contract.md`,
`.github/agent/prompt-test-plan.md`, `.github/agent/prompt-implementation.md`,
and `.github/agent/prompt-verification.md`.

## Registration checklist

1. Land the stage prompts, native scripts, and `ambience_preview` stage
   helpers on the workflow checkout ref.
2. Register the live Glimmung workflow with `prepare` containing both
   `env-prep` and `issue-contract`, then wire `issue_contract` into
   `llm-work` and the bounded verification phase with jobs
   `verify-case-01` through `verify-case-10`.
3. Run a real issue through the shape. Watch for stages reaching into adjacent
   stage surfaces; tighten tool permissions per stage if they do.

## Open questions

- ~~Should the test-plan stage sanity-check `must_show` language against
  the validation env?~~ **Settled** by the pinned session contract above:
  claims are written against schema defaults + declared `session_config`,
  the verifier pins that config before capture, and the wrapper enforces
  the pin. The planner does not need env access to make claims decidable —
  it needs the schema defaults, which live in the repo it already reads.
- How do we surface per-stage cost in the run summary? The current
  `verification_cost` jq filter sums all `result` events; with three
  invocations it should report each separately so cost regressions
  are attributable.
- Should `push-branch` block on the impl-stage local build passing,
  or also on a fresh CI build of the pushed branch? Today it only
  checks that `go build ./...` passes locally before push.
