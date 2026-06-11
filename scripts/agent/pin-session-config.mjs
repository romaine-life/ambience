// pin-session-config.mjs — pin an isolated dev session to a deterministic
// effect config before capturing verification evidence.
//
// Dev sessions are created with RANDOMIZED knob values (a product feature of
// /dev). Verification claims, however, are written against a declared config:
// schema defaults plus the verification case's optional `session_config`
// overrides. This helper makes that contract real:
//
//   pin mode (default)   POST /dev/config with every schema knob set to its
//                        pinned value, then poll /dev/snapshot until the live
//                        config matches. Run this BEFORE loading the page or
//                        firing triggers.
//   --check-only         compare the live session config against the pinned
//                        contract without mutating it. Exit 1 with a mismatch
//                        report when the session was not pinned. Used by the
//                        verify wrapper to enforce the contract post-capture.
//
// Output is a single line of JSON (machine-readable; the verify wrapper and
// the verifier agent both consume it).
//
// Usage:
//   node pin-session-config.mjs \
//     --base-url "$VALIDATION_URL" \
//     --effect paper-lanterns \
//     --session paper-lanterns-release-pulse \
//     [--overrides '{"cluster_min":5,"cluster_max":10}'] \
//     [--check-only] [--timeout-ms 15000] [--manifest /path/manifest.json]

import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

function readArg(flag, fallback = "") {
  const index = process.argv.indexOf(flag);
  if (index === -1 || index === process.argv.length - 1) {
    return fallback;
  }
  return process.argv[index + 1];
}

function hasFlag(flag) {
  return process.argv.includes(flag);
}

function fail(message, extra = {}) {
  console.log(JSON.stringify({ pinned: false, error: message, ...extra }));
  process.exit(1);
}

const baseUrl = readArg("--base-url", process.env.VALIDATION_URL || "");
const effect = readArg("--effect");
const session = readArg("--session");
const overridesRaw = readArg("--overrides", "");
const checkOnly = hasFlag("--check-only");
const timeoutMs = Number(readArg("--timeout-ms", "15000"));
const manifest = readArg("--manifest", "");

if (!baseUrl) fail("--base-url or VALIDATION_URL is required");
if (!effect) fail("--effect is required");
if (!session) fail("--session is required");

let overrides = {};
if (overridesRaw !== "") {
  let parsed;
  try {
    parsed = JSON.parse(overridesRaw);
  } catch (err) {
    fail(`--overrides is not valid JSON: ${err.message}`);
  }
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    fail("--overrides must be a JSON object of knob key -> numeric value");
  }
  overrides = parsed;
}

async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`GET ${url} -> HTTP ${response.status}`);
  }
  return response.json();
}

const schemaUrl = new URL(`/effects/${encodeURIComponent(effect)}/schema`, baseUrl);
let schema;
try {
  schema = await fetchJSON(schemaUrl);
} catch (err) {
  fail(`could not fetch effect schema: ${err.message}`, { schema_url: schemaUrl.toString() });
}
const knobs = Array.isArray(schema.knobs) ? schema.knobs : [];
if (knobs.length === 0) {
  fail(`effect schema for ${effect} declares no knobs; nothing to pin`);
}

// Pinned contract = schema defaults, overridden by session_config entries.
// Overrides must name real knobs, be numeric, and sit inside [min, max] so a
// typo'd plan fails here with a named knob instead of drifting silently.
const knobsByKey = new Map(knobs.map((k) => [k.key, k]));
const expected = {};
for (const knob of knobs) {
  const def = Number(knob.default);
  expected[knob.key] = knob.type === "int" ? Math.round(def) : def;
}
const overrideErrors = [];
for (const [key, raw] of Object.entries(overrides)) {
  const knob = knobsByKey.get(key);
  const value = Number(raw);
  if (!knob) {
    overrideErrors.push(`unknown knob ${key}`);
    continue;
  }
  if (typeof raw !== "number" || !Number.isFinite(value)) {
    overrideErrors.push(`knob ${key} override is not a finite number`);
    continue;
  }
  if (value < Number(knob.min) || value > Number(knob.max)) {
    overrideErrors.push(`knob ${key}=${value} outside schema range [${knob.min}, ${knob.max}]`);
    continue;
  }
  expected[key] = knob.type === "int" ? Math.round(value) : value;
}
if (overrideErrors.length > 0) {
  fail(`session_config overrides rejected: ${overrideErrors.join("; ")}`);
}

