const assert = require("node:assert/strict");
const test = require("node:test");

const {
  assertExpectedBranchUpdate,
  assertUpToDate,
  evaluateChecks,
  expectedChecks,
  hasRequiredMergeProtection,
  isMergeCommand,
  isWorkflowChange,
  latestReviewsByUser,
  normalizeCheckRuns,
  normalizeStatuses,
  normalizeVercelDeployment,
  requirement,
  run,
  statusBody,
  upsertStatusComment,
} = require("./merge-command");

function commitClient(parents) {
  return {
    rest: {
      repos: {
        getCommit: async () => ({ data: { parents: parents.map((sha) => ({ sha })) } }),
      },
    },
  };
}

function compareClient(behindBy) {
  return {
    rest: {
      repos: {
        compareCommits: async () => ({ data: { behind_by: behindBy } }),
      },
    },
  };
}

function protectedRuleset(overrides = {}) {
  return {
    id: 1,
    target: "branch",
    enforcement: "active",
    current_user_can_bypass: "never",
    conditions: { ref_name: { include: ["~DEFAULT_BRANCH"], exclude: [] } },
    rules: [{
      type: "required_status_checks",
      parameters: {
        strict_required_status_checks_policy: true,
        required_status_checks: [
          "check-branch-status",
          "dependency-review",
          "Analyze (go)",
          "Analyze (javascript-typescript)",
          "Analyze (rust)",
          "Vercel",
        ].map((context) => context === "Vercel"
          ? { context, integration_id: 8329 }
          : { context }),
      },
    }, {
      type: "pull_request",
      parameters: {
        required_approving_review_count: 1,
        required_review_thread_resolution: true,
      },
    }],
    ...overrides,
  };
}

function effectiveRules() {
  const ruleset = protectedRuleset();
  return ruleset.rules.map((rule) => ({ ...rule, ruleset_id: ruleset.id }));
}

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
  assert.equal(isWorkflowChange([".github/scripts/merge-command.js"]), true);
  assert.equal(isWorkflowChange([".github/scripts/merge-command.test.js"]), false);
});

test("accepts only the expected merge commit after a branch update", async () => {
  await assertExpectedBranchUpdate(
    commitClient(["old-head", "base"]),
    "owner",
    "repo",
    "new-head",
    "old-head",
    "base",
  );
  await assert.rejects(
    assertExpectedBranchUpdate(
      commitClient(["contributor-push"]),
      "owner",
      "repo",
      "new-head",
      "old-head",
      "base",
    ),
    /not the expected merge/,
  );
});

test("rejects an authorized head when main advanced during the update", async () => {
  await assertUpToDate(compareClient(0), "owner", "repo", "base", "head");
  await assert.rejects(
    assertUpToDate(compareClient(1), "owner", "repo", "new-base", "updated-head"),
    /still behind `main`/,
  );
});

test("requires non-bypassable atomic merge protections", () => {
  assert.equal(hasRequiredMergeProtection(effectiveRules(), [protectedRuleset()]), true);
  assert.equal(hasRequiredMergeProtection(
    effectiveRules(),
    [protectedRuleset({ current_user_can_bypass: "always" })],
  ), false);
  const missingVercel = protectedRuleset();
  missingVercel.rules[0].parameters.required_status_checks =
    missingVercel.rules[0].parameters.required_status_checks.filter((check) => check.context !== "Vercel");
  assert.equal(hasRequiredMergeProtection(
    missingVercel.rules.map((rule) => ({ ...rule, ruleset_id: missingVercel.id })),
    [missingVercel],
  ), false);
  const unpinnedVercel = protectedRuleset();
  delete unpinnedVercel.rules[0].parameters.required_status_checks
    .find((check) => check.context === "Vercel").integration_id;
  assert.equal(hasRequiredMergeProtection(
    unpinnedVercel.rules.map((rule) => ({ ...rule, ruleset_id: unpinnedVercel.id })),
    [unpinnedVercel],
  ), false);
  const noApproval = protectedRuleset();
  noApproval.rules[1].parameters.required_approving_review_count = 0;
  assert.equal(hasRequiredMergeProtection(
    noApproval.rules.map((rule) => ({ ...rule, ruleset_id: noApproval.id })),
    [noApproval],
  ), false);
});

