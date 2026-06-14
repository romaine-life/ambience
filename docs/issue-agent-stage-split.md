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
  └─ env-prep           (validation image/env)
       ↓
llm-work
  ├─ run-test-plan      (LLM, no code edits, no kubectl; when-skipped for effect runs)
  └─ run-implementation (LLM, no GitHub, no kubectl, no Playwright)
       ↓
verify-case-01..10     (bounded LLM evidence cases, no code edits, no GitHub-write)
       ↓
evidence-gate
```

The retired issue-contract stage is deleted end to end: it existed to let
the parallel jobs share predicted public names, and for greenfield effects
prediction is structurally impossible (the surface does not exist yet).
Public names settle by the implementation's own declaration — slug from the
issue title per cookbook convention, trigger names from the issue body's
event list, anchored by the `ui_hint` output — and the verify wrapper
mechanically checks the declared surface serves. Implementation still
solves the issue, not the test plan.

## Feature types and case sources

The workflow registration declares a **feature type**
(`AMBIENCE_FEATURE_TYPE` in the job env), and the type selects where
verification cases come from:

- **Standing** — the type has a repo-versioned case at
  `.github/agent/standing-cases/<type>.json`. Used when the feature's
  public surface does not exist at plan time: a new effect has no route,
  name, or schema until the implementation lands, so a generated plan
  could only *guess* implementation specifics — ambience#167 run 11 is
  the motivating failure (the plan invented `session_config` knob names
  the parallel implementation never defined, and the pin contract
  correctly refused to verify against them). `ambience.default` declares
  `feature_type=effect` and uses the standing `effect-acceptance` case:
  find the new effect in the `/dev` picker, pin its session to schema
  defaults, record it, and judge the capture against the issue text.
- **Generated** — no feature type (or no standing case file): the
  test-plan agent authors the plan, unchanged. Correct for projects whose
  world exists before the feature (e.g. a stats display measuring an
  existing game), so plan-time references to real surface are honest.

The workflow shape is **total** across the type's live states: the
`llm-test-plan` job stays declared and is skipped at the PLATFORM level
via the registration `when` condition
(`${{ vars.feature_type }} != 'effect'`) — Glimmung never creates its pod
(zero compute, GitHub-Actions `if:` parity) and the run graph renders the
declared-but-skipped leg with the resolved condition as the reason. The
skip is honest because the toggle has a live second state: generated
plans for feature types whose world exists before the feature. A leg with
no live second state does not get a toggle — it gets deleted (the
issue-contract stage). A skipped leg publishes nothing: its outputs resolve to empty
strings downstream, and the verify wrapper sources the standing case
from its own workflow checkout (`stage_standing_test_plan`) when the
`test_plan` input is empty. The standing case passes the **same lint
gates** as a generated plan — the contract harness wraps the repo file
in the runtime envelope and runs the full claim-vocabulary pipeline on
it in CI.

With the issue-contract stage retired, public names settle by
**declaration instead of prediction**: the implementation derives the
slug/trigger names from the issue + cookbook conventions, declares its
surface through `ui_hint`, and the verify wrapper mechanically checks
the declared dev route and derived schema route actually serve
(`enforce_declared_surface`). Trigger/lifecycle behavior is covered by
the implementer's sim tests in PR CI and the judged acceptance capture,
not by a pre-declared trigger sweep.

**The ui_hint bright line.** Standing cases are bound to the
implementation at verify time through the `ui_hint` phase output
(`{menu_label, route}`, emitted by `implement.sh`, required on a passing
implementation when the feature type declares a standing case —
`missing_ui_hint` fails the implement job before any verify spend). The
hint is a **discovery aid only**: navigation knowledge flows forward
(where to look), evaluation knowledge does not (what success looks like
stays the issue text). `resolve_standing_case` in `verify.sh` binds
`url_path = route + ?session=<case-id>`, the session is pinned to schema
defaults (name-free — no plan-authored knob overrides exist on a
standing case), and the verifier judges the capture against the issue
body, which the wrapper stages into the prompt for standing cases only.

## Stage contracts

### Stage 0 — retired (`run-issue-contract`)

Deleted end to end. The stage predicted public names before implementation;
for greenfield effects the prediction is structurally impossible, and for
existing-world features both sibling stages can read the world directly.
Public-name settlement moved to the implementation's declaration (see
Stage 2's `ui_hint` and the verify wrapper's `enforce_declared_surface`).
Do not reintroduce a pre-implementation contract stage; the contract
harness guards against the retired surface returning.

### Stage 1 — `run-test-plan`

**Goal:** Read the issue, decide the change shape, list the evidence
that would prove the change works.

**Agent runtime slot:** `test_plan`.

**Input context:** issue body,
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
the repo-generated implementation contract for the feature type.

**Agent runtime slot:** `implementation`.

**Input context:** issue context, repo-generated
`implementation-contract.json`, `.github/agent/prompt-implementation.md`,
`AGENTS.md`, `CLAUDE.md`, and the cookbook docs. The implementation stage
does **not** read the test-plan artifact. For `feature_type=effect`, the
contract is generated by `scripts/agent/contracts/generate.sh` and names the
current Go/WASM touchpoints, forbidden legacy paths, required outputs, and
validation commands.

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
env. Pin the case's dev session to its declared config over plain HTTP
(GET `/effects/<effect>/schema` → POST `/dev/config` with schema defaults +
the case's `session_config`), capture the selected item from the test plan
through Glimmung's central capture tools, and — for judged cases — confirm the
`must_show` gestalt matches what the captured artifact actually shows. The
verifier's judgment is gestalt-only: it never counts, measures, or infers
internal state; every mechanical fact is wrapper-enforced.

**Browser evidence comes only from the central tools.** The verifier does not
drive a browser. There is no Playwright in the image, no
`PLAYWRIGHT_WS_ENDPOINT`, and no per-repo capture script — those were deleted
end to end. Glimmung's runner-MCP sidecar exposes `capture_video` and
`capture_screenshot`: the agent calls them by name with JSON args (`url`,
`wait_ms`, `label`, optional `trigger_url`, `width`/`height`, and `full_page`
for screenshots), they connect to the leased slot browser, start recording
only after the page renders (server-side blank-frame gate), upload the
artifact, and return its `ref`/`url`. The verifier puts that ref on the case
result. Pin/trigger/confirm stay plain `curl` HTTP.

**Agentless mechanical cases.** A selected case without a `must_show` never
launches the verification LLM. The wrapper (`run_mechanical_case` in
`verify.sh`) performs the case itself with curl + jq: re-asserts the session
pin (GET schema → POST `/dev/config` → poll `/dev/snapshot`), fires a declared
`trigger_event` through the documented flow
(`POST /dev/trigger/<session>/<event>?effect=<effect>`, then requires the
event to register in `snapshot.appliedEvents` — accepted is not applied),
proves a declared `terminal_lifecycle` via the `/dev/observe` HTTP endpoint
(`POST /dev/observe` with the `lifecycle=` predicate; freezes a frame for
`screenshot` cases via the observation's `frameUrl`, echoing the session's
current lifecycle as the predicate when none is declared), and synthesizes
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

**Input context:** test-plan JSON, implementation JSON,
`verification-case.json`, `.github/agent/prompt-verification.md`, the rebuilt
validation URL, and the issue body for standing cases.

**Tools:** Read, Grep, Bash (curl), the `capture_video` / `capture_screenshot`
MCP tools, Write (only to `/workspace/evidence/`). **No** Edit on
`/workspace/repo/`. **No** local browser/Playwright. **No** GitHub-write tools.

**Output:** `/workspace/evidence/issue-agent-verification.json` and `.md`. The
captured WebM/PNG live as uploaded refs returned by the capture tools (not
local files), carried on the `evidence_results` entry. The central tool's
blank-frame gate already proves render + upload, so there is no local
video-decode step and no ad hoc playback server. JSON shape:

```json
{
  "schema_version": 1,
  "status": "pass",
  "abort_reason": "",
  "failure": null,
  "evidence": [
    {"kind": "video", "ref": "<ref returned by capture_video>", "content_type": "video/webm", "duration_ms": 6000}
  ],
  "evidence_results": [
    {"id": "dev-distant-storm-default", "status": "pass", "video": "<ref returned by capture_video>", "observed_text": "steady horizon line with layered cloud bank; no flash fired"}
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

`verify-case-01` also enforces the implementation-declared public surface
against the rebuilt validation environment: declared dev/schema routes must
exist and declared trigger events must be accepted.
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
`abort_reason: target_evidence_missing`. Video evidence is captured by the
central `capture_video` tool, whose server-side blank-frame gate proves the
clip rendered before its `ref` exists, so the wrapper performs no local
video-decode pass — the retired per-repo decode gate was deleted with the
browser scripts rather than reimplemented.

## Wrapper changes

The live phase scripts expose these job step boundaries:

```bash
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
Stage prompts live at `.github/agent/prompt-test-plan.md`,
`.github/agent/prompt-implementation.md`, and
`.github/agent/prompt-verification.md`. The retired issue-contract stage must
not return; public names settle by the implementation's `ui_hint` declaration
and the feature-type implementation contract.

## Registration checklist

1. Land the stage prompts, native scripts, and `ambience_preview` stage
   helpers on the workflow checkout ref.
2. Register the live Glimmung workflow with `prepare` containing `env-prep`,
   `llm-work` containing the skipped-or-active test-plan job plus
   implementation job, and the bounded verification phase with jobs
   `verify-case-01` through `verify-case-10` or the dynamic verification job.
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