function configMatches(liveConfig) {
  const mismatches = [];
  for (const knob of knobs) {
    const want = expected[knob.key];
    const got = Number(liveConfig?.[knob.key]);
    if (!Number.isFinite(got)) {
      mismatches.push({ key: knob.key, expected: want, actual: liveConfig?.[knob.key] ?? null });
      continue;
    }
    const ok = knob.type === "int" ? Math.round(got) === want : Math.abs(got - want) <= 1e-6;
    if (!ok) {
      mismatches.push({ key: knob.key, expected: want, actual: got });
    }
  }
  return mismatches;
}

const snapshotUrl = new URL("/dev/snapshot", baseUrl);
snapshotUrl.searchParams.set("session", session);
snapshotUrl.searchParams.set("effect", effect);

async function fetchSnapshot() {
  return fetchJSON(snapshotUrl);
}

if (checkOnly) {
  let snapshot;
  try {
    snapshot = await fetchSnapshot();
  } catch (err) {
    fail(`could not fetch session snapshot: ${err.message}`, { snapshot_url: snapshotUrl.toString() });
  }
  const mismatches = configMatches(snapshot.config ?? {});
  const result = {
    pinned: mismatches.length === 0,
    mode: "check",
    effect,
    session,
    knob_count: knobs.length,
    override_keys: Object.keys(overrides),
    mismatches,
  };
  console.log(JSON.stringify(result));
  process.exit(mismatches.length === 0 ? 0 : 1);
}

// Pin: POST every knob explicitly. /dev/config creates the session when it
// does not exist yet, so pinning before the first page load both creates the
// session and replaces its randomized starting config in one step.
const configUrl = new URL("/dev/config", baseUrl);
configUrl.searchParams.set("session", session);
configUrl.searchParams.set("effect", effect);
for (const knob of knobs) {
  configUrl.searchParams.set(knob.key, String(expected[knob.key]));
}
const postResponse = await fetch(configUrl, { method: "POST" });
if (!postResponse.ok) {
  fail(`POST /dev/config -> HTTP ${postResponse.status}: ${await postResponse.text()}`);
}

// Confirm the pinned config is live before declaring success — the page and
// trigger captures that follow must observe this exact config.
const deadline = Date.now() + (Number.isFinite(timeoutMs) ? timeoutMs : 15000);
let snapshot = null;
let mismatches = null;
for (;;) {
  try {
    snapshot = await fetchSnapshot();
    mismatches = configMatches(snapshot.config ?? {});
    if (mismatches.length === 0) {
      break;
    }
  } catch {
    // transient; retry until deadline
  }
  if (Date.now() >= deadline) {
    fail("pinned config did not become live before timeout", { mismatches: mismatches ?? [] });
  }
  await new Promise((resolve) => setTimeout(resolve, 400));
}

// Events applied before the pin landed (possible only in the sub-second
// window between session creation and config replacement, or when a session
// pre-existed) are surfaced rather than hidden: the verifier should judge
// triggered behavior from events at or after pinned_at_tick.
const result = {
  pinned: true,
  mode: "pin",
  effect,
  session,
  knob_count: knobs.length,
  override_keys: Object.keys(overrides),
  pinned_at_tick: snapshot.tick ?? null,
  pre_pin_events: Array.isArray(snapshot.appliedEvents) ? snapshot.appliedEvents : [],
  config: expected,
};
if (manifest) {
  const out = path.resolve(manifest);
  await mkdir(path.dirname(out), { recursive: true });
  await writeFile(out, JSON.stringify(result, null, 2));
}
console.log(JSON.stringify(result));