test("uses GitHub-resolved effective rules independent of ref patterns", () => {
  const wildcard = protectedRuleset({
    conditions: { ref_name: { include: ["refs/heads/*"], exclude: [] } },
  });
  assert.equal(hasRequiredMergeProtection(
    wildcard.rules.map((rule) => ({ ...rule, ruleset_id: wildcard.id })),
    [wildcard],
  ), true);
});

test("always requires branch, security, CodeQL, and Vercel gates", () => {
  assert.deepEqual(expectedChecks([]).map((item) => item.name).sort(), [
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
    assert.equal(checks.some((item) => item.name === name), true, name);
  }
  assert.equal(checks.some((item) => item.name === "rust-tests"), false);
  assert.equal(checks.some((item) => item.name === "web-lint-build"), false);
});

test("uses both sides of a renamed path when selecting checks", () => {
  const checks = expectedChecks([{
    filename: "docs/old-main.go",
    previous_filename: "gateway/old-main.go",
  }]);
  assert.equal(checks.some((item) => item.name === "go-tests"), true);
  assert.equal(checks.some((item) => item.name === "go-lint"), true);
});

test("maps verifier, web, SDK, and automation paths", () => {
  const verifier = expectedChecks(["verifier/src/main.rs"]);
  assert.equal(verifier.some((item) => item.name === "rust-lint"), true);
  assert.equal(verifier.some((item) => item.name === "rust-tests"), true);
  assert.equal(verifier.some((item) => item.name === "sdk-tests"), true);
  assert.equal(verifier.some((item) => item.name === "e2e"), true);

  const web = expectedChecks(["web/src/app/page.tsx"]);
  assert.equal(web.some((item) => item.name === "web-lint-build"), true);
  assert.equal(web.some((item) => item.name === "sdk-tests"), true);
  assert.equal(web.some((item) => item.name === "e2e"), false);

  assert.equal(expectedChecks(["sdk/typescript/src/index.ts"]).some((item) => item.name === "sdk-tests"), true);
  assert.equal(expectedChecks([".github/AUTOMATION.md"]).some((item) => item.name === "validate"), true);
});

test("shared payment fixture requires verifier and web checks", () => {
  const checks = expectedChecks(["tests/fixtures/payment-authorization-v2.json"]);
  assert.equal(checks.some((item) => item.name === "rust-tests"), true);
  assert.equal(checks.some((item) => item.name === "web-lint-build"), true);
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
    assert.equal(checks.some((item) => item.name === name), true, name);
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
  const statuses = normalizeStatuses([{
    id: 2,
    context: "Vercel",
    state: "pending",
    description: "building",
    target_url: "https://example.test/vercel",
    creator: { login: "vercel" },
  }]);
  assert.equal(statuses[0].status, "in_progress");
  assert.equal(statuses[0].source, "status:unknown");
});

test("trusts only deployments created and reported by the immutable Vercel bot", () => {
  const deployment = { id: 1, creator: { id: 35613825 } };
  const status = {
    id: 2,
    creator: { id: 35613825 },
    state: "success",
    description: "Deployment completed",
    environment_url: "https://preview.example",
  };
  assert.deepEqual(normalizeVercelDeployment(deployment, status), {
    id: 2,
    name: "Vercel",
    source: "deployment:vercel-bot",
    status: "completed",
    conclusion: "success",
    description: "Deployment completed",
    url: "https://preview.example",
  });
  assert.equal(normalizeVercelDeployment(
    { ...deployment, creator: { id: 123 } },
    status,
  ), null);
  assert.equal(normalizeVercelDeployment(
    deployment,
    { ...status, creator: { id: 123 } },
  ), null);
});

test("waits for missing and pending checks", () => {
  const result = evaluateChecks([
    requirement("go-tests"),
    requirement("Vercel", "deployment:vercel-bot"),
  ], [{
    id: 1,
    name: "go-tests",
    source: "check:github-actions",
    status: "in_progress",
    conclusion: null,
  }]);
  assert.deepEqual(result.missing.map((item) => item.name), ["Vercel"]);
  assert.deepEqual(result.pending.map((item) => item.name), ["go-tests"]);
  assert.deepEqual(result.failed, []);
});

test("treats Vercel fork authorization as waitable", () => {
  const result = evaluateChecks([requirement("Vercel", "deployment:vercel-bot")], [{
    id: 1,
    name: "Vercel",
    source: "deployment:vercel-bot",
    status: "completed",
    conclusion: "failure",
    description: "Authorization required to deploy.",
    url: "https://vercel.test/authorize",
  }]);
  assert.deepEqual(result.failed, []);
  assert.equal(result.pending[0].reason, "manual Vercel authorization required");
});

test("fails real failures and unknown skipped checks", () => {
  const result = evaluateChecks([], [
    { id: 1, name: "go-tests", source: "check:github-actions", status: "completed", conclusion: "failure" },
    { id: 2, name: "unexpected", source: "check:other", status: "completed", conclusion: "skipped" },
  ]);
  assert.deepEqual(result.failed.map((item) => item.name), ["go-tests", "unexpected"]);
});

test("allows only known non-applicable skipped checks", () => {
  const result = evaluateChecks([], [
    { id: 1, name: "claude", source: "check:github-actions", status: "completed", conclusion: "skipped" },
    { id: 2, name: "Macroscope - Correctness Check", source: "check:macroscope", status: "completed", conclusion: "skipped" },
  ]);
  assert.deepEqual(result.failed, []);
  assert.deepEqual(result.pending, []);
});

test("uses only the newest result for duplicate check names", () => {
  const result = evaluateChecks([requirement("Vercel", "status:vercel")], [
    { id: 1, name: "Vercel", source: "status:vercel", status: "completed", conclusion: "failure", description: "build failed" },
    { id: 2, name: "Vercel", source: "status:vercel", status: "completed", conclusion: "success" },
  ]);
  assert.deepEqual(result.failed, []);
  assert.deepEqual(result.missing, []);
});

test("requires same-named checks from different providers to both pass", () => {
  const result = evaluateChecks([requirement("security", "check:first")], [
    { id: 1, name: "security", source: "check:first", status: "completed", conclusion: "success" },
    { id: 2, name: "security", source: "status:second", status: "completed", conclusion: "failure" },
  ]);
  assert.deepEqual(result.failed.map((item) => item.source), ["status:second"]);
});

test("an untrusted same-named check cannot satisfy a trusted requirement", () => {
  const result = evaluateChecks([requirement("security", "check:trusted")], [
    { id: 1, name: "security", source: "check:untrusted", status: "completed", conclusion: "success" },
  ]);
  assert.deepEqual(result.missing, [requirement("security", "check:trusted")]);
});

test("accepts neutral check conclusions as passing", () => {
  const result = evaluateChecks([requirement("advisory")], [
    { id: 1, name: "advisory", source: "check:github-actions", status: "completed", conclusion: "neutral" },
  ]);
  assert.deepEqual(result.missing, []);
  assert.deepEqual(result.failed, []);
});

test("tracks each reviewer's latest state", () => {
  const latest = latestReviewsByUser([
    { id: 1, user: { login: "reviewer" }, state: "CHANGES_REQUESTED" },
    { id: 2, user: { login: "reviewer" }, state: "COMMENTED" },
    { id: 3, user: { login: "other" }, state: "APPROVED" },
  ]);
  assert.equal(latest.get("reviewer").state, "CHANGES_REQUESTED");
  assert.equal(latest.get("other").id, 3);
});

test("only approval or dismissal clears an earlier change request", () => {
  const approved = latestReviewsByUser([
    { id: 1, user: { login: "reviewer" }, state: "CHANGES_REQUESTED" },
    { id: 2, user: { login: "reviewer" }, state: "APPROVED" },
  ]);
  assert.equal(approved.get("reviewer").state, "APPROVED");
  const dismissed = latestReviewsByUser([
    { id: 1, user: { login: "reviewer" }, state: "CHANGES_REQUESTED" },
    { id: 2, user: { login: "reviewer" }, state: "DISMISSED" },
  ]);
  assert.equal(dismissed.get("reviewer").state, "DISMISSED");
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

test("writes fork PR status comments with the maintainer token", async (t) => {
  const originalFetch = global.fetch;
  t.after(() => { global.fetch = originalFetch; });
  let request;
  global.fetch = async (url, options) => {
    request = { url, options };
    return {
      ok: true,
      text: async () => JSON.stringify({ id: 42 }),
    };
  };
  const github = {
    paginate: async () => [],
    rest: { issues: { listComments: async () => ({ data: [] }) } },
  };

  const commentId = await upsertStatusComment(
    github,
    "maintainer-token",
    "owner",
    "repo",
    314,
    "status",
  );

  assert.equal(commentId, 42);
  assert.equal(request.url, "https://api.github.com/repos/owner/repo/issues/314/comments");
  assert.equal(request.options.method, "POST");
  assert.equal(request.options.headers.Authorization, "Bearer maintainer-token");
});

test("updates only the maintainer-owned merge status comment", async (t) => {
  const originalFetch = global.fetch;
  t.after(() => { global.fetch = originalFetch; });
  const requests = [];
  global.fetch = async (url, options) => {
    requests.push({ url, options });
    return {
      ok: true,
      text: async () => JSON.stringify({ id: 42 }),
    };
  };
  const github = {
    paginate: async () => [{
      id: 41,
      body: "<!-- merge-command-bot -->\nuntrusted",
      user: { id: 999, type: "Bot" },
    }, {
      id: 42,
      body: "<!-- merge-command-bot -->\nold status",
      user: { id: 143676135, type: "User" },
    }],
    rest: { issues: { listComments: async () => ({ data: [] }) } },
  };

  const commentId = await upsertStatusComment(
    github,
    "maintainer-token",
    "owner",
    "repo",
    314,
    "new status",
  );

  assert.equal(commentId, 42);
  assert.equal(requests.length, 1);
  assert.equal(requests[0].url, "https://api.github.com/repos/owner/repo/issues/comments/42");
  assert.equal(requests[0].options.method, "PATCH");
});

test("preserves the original gate error when status reporting fails", async (t) => {
  const originalFetch = global.fetch;
  t.after(() => { global.fetch = originalFetch; });
  global.fetch = async () => ({
    ok: false,
    status: 403,
    text: async () => JSON.stringify({ message: "comment forbidden" }),
  });
  const failures = [];
  const warnings = [];
  const github = {
    paginate: async () => [],
    rest: {
      issues: { listComments: async () => ({ data: [] }) },
      pulls: {
        get: async () => { throw new Error("original gate failure"); },
      },
    },
  };

  await run({
    github,
    context: {
      repo: { owner: "owner", repo: "repo" },
      payload: {
        issue: { number: 314 },
        comment: { body: "/merge" },
        sender: { id: 143676135 },
      },
    },
    core: {
      setFailed: (message) => failures.push(message),
      warning: (message) => warnings.push(message),
    },
    token: "maintainer-token",
  });

  assert.deepEqual(failures, ["original gate failure"]);
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /comment forbidden/);
});
