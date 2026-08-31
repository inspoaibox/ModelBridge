import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";

const root = resolve(import.meta.dirname, "..");

test("production bundle has a browser entrypoint and generated assets", () => {
  const indexPath = resolve(root, "dist", "index.html");
  assert.equal(existsSync(indexPath), true, "run the frontend build before serving dist");
  const html = readFileSync(indexPath, "utf8");
  assert.match(html, /<div id="root"><\/div>/);
  assert.match(html, /<script type="module"[^>]+src="\/assets\/index-[^"]+\.js"/);
  assert.match(html, /<link rel="stylesheet"[^>]+href="\/assets\/index-[^"]+\.css"/);
  assert.doesNotMatch(html, /\/src\/main\.tsx/);
});

test("console documentation route is wired into the application", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  const consoleView = readFileSync(resolve(root, "src", "components", "ConsoleView.tsx"), "utf8");
  assert.match(app, /console_section: normalizeConsoleSection\(section\)/);
  assert.match(consoleView, /UsageDocsPanel/);
  assert.match(consoleView, /section === "docs"/);
});

test("unknown hash routes render the application not-found view", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  const types = readFileSync(resolve(root, "src", "types", "index.ts"), "utf8");
  assert.match(app, /view: "not-found"/);
  assert.match(app, /<NotFoundView/);
  assert.match(types, /"not-found"/);
  assert.match(types, /"reset"/);
  assert.match(app, /<ResetPasswordView/);
});
