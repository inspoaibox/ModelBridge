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
  assert.match(consoleView, /displayedSection === "docs"/);
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

test("home hash route is wired into the application", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  assert.match(app, /raw === "" \|\| raw === "home"/);
  assert.match(app, /currentView === "home"/);
});

test("interface debugging is available inside the customer console", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  const types = readFileSync(resolve(root, "src", "types", "index.ts"), "utf8");
  const plaza = readFileSync(resolve(root, "src", "components", "ModelPlazaView.tsx"), "utf8");
  const labs = readFileSync(resolve(root, "src", "components", "MediaLabsView.tsx"), "utf8");
  assert.match(types, /"interface-debug-text"/);
  assert.match(types, /"interface-debug-model"/);
  assert.match(types, /"interface-debug-image"/);
  assert.match(app, /value === "interface-debug-text"/);
  assert.match(app, /value === "interface-debug-model"/);
  assert.match(app, /value === "interface-debug-image"/);
  assert.match(plaza, /export function ModelPlazaView/);
  assert.doesNotMatch(plaza, /routeTo\("#image-lab"\)/);
  assert.doesNotMatch(plaza, /routeTo\("#video-lab"\)/);
  assert.match(labs, /kind === "image" \? "images\/generations" : "videos"/);
  assert.match(labs, /\/v1\/videos/);
  assert.match(labs, /navigator\.clipboard\.writeText/);
  assert.doesNotMatch(labs, /localStorage/);
});

test("feature settings updates do not send read-only audit metadata", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  assert.match(app, /function featureSettingsUpdatePayload\(settings: FeatureSettings\)/);
  assert.match(app, /body: JSON\.stringify\(featureSettingsUpdatePayload\(next\)\)/);
  assert.doesNotMatch(app, /body: JSON\.stringify\(next\)/);
});

test("TOTP operation policies are submitted and only shown after TOTP is enabled", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  const panel = readFileSync(resolve(root, "src", "components", "admin", "AdminSettingsPanel.tsx"), "utf8");
  assert.match(app, /step_up_channel_model_enabled: settings\.step_up_channel_model_enabled/);
  assert.match(app, /step_up_billing_enabled: settings\.step_up_billing_enabled/);
  assert.match(panel, /featureStepUpPoliciesTitle/);
  assert.match(panel, /draft\.totp_enabled \? \(/);
});

test("API endpoints are configured centrally and shown on the customer token page", () => {
  const app = readFileSync(resolve(root, "src", "App.tsx"), "utf8");
  const panel = readFileSync(resolve(root, "src", "components", "admin", "AdminSettingsPanel.tsx"), "utf8");
  const consoleView = readFileSync(resolve(root, "src", "components", "ConsoleView.tsx"), "utf8");
  const usageDocs = readFileSync(resolve(root, "src", "components", "UsageDocsPanel.tsx"), "utf8");
  assert.match(app, /\/admin\/v1\/settings\/api-endpoints/);
  assert.match(app, /apiEndpoints=\{publicAPIEndpoints\}/);
  assert.match(panel, /systemAPIEndpointsTitle/);
  assert.match(consoleView, /tokensAPIEndpointsTitle/);
  assert.match(consoleView, /navigator\.clipboard\.writeText/);
  assert.match(consoleView, /document\.execCommand\("copy"\)/);
  assert.match(app, /openai_base_url/);
  assert.match(app, /anthropic_base_url/);
  assert.match(usageDocs, /apiEndpoints/);
  assert.match(usageDocs, /ANTHROPIC_BASE_URL/);
  assert.doesNotMatch(usageDocs, /window\.location\.origin \+ "\/v1"/);
  assert.doesNotMatch(readFileSync(resolve(root, "src", "components", "HomeView.tsx"), "utf8"), /window\.location\.origin \+ "\/v1"/);
});
