import { mkdir, readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

const playwrightModule =
  process.env.PLAYWRIGHT_PACKAGE_PATH
    ? pathToFileURL(process.env.PLAYWRIGHT_PACKAGE_PATH).href
    : "playwright";
const { chromium } = await import(playwrightModule);

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

function evidenceRefFor(filePath) {
  const evidenceRoot = path.resolve(process.env.EVIDENCE_DIR || "/workspace/evidence");
  const relative = path.relative(evidenceRoot, filePath);
  if (relative && !relative.startsWith("..") && !path.isAbsolute(relative)) {
    return relative.split(path.sep).join("/");
  }
  return filePath;
}

function contentTypeFor(filePath) {
  switch (path.extname(filePath).toLowerCase()) {
    case ".mp4":
    case ".m4v":
      return "video/mp4";
    case ".mov":
      return "video/quicktime";
    default:
      return "video/webm";
  }
}

const file = readArg("--file");
const screenshot = readArg("--screenshot");
const manifest = readArg("--manifest");
const minDurationMs = Number(readArg("--min-duration-ms", "0"));
const width = Number(readArg("--width", "1280"));
const height = Number(readArg("--height", "720"));

if (!file) {
  throw new Error("--file is required");
}

const videoPath = path.resolve(file);
const info = await stat(videoPath);
if (info.size <= 0) {
  throw new Error(`video is empty: ${videoPath}`);
}

if (screenshot) {
  await mkdir(path.dirname(path.resolve(screenshot)), { recursive: true });
}

const browser = await chromium.launch({ headless: true });

try {
  const context = await browser.newContext({ viewport: { width, height } });
  const page = await context.newPage();
  const contentType = contentTypeFor(videoPath);
  const videoBytes = await readFile(videoPath);
  const videoUrl = `data:${contentType};base64,${videoBytes.toString("base64")}`;
  await page.setContent(
    `<!doctype html>
<html>
  <body style="margin:0;background:#000;">
    <video src=${JSON.stringify(videoUrl)} muted playsinline controls style="display:block;width:100vw;height:100vh;object-fit:contain;"></video>
  </body>
</html>`,
    { waitUntil: "domcontentloaded", timeout: 30000 },
  );
  await page.waitForFunction(
    () => {
      const video = document.querySelector("video");
      return (
        video &&
        Number.isFinite(video.duration) &&
        video.duration > 0 &&
        video.videoWidth > 0 &&
        video.videoHeight > 0
      );
    },
    null,
    { timeout: 30000 },
  );

  const loaded = await page.$eval("video", (video) => ({
    duration: video.duration,
    width: video.videoWidth,
    height: video.videoHeight,
  }));
  const durationMs = Math.round(loaded.duration * 1000);
  if (minDurationMs > 0 && durationMs < minDurationMs) {
    throw new Error(
      `video duration ${durationMs}ms is shorter than required ${minDurationMs}ms: ${videoPath}`,
    );
  }

  const seekTo = Math.max(0, Math.min(loaded.duration - 0.25, loaded.duration * 0.75));
  await page.$eval("video", (video, target) => {
    return new Promise((resolve, reject) => {
      const timer = window.setTimeout(() => {
        cleanup();
        reject(new Error(`seek timed out at ${target}`));
      }, 10000);
      const cleanup = () => {
        window.clearTimeout(timer);
        video.removeEventListener("seeked", onSeeked);
        video.removeEventListener("error", onError);
      };
      const onSeeked = () => {
        cleanup();
        resolve();
      };
      const onError = () => {
        cleanup();
        reject(new Error("video seek failed"));
      };
      video.addEventListener("seeked", onSeeked, { once: true });
      video.addEventListener("error", onError, { once: true });
      video.currentTime = target;
    });
  }, seekTo);

  if (screenshot) {
    await page.screenshot({ path: path.resolve(screenshot), fullPage: false });
  }

  const payload = {
    kind: "video",
    ref: evidenceRefFor(videoPath),
    label: path.basename(videoPath, path.extname(videoPath)),
    content_type: contentType,
    size_bytes: info.size,
    duration_ms: durationMs,
    width: loaded.width,
    height: loaded.height,
    inspected_frame_seconds: Number(seekTo.toFixed(3)),
    screenshot: screenshot ? evidenceRefFor(path.resolve(screenshot)) : undefined,
  };

  if (manifest || hasFlag("--manifest")) {
    const manifestPath = manifest ? path.resolve(manifest) : `${videoPath}.inspect.json`;
    await writeFile(manifestPath, JSON.stringify(payload, null, 2));
  }
  console.log(JSON.stringify(payload));
  await context.close();
} finally {
  await browser.close();
}
