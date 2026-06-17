#!/usr/bin/env node
// Guard for the chart image tag contract.
//
// The prod values file (chart/ambience/values-prod.yaml) and the base
// values file (chart/ambience/values.yaml) MUST share the same
// `image.tag`. The build workflow bumps both files atomically; this
// guard fails CI if anyone reintroduces drift by hand or removes one
// file's pin. Any chart values file that pins the app image must use the
// canonical app-<fingerprint> tag shape rather than a commit SHA alias.
//
// Why this matters: Glimmung-managed warm test slots install
// chart/ambience with default values only — no `-f values-prod.yaml`
// override — so the base file's `image.tag` is what they pin to.
// When the base file lags prod, slots run an older ambience image
// than prod and can trip bugs already fixed upstream. The lockstep
// guarantee belongs in CI, not in tribal knowledge.

import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const files = ["chart/ambience/values.yaml", "chart/ambience/values-prod.yaml"];
const chartDir = path.join(repoRoot, "chart/ambience");
const allValuesFiles = (await fs.readdir(chartDir))
  .filter((name) => /^values.*\.yaml$/.test(name))
  .sort()
  .map((name) => path.join("chart/ambience", name));
const fingerprintTagPattern = /^app-[0-9a-f]{64}$/;

// Extract the `image.tag` value from a YAML file via a deliberately
// shallow line scan — both files keep the field at the same two-space
// indent. If the format changes, this guard fails loudly rather than
// silently miss the field.
function extractImageTag(text, filePath) {
  const lines = text.split(/\r?\n/);
  let inImage = false;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (/^image:\s*$/.test(line)) {
      inImage = true;
      continue;
    }
    // Image block ends at the next non-indented, non-comment, non-empty line.
    if (inImage && line.length > 0 && !line.startsWith(" ") && !line.startsWith("#")) {
      inImage = false;
    }
    if (!inImage) continue;
    const m = /^  tag:\s*"?([^"\s#]+)"?\s*(#.*)?$/.exec(line);
    if (m) return m[1];
  }
  throw new Error(`${filePath}: could not find image.tag at expected two-space indent under image:`);
}

const results = await Promise.all(
  allValuesFiles.map(async (rel) => {
    const abs = path.join(repoRoot, rel);
    const text = await fs.readFile(abs, "utf8");
    return { file: rel, tag: extractImageTag(text, rel) };
  })
);

const invalidTags = results.filter((r) => !fingerprintTagPattern.test(r.tag));
if (invalidTags.length > 0) {
  const detail = invalidTags.map((r) => `  ${r.file}: ${r.tag}`).join("\n");
  console.error(
    "chart app image tags must use app-<64-hex-fingerprint>:\n" +
      detail +
      "\n\nDo not pin chart-managed app images to commit SHA aliases.",
  );
  process.exit(1);
}

const lockstepResults = results.filter((r) => files.includes(r.file));
const tags = new Set(lockstepResults.map((r) => r.tag));
if (tags.size !== 1) {
  const detail = lockstepResults.map((r) => `  ${r.file}: ${r.tag}`).join("\n");
  console.error(
    "image.tag drift between chart values files:\n" +
      detail +
      "\n\nThe build workflow bumps both files atomically on every push.\n" +
      "If you edited one by hand, edit the other to match (or rerun the\n" +
      "build workflow). See .github/workflows/build-and-deploy.yml.",
  );
  process.exit(1);
}

console.log(`image.tag fingerprint contract valid across ${results.length} files`);
console.log(`image.tag consistent across ${files.length} lockstep files: ${[...tags][0]}`);
