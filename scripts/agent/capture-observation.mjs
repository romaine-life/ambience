import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

function readArg(flag, fallback = "") {
  const index = process.argv.indexOf(flag);
  if (index === -1 || index === process.argv.length - 1) {
    return fallback;
  }
  return process.argv[index + 1];
}

function appendIf(params, key, value) {
  if (value !== "") {
    params.set(key, value);
  }
}

function evidenceRefFor(filePath) {
  const evidenceRoot = path.resolve(process.env.EVIDENCE_DIR || "/workspace/evidence");
  const relative = path.relative(evidenceRoot, filePath);
  if (relative && !relative.startsWith("..") && !path.isAbsolute(relative)) {
    return relative.split(path.sep).join("/");
  }
  return filePath;
}

const baseUrl = readArg("--base-url", process.env.VALIDATION_URL || "");
const effect = readArg("--effect", "rain");
const session = readArg("--session", "observe");
const trigger = readArg("--trigger");
const waitEvent = readArg("--wait-event");
// Lifecycle predicate: observe until the session reports this lifecycle
// value (intro | running | ending | ended) and it holds for --hold-ticks.
// This replaced the retired --state-path/--state-equals raw state probing —
// lifecycle claims assert the contract, not effect-internal field names
// (ambience#174).
const lifecycle = readArg("--lifecycle");
const maxTicks = readArg("--max-ticks");
const holdTicks = readArg("--hold-ticks");
const output = readArg("--output");
const screenshot = readArg("--screenshot");

if (readArg("--state-path") || readArg("--state-equals")) {
  throw new Error(
    "--state-path/--state-equals were retired: assert the lifecycle contract with --lifecycle intro|running|ending|ended (ambience#174)",
  );
}

if (!baseUrl) {
  throw new Error("--base-url or VALIDATION_URL is required");
}
if (!output) {
  throw new Error("--output is required");
}

const observeUrl = new URL("/dev/observe", baseUrl);
observeUrl.searchParams.set("effect", effect);
observeUrl.searchParams.set("session", session);
appendIf(observeUrl.searchParams, "trigger", trigger);
appendIf(observeUrl.searchParams, "wait_event", waitEvent);
appendIf(observeUrl.searchParams, "lifecycle", lifecycle);
appendIf(observeUrl.searchParams, "max_ticks", maxTicks);
appendIf(observeUrl.searchParams, "hold_ticks", holdTicks);

const response = await fetch(observeUrl, { method: "POST" });
if (!response.ok) {
  throw new Error(`observe failed ${response.status}: ${await response.text()}`);
}
const payload = await response.json();

const out = path.resolve(output);
await mkdir(path.dirname(out), { recursive: true });
await writeFile(out, JSON.stringify(payload, null, 2));

let screenshotRef;
if (screenshot) {
  if (!payload.frameUrl) {
    throw new Error("observe response did not include frameUrl");
  }
  const frameUrl = new URL(payload.frameUrl, baseUrl);
  const frameResponse = await fetch(frameUrl);
  if (!frameResponse.ok) {
    throw new Error(`frame fetch failed ${frameResponse.status}: ${await frameResponse.text()}`);
  }
  const shot = path.resolve(screenshot);
  await mkdir(path.dirname(shot), { recursive: true });
  await writeFile(shot, Buffer.from(await frameResponse.arrayBuffer()));
  screenshotRef = evidenceRefFor(shot);
}

console.log(
  JSON.stringify({
    kind: "observation",
    ref: evidenceRefFor(out),
    screenshot: screenshotRef,
    observed: Boolean(payload.observed),
    applied: Boolean(payload.applied),
    observed_tick: payload.observedTick,
    held_until_tick: payload.heldUntilTick,
    frame_url: payload.frameUrl,
  }),
);
