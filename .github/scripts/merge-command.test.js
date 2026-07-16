const assert = require("node:assert/strict");
const test = require("node:test");

const {
  evaluateChecks,
  expectedChecks,
  isMergeCommand,
  isWorkflowChange,
  latestReviewsByUser,
  normalizeCheckRuns,
  normalizeStatuses,
  statusBody,
} = require("./merge-command");

test("recognizes only the exact merge command", () => {
  assert.equal(isMergeCommand("/merge"), true);
  assert.equal(isMergeCommand("  /merge\n"), true);
  assert.equal(isMergeCommand("@bot merge"), false);
  assert.equal(isMergeCommand("please /merge"), false);
});

test("identifies workflow-changing pull requests for manual merging", () => {
  assert.equal(isWorkflowChange([".github/workflows/ci.yml"]), true);
  assert.equal(isWorkflowChange([{ filename: ".github/workflows/ci.yml" }]), true);
  assert.equal(isWorkflowChange([{
    filename: "docs/retired-ci.yml",
    previous_filename: ".github/workflows/ci.yml",
  }]), true);
  assert.equal(isWorkflowChange([".github/AUTOMATION.md"]), false);
});

test("always requires branch, security, CodeQL, and Vercel gates", () => {
  assert.deepEqual([...expectedChecks([])].sort(), [
    "Analyze (go)",
    "Analyze (javascript-typescript)",
    "Analyze (rust)",
    "Vercel",
    "check-branch-status",
    "dependency-review",
  ]);
});

test("maps gateway changes to every applicable path-filtered check", () => {
  const checks = expectedChecks([{ filename: "gateway/main.go" }]);
  for (const name of ["go-lint", "go-tests", "sdk-tests", "e2e"]) {
    assert.equal(checks.has(name), true, name);
  }
  assert.equal(checks.has("rust-tests"), false);
  assert.equal(checks.has("web-lint-build"), false);
});

test("uses both sides of a renamed path when selecting checks", () => {
  const checks = expectedChecks([{
    filename: "docs/old-main.go",
    previous_filename: "gateway/old-main.go",
  }]);
  assert.equal(checks.has("go-tests"), true);
  assert.equal(checks.has("go-lint"), true);
});

test("maps verifier, web, SDK, and automation paths", () => {
  const verifier = expectedChecks(["verifier/src/main.rs"]);
  assert.equal(verifier.has("rust-lint"), true);
  assert.equal(verifier.has("rust-tests"), true);
  assert.equal(verifier.has("sdk-tests"), true);
  assert.equal(verifier.has("e2e"), true);

  const web = expectedChecks(["web/src/app/page.tsx"]);
  assert.equal(web.has("web-lint-build"), true);
  assert.equal(web.has("sdk-tests"), true);
  assert.equal(web.has("e2e"), false);

  assert.equal(expectedChecks(["sdk/typescript/src/index.ts"]).has("sdk-tests"), true);
  assert.equal(expectedChecks([".github/AUTOMATION.md"]).has("validate"), true);
});

test("docker compose changes require all affected service checks", () => {
  const checks = expectedChecks(["docker-compose.yml"]);
  for (const name of [
    "go-lint",
    "go-tests",
    "rust-lint",
    "rust-tests",
    "web-lint-build",
    "e2e",
  ]) {
    assert.equal(checks.has(name), true, name);
  }
});

test("normalizes check runs and commit statuses", () => {
  assert.deepEqual(normalizeCheckRuns([{
    id: 1,
    name: "go-tests",
    app: { slug: "github-actions" },
    status: "completed",
    conclusion: "success",
    output: { summary: "done" },
    html_url: "https://example.test/check",
  }]), [{
    id: 1,
    name: "go-tests",
    source: "check:github-actions",
    status: "completed",
    conclusion: "success",
    description: "done",
    url: "https://example.test/check",
  }]);
  assert.equal(normalizeStatuses([{
    id: 2,
    context: "Vercel",
    state: "pending",
    description: "building",
    target_url: "https://example.test/vercel",
    creator: { login: "vercel" },
  }])[0].status, "in_progress");
});

test("waits for missing and pending checks", () => {
  const result = evaluateChecks(new Set(["go-tests", "Vercel"]), [{
    id: 1,
    name: "go-tests",
    source: "check:github-actions",
    status: "in_progress",
    conclusion: null,
  }]);
  assert.deepEqual(result.missing, ["Vercel"]);
  assert.deepEqual(result.pending.map((item) => item.name), ["go-tests"]);
  assert.deepEqual(result.failed, []);
});

test("treats Vercel fork authorization as waitable", () => {
  const result = evaluateChecks(new Set(["Vercel"]), [{
    id: 1,
    name: "Vercel",
    source: "status:vercel",
    status: "completed",
    conclusion: "failure",
    description: "Authorization required to deploy.",
    url: "https://vercel.test/authorize",
  }]);
  assert.deepEqual(result.failed, []);
  assert.equal(result.pending[0].reason, "manual Vercel authorization required");
});

test("fails real failures and unknown skipped checks", () => {
  const result = evaluateChecks(new Set(), [
    { id: 1, name: "go-tests", source: "check:github-actions", status: "completed", conclusion: "failure" },
    { id: 2, name: "unexpected", source: "check:other", status: "completed", conclusion: "skipped" },
  ]);
  assert.deepEqual(result.failed.map((item) => item.name), ["go-tests", "unexpected"]);
});

test("allows only known non-applicable skipped checks", () => {
  const result = evaluateChecks(new Set(), [
    { id: 1, name: "claude", source: "check:github-actions", status: "completed", conclusion: "skipped" },
    { id: 2, name: "Macroscope - Correctness Check", source: "check:macroscope", status: "completed", conclusion: "skipped" },
  ]);
  assert.deepEqual(result.failed, []);
  assert.deepEqual(result.pending, []);
});

test("uses only the newest result for duplicate check names", () => {
  const result = evaluateChecks(new Set(["Vercel"]), [
    { id: 1, name: "Vercel", source: "status:vercel", status: "completed", conclusion: "failure", description: "build failed" },
    { id: 2, name: "Vercel", source: "status:vercel", status: "completed", conclusion: "success" },
  ]);
  assert.deepEqual(result.failed, []);
  assert.deepEqual(result.missing, []);
});

test("requires same-named checks from different providers to both pass", () => {
  const result = evaluateChecks(new Set(["security"]), [
    { id: 1, name: "security", source: "check:first", status: "completed", conclusion: "success" },
    { id: 2, name: "security", source: "status:second", status: "completed", conclusion: "failure" },
  ]);
  assert.deepEqual(result.failed.map((item) => item.source), ["status:second"]);
});

test("tracks each reviewer's latest state", () => {
  const latest = latestReviewsByUser([
    { id: 1, user: { login: "reviewer" }, state: "CHANGES_REQUESTED" },
    { id: 2, user: { login: "reviewer" }, state: "APPROVED" },
    { id: 3, user: { login: "other" }, state: "APPROVED" },
  ]);
  assert.equal(latest.get("reviewer").state, "APPROVED");
  assert.equal(latest.get("other").id, 3);
});

test("status output includes the bound SHA and waiting details", () => {
  const body = statusBody({
    phase: "waiting",
    sha: "abc123",
    required: ["Vercel"],
    missing: ["go-tests"],
    pending: [{ name: "Vercel" }],
  });
  assert.match(body, /merge-command-bot/);
  assert.match(body, /abc123/);
  assert.match(body, /go-tests/);
  assert.match(body, /Vercel/);
});
