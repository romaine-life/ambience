import path from "node:path";
import { pathToFileURL } from "node:url";

const playwrightModule =
  process.env.PLAYWRIGHT_PACKAGE_PATH
    ? pathToFileURL(process.env.PLAYWRIGHT_PACKAGE_PATH).href
    : "playwright";
// Accept both shapes: a bare-specifier import exposes `chromium` as a named
// export, while importing the package's index.js by absolute path (e.g. via
// PLAYWRIGHT_PACKAGE_PATH inside a slot's playwright pod) exposes it on default.
const playwrightImport = await import(playwrightModule);
const chromium = playwrightImport.chromium ?? playwrightImport.default?.chromium;
if (!chromium) {
  throw new Error(`could not load chromium from ${playwrightModule}`);
}

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

const url = readArg("--url");
const output = readArg("--output");
const waitMs = Number(readArg("--wait-ms", "5000"));
const width = Number(readArg("--width", "1600"));
const height = Number(readArg("--height", "900"));
const dpr = Number(readArg("--dpr", "1"));
const fullPage = hasFlag("--full-page");

// Ordered pre-capture interactions, applied in argv order after the initial
// --wait-ms settle. Lets a caller drive keyboard-only / interactive UI (e.g.
// the ambience monitor's Esc-summoned chrome) before the shot, so an
// interactive state is captured without a bespoke Playwright script:
//   --press <key>       page.keyboard.press (e.g. Escape)
//   --click <selector>  page.click
//   --eval  <js>        page.evaluate(js)
//   --wait  <ms>        pause between actions
function collectActions() {
  const actions = [];
  for (let i = 0; i < process.argv.length - 1; i++) {
    const flag = process.argv[i];
    const value = process.argv[i + 1];
    if (flag === "--press") actions.push({ kind: "press", value });
    else if (flag === "--click") actions.push({ kind: "click", value });
    else if (flag === "--eval") actions.push({ kind: "eval", value });
    else if (flag === "--wait") actions.push({ kind: "wait", value });
  }
  return actions;
}

async function runActions(page, actions) {
  for (const a of actions) {
    if (a.kind === "press") await page.keyboard.press(a.value);
    else if (a.kind === "click") await page.click(a.value);
    else if (a.kind === "eval") await page.evaluate(a.value);
    else if (a.kind === "wait") await page.waitForTimeout(Number(a.value));
  }
}

if (!url) {
  throw new Error("--url is required");
}

if (!output) {
  throw new Error("--output is required");
}

const browser = await chromium.launch({ headless: true });

try {
  const context = await browser.newContext({
    viewport: { width, height },
    deviceScaleFactor: dpr,
  });
  const page = await context.newPage();
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(waitMs);
  await runActions(page, collectActions());
  await page.screenshot({
    path: path.resolve(output),
    fullPage,
  });
  await context.close();
} finally {
  await browser.close();
}
