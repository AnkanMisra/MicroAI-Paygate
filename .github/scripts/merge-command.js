const BOT_MARKER = "<!-- merge-command-bot -->";
const ALLOWED_SKIPPED_CHECKS = new Set([
  "claude",
  "Macroscope - Correctness Check",
]);
const ALWAYS_REQUIRED_CHECKS = new Set([
  "check-branch-status",
  "dependency-review",
  "Analyze (go)",
  "Analyze (javascript-typescript)",
  "Analyze (rust)",
  "Vercel",
]);
const AUTHORIZED_USER_ID = 143676135;
const GITHUB_ACTIONS_SOURCE = "check:github-actions";
const VERCEL_SOURCE = "status:vercel.com";
const SUCCESSFUL_CONCLUSIONS = new Set(["success", "neutral"]);
const FAILING_CONCLUSIONS = new Set([
  "action_required",
  "cancelled",
  "failure",
  "startup_failure",
  "stale",
  "timed_out",
]);

function isMergeCommand(body) {
  return typeof body === "string" && body.trim() === "/merge";
}

function isWorkflowChange(files) {
  return files.some((file) => {
    const paths = typeof file === "string" ? [file] : [file.filename, file.previous_filename];
    return paths.some((path) => path?.startsWith(".github/workflows/"));
  });
}

function requirement(name, source = GITHUB_ACTIONS_SOURCE) {
  return { name, source };
}

function requirementKey(item) {
  return `${item.source}:${item.name}`;
}

function matchesPath(path, prefixes, exact = []) {
  return prefixes.some((prefix) => path.startsWith(prefix)) || exact.includes(path);
}

function expectedChecks(files) {
  const checks = new Map([
    ...[...ALWAYS_REQUIRED_CHECKS]
      .filter((name) => name !== "Vercel")
      .map((name) => [requirementKey(requirement(name)), requirement(name)]),
    [requirementKey(requirement("Vercel", VERCEL_SOURCE)), requirement("Vercel", VERCEL_SOURCE)],
  ]);
  const paths = files.flatMap((file) => typeof file === "string"
    ? [file]
    : [file.filename, file.previous_filename].filter(Boolean));

  if (paths.some((path) => matchesPath(path, [".github/"], ["codecov.yml"]))) {
    checks.set(requirementKey(requirement("validate")), requirement("validate"));
  }

  const gateway = paths.some((path) => matchesPath(
    path,
    ["gateway/"],
    ["deploy/fly/gateway.fly.toml", "docker-compose.yml", ".github/workflows/go-lint.yml", ".github/workflows/go-tests.yml"],
  ));
  if (gateway) {
    checks.set(requirementKey(requirement("go-lint")), requirement("go-lint"));
    checks.set(requirementKey(requirement("go-tests")), requirement("go-tests"));
  }

  const verifier = paths.some((path) => matchesPath(
    path,
    ["verifier/"],
    [
      "tests/fixtures/payment-authorization-v2.json",
      "deploy/fly/verifier.fly.toml",
      "docker-compose.yml",
      ".github/workflows/rust-lint.yml",
      ".github/workflows/rust-tests.yml",
    ],
  ));
  if (verifier) {
    checks.set(requirementKey(requirement("rust-lint")), requirement("rust-lint"));
    checks.set(requirementKey(requirement("rust-tests")), requirement("rust-tests"));
  }

  const web = paths.some((path) => matchesPath(
    path,
    ["web/"],
    [
      "tests/fixtures/payment-authorization-v2.json",
      "docker-compose.yml",
      ".github/workflows/web-lint-build.yml",
    ],
  ));
  if (web) checks.set(requirementKey(requirement("web-lint-build")), requirement("web-lint-build"));

  const sdk = paths.some((path) => matchesPath(
    path,
    ["sdk/", "gateway/", "verifier/", "web/", "tests/"],
    ["package.json", "bun.lock", ".github/workflows/sdk-tests.yml"],
  ));
  if (sdk) checks.set(requirementKey(requirement("sdk-tests")), requirement("sdk-tests"));

  const e2e = paths.some((path) => matchesPath(
    path,
    ["gateway/", "verifier/", "tests/", "deploy/"],
    [
      "run_e2e.sh",
      "DEPLOY.md",
      ".env.production.example",
      "docker-compose.yml",
      "package.json",
      "bun.lock",
      "tsconfig.json",
      ".github/workflows/e2e.yml",
    ],
  ));
  if (e2e) checks.set(requirementKey(requirement("e2e")), requirement("e2e"));

  return [...checks.values()];
}

function newestByName(items) {
  const result = new Map();
  for (const item of items) {
    const key = `${item.source}:${item.name}`;
    const current = result.get(key);
    if (!current || Number(item.id) > Number(current.id)) result.set(key, item);
  }
  return result;
}

function normalizeCheckRuns(checkRuns) {
  return checkRuns.map((check) => ({
    id: check.id,
    name: check.name,
    source: `check:${check.app?.slug || "unknown"}`,
    status: check.status,
    conclusion: check.conclusion,
    description: check.output?.summary || check.output?.title || "",
    url: check.html_url || check.details_url || "",
  }));
}

function normalizeStatuses(statuses) {
  return statuses.map((status) => ({
    id: status.id,
    name: status.context,
    source: `status:${statusTargetHost(status.target_url) || status.creator?.login || "unknown"}`,
    status: status.state === "pending" ? "in_progress" : "completed",
    conclusion: status.state,
    description: status.description || "",
    url: status.target_url || "",
  }));
}

function statusTargetHost(targetUrl) {
  if (!targetUrl) return null;
  try {
    const hostname = new URL(targetUrl).hostname.toLowerCase();
    return hostname === "vercel.com" || hostname.endsWith(".vercel.com") ? "vercel.com" : hostname;
  } catch {
    return null;
  }
}

function evaluateChecks(required, observedItems) {
  const observed = newestByName(observedItems);
  const requiredItems = [...required];
  const missing = requiredItems
    .filter((item) => !observed.has(requirementKey(item)))
    .sort((a, b) => a.name.localeCompare(b.name));
  const pending = [];
  const failed = [];

  for (const item of observed.values()) {
    const conclusion = (item.conclusion || "").toLowerCase();
    const status = (item.status || "").toLowerCase();

    if (item.name === "Vercel" && /authorization required/i.test(item.description)) {
      pending.push({ ...item, reason: "manual Vercel authorization required" });
      continue;
    }
    if (status !== "completed" || !conclusion || conclusion === "pending") {
      pending.push(item);
      continue;
    }
    if (SUCCESSFUL_CONCLUSIONS.has(conclusion)) continue;
    if (conclusion === "skipped" && ALLOWED_SKIPPED_CHECKS.has(item.name)) continue;
    if (FAILING_CONCLUSIONS.has(conclusion) || conclusion === "error") {
      failed.push(item);
      continue;
    }
    failed.push({ ...item, description: item.description || `unexpected conclusion: ${conclusion}` });
  }

  return { missing, pending, failed, observed };
}

function latestReviewsByUser(reviews) {
  const latest = new Map();
  for (const review of reviews) {
    if (!review.user?.login) continue;
    const current = latest.get(review.user.login);
    if (!current || Number(review.id) > Number(current.id)) latest.set(review.user.login, review);
  }
  return latest;
}

async function evaluateReviews({ github, owner, repo, pullNumber, headSha, author }) {
  const reviews = await github.paginate(github.rest.pulls.listReviews, {
    owner,
    repo,
    pull_number: pullNumber,
    per_page: 100,
  });
  const latest = latestReviewsByUser(reviews);
  const changesRequested = [...latest.values()].filter((review) => review.state === "CHANGES_REQUESTED");
  const approvals = [];

  for (const review of latest.values()) {
    if (review.state !== "APPROVED" || review.commit_id !== headSha || review.user.login === author) continue;
    try {
      const { data } = await github.rest.repos.getCollaboratorPermissionLevel({
        owner,
        repo,
        username: review.user.login,
      });
      if (["admin", "maintain", "write"].includes(data.permission)) approvals.push(review.user.login);
    } catch (error) {
      if (error.status !== 404) throw error;
    }
  }

  return { approvals, changesRequested };
}

async function unresolvedThreads(github, owner, repo, pullNumber) {
  const query = `
    query($owner: String!, $repo: String!, $pullNumber: Int!, $cursor: String) {
      repository(owner: $owner, name: $repo) {
        pullRequest(number: $pullNumber) {
          reviewThreads(first: 100, after: $cursor) {
            nodes { id isResolved isOutdated }
            pageInfo { hasNextPage endCursor }
          }
        }
      }
    }
  `;
  const unresolved = [];
  let cursor = null;
  do {
    const data = await github.graphql(query, { owner, repo, pullNumber, cursor });
    const threads = data.repository.pullRequest.reviewThreads;
    unresolved.push(...threads.nodes.filter((thread) => !thread.isResolved));
    cursor = threads.pageInfo.hasNextPage ? threads.pageInfo.endCursor : null;
  } while (cursor);
  return unresolved;
}

function list(items) {
  if (!items.length) return "- None";
  return items.map((item) => `- ${item.name || item}`).join("\n");
}

function statusBody({ phase, sha, required = [], missing = [], pending = [], detail = "" }) {
  return [
    BOT_MARKER,
    "## Merge command",
    "",
    `**Status:** ${phase}`,
    sha ? `**Authorized head:** \`${sha}\`` : null,
    detail || null,
    "",
    required.length ? `<details><summary>Required checks (${required.length})</summary>\n\n${list(required)}\n</details>` : null,
    missing.length ? `\n**Waiting for checks to report:**\n${list(missing)}` : null,
    pending.length ? `\n**Checks still running or awaiting action:**\n${list(pending)}` : null,
    "",
    "A failed check or a new contributor commit cancels this authorization; post `/merge` again after fixing it.",
  ].filter(Boolean).join("\n");
}

async function upsertStatusComment(github, owner, repo, pullNumber, body) {
  const comments = await github.paginate(github.rest.issues.listComments, {
    owner,
    repo,
    issue_number: pullNumber,
    per_page: 100,
  });
  const existing = [...comments].reverse().find((comment) =>
    comment.user?.type === "Bot" && comment.body?.includes(BOT_MARKER));
  if (existing) {
    await github.rest.issues.updateComment({ owner, repo, comment_id: existing.id, body });
    return existing.id;
  }
  const { data } = await github.rest.issues.createComment({
    owner,
    repo,
    issue_number: pullNumber,
    body,
  });
  return data.id;
}

async function tokenRequest(token, method, path, body) {
  const response = await fetch(`https://api.github.com${path}`, {
    method,
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${token}`,
      "X-GitHub-Api-Version": "2022-11-28",
      "User-Agent": "microai-paygate-merge-command",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  if (!response.ok) {
    const error = new Error(payload.message || `${method} ${path} failed with ${response.status}`);
    error.status = response.status;
    throw error;
  }
  return payload;
}

async function waitForHeadChange(github, owner, repo, pullNumber, oldSha, sleep, timeoutMs = 120000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const { data } = await github.rest.pulls.get({ owner, repo, pull_number: pullNumber });
    if (data.head.sha !== oldSha) return data;
    await sleep(3000);
  }
  throw new Error("Timed out waiting for GitHub to update the pull request branch.");
}

async function assertExpectedBranchUpdate(github, owner, repo, newSha, oldHeadSha, baseSha) {
  const { data: commit } = await github.rest.repos.getCommit({ owner, repo, ref: newSha });
  const parents = new Set(commit.parents.map((parent) => parent.sha));
  if (!parents.has(oldHeadSha) || !parents.has(baseSha)) {
    throw new Error(
      "The PR head changed, but it was not the expected merge of the authorized head and `main`. Post `/merge` again.",
    );
  }
}

async function checkSnapshot(github, owner, repo, sha) {
  const [checkRuns, statuses] = await Promise.all([
    github.paginate(github.rest.checks.listForRef, { owner, repo, ref: sha, per_page: 100 }),
    github.paginate(github.rest.repos.listCommitStatusesForRef, { owner, repo, ref: sha, per_page: 100 }),
  ]);
  return [...normalizeCheckRuns(checkRuns), ...normalizeStatuses(statuses)];
}

async function compareBehind(github, owner, repo, baseSha, headSha) {
  const { data } = await github.rest.repos.compareCommits({ owner, repo, base: baseSha, head: headSha });
  return data.behind_by > 0;
}

async function assertUpToDate(github, owner, repo, baseSha, headSha) {
  if (await compareBehind(github, owner, repo, baseSha, headSha)) {
    throw new Error("The PR is still behind `main`. Post `/merge` again to update and revalidate.");
  }
}

async function run({
  github,
  context,
  core,
  token,
  sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
  pollIntervalMs = 30000,
  timeoutMs = 120 * 60 * 1000,
}) {
  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const pullNumber = context.payload.issue.number;
  let authorizedSha = null;
  let required = [];

  const reportFailure = async (message) => {
    await upsertStatusComment(github, owner, repo, pullNumber, statusBody({
      phase: "❌ Stopped",
      sha: authorizedSha,
      required,
      detail: message,
    }));
    core.setFailed(message);
  };

  try {
    if (!isMergeCommand(context.payload.comment.body)) throw new Error("The command must be exactly `/merge`.");
    if (Number(context.payload.sender?.id) !== AUTHORIZED_USER_ID) {
      throw new Error("Only the configured repository owner may use `/merge`.");
    }
    if (!token) throw new Error("Repository secret `MERGE_BOT_TOKEN` is not configured.");

    await github.rest.reactions.createForIssueComment({
      owner,
      repo,
      comment_id: context.payload.comment.id,
      content: "rocket",
    });

    let { data: pull } = await github.rest.pulls.get({ owner, repo, pull_number: pullNumber });
    if (pull.state !== "open") throw new Error("The pull request is not open.");
    if (pull.draft) throw new Error("Draft pull requests cannot be merged.");
    if (pull.base.ref !== "main") throw new Error("The merge bot only accepts pull requests targeting `main`.");
    if (pull.mergeable === false) throw new Error("The pull request has merge conflicts.");

    const files = await github.paginate(github.rest.pulls.listFiles, {
      owner,
      repo,
      pull_number: pullNumber,
      per_page: 100,
    });
    if (files.length !== pull.changed_files) {
      throw new Error(
        `GitHub returned ${files.length} of ${pull.changed_files} changed files; refusing an incomplete inspection.`,
      );
    }
    if (isWorkflowChange(files)) {
      throw new Error("Workflow-changing pull requests must be merged manually in v1.");
    }
    required = expectedChecks(files).sort((a, b) => a.name.localeCompare(b.name));

    const { data: baseBefore } = await github.rest.repos.getBranch({ owner, repo, branch: "main" });
    if (await compareBehind(github, owner, repo, baseBefore.commit.sha, pull.head.sha)) {
      const oldHeadSha = pull.head.sha;
      const baseSha = baseBefore.commit.sha;
      await upsertStatusComment(github, owner, repo, pullNumber, statusBody({
        phase: "🔄 Updating the branch from `main`",
        sha: pull.head.sha,
        required,
        detail: "Fresh checks and a fresh approval will be required on the updated head.",
      }));
      await tokenRequest(
        token,
        "PUT",
        `/repos/${owner}/${repo}/pulls/${pullNumber}/update-branch`,
        { expected_head_sha: oldHeadSha },
      );
      pull = await waitForHeadChange(github, owner, repo, pullNumber, oldHeadSha, sleep);
      await assertExpectedBranchUpdate(github, owner, repo, pull.head.sha, oldHeadSha, baseSha);
    }

    authorizedSha = pull.head.sha;
    const { data: authorizedBase } = await github.rest.repos.getBranch({ owner, repo, branch: "main" });
    const authorizedBaseSha = authorizedBase.commit.sha;
    await assertUpToDate(github, owner, repo, authorizedBaseSha, authorizedSha);
    const deadline = Date.now() + timeoutMs;

    while (Date.now() < deadline) {
      const { data: currentPull } = await github.rest.pulls.get({ owner, repo, pull_number: pullNumber });
      if (currentPull.head.sha !== authorizedSha) {
        throw new Error(`The PR head changed from \`${authorizedSha}\` to \`${currentPull.head.sha}\`.`);
      }
      if (currentPull.state !== "open" || currentPull.draft) throw new Error("The pull request is no longer open and ready.");

      const { data: currentBase } = await github.rest.repos.getBranch({ owner, repo, branch: "main" });
      if (currentBase.commit.sha !== authorizedBaseSha) {
        throw new Error("`main` changed while the bot was waiting. Post `/merge` again to update and revalidate.");
      }

      const [items, reviewState, threads] = await Promise.all([
        checkSnapshot(github, owner, repo, authorizedSha),
        evaluateReviews({
          github,
          owner,
          repo,
          pullNumber,
          headSha: authorizedSha,
          author: currentPull.user.login,
        }),
        unresolvedThreads(github, owner, repo, pullNumber),
      ]);
      const checks = evaluateChecks(required, items);
      if (checks.failed.length) {
        throw new Error(`Failed checks:\n${list(checks.failed)}`);
      }
      if (reviewState.changesRequested.length) {
        throw new Error(`Changes are requested by: ${reviewState.changesRequested.map((review) => review.user.login).join(", ")}.`);
      }

      const waiting = [];
      if (!reviewState.approvals.length) waiting.push({ name: "a current approval from a write-access reviewer" });
      if (threads.length) waiting.push({ name: `${threads.length} unresolved review thread(s)` });
      waiting.push(...checks.pending);

      if (!checks.missing.length && !waiting.length) {
        const [finalItems, finalReviewState, finalThreads] = await Promise.all([
          checkSnapshot(github, owner, repo, authorizedSha),
          evaluateReviews({
            github,
            owner,
            repo,
            pullNumber,
            headSha: authorizedSha,
            author: currentPull.user.login,
          }),
          unresolvedThreads(github, owner, repo, pullNumber),
        ]);
        const finalChecks = evaluateChecks(required, finalItems);
        const { data: finalPull } = await github.rest.pulls.get({ owner, repo, pull_number: pullNumber });
        const { data: finalBase } = await github.rest.repos.getBranch({ owner, repo, branch: "main" });
        if (finalPull.head.sha !== authorizedSha || finalBase.commit.sha !== authorizedBaseSha) {
          throw new Error("The PR or `main` changed during the final merge check.");
        }
        if (
          finalChecks.failed.length ||
          finalChecks.missing.length ||
          finalChecks.pending.length ||
          finalReviewState.changesRequested.length ||
          !finalReviewState.approvals.length ||
          finalThreads.length
        ) {
          await sleep(pollIntervalMs);
          continue;
        }

        await upsertStatusComment(github, owner, repo, pullNumber, statusBody({
          phase: "🚀 All gates passed; squash-merging",
          sha: authorizedSha,
          required,
          detail: `Approval: @${finalReviewState.approvals[0]}`,
        }));
        const result = await tokenRequest(
          token,
          "PUT",
          `/repos/${owner}/${repo}/pulls/${pullNumber}/merge`,
          { sha: authorizedSha, merge_method: "squash" },
        );
        if (!result.merged) throw new Error(result.message || "GitHub declined the merge.");
        await upsertStatusComment(github, owner, repo, pullNumber, statusBody({
          phase: "✅ Squash-merged",
          sha: authorizedSha,
          required,
          detail: `Merge commit: \`${result.sha}\``,
        }));
        return result;
      }

      const vercel = waiting.find((item) => item.name === "Vercel" && item.reason);
      const vercelDetail = vercel?.url
        ? `Vercel needs manual authorization: ${vercel.url}`
        : "The bot will keep watching this exact commit.";
      await upsertStatusComment(github, owner, repo, pullNumber, statusBody({
        phase: "⏳ Waiting for merge gates",
        sha: authorizedSha,
        required,
        missing: checks.missing,
        pending: waiting,
        detail: vercelDetail,
      }));
      await sleep(pollIntervalMs);
    }

    throw new Error("Timed out after 120 minutes waiting for merge gates.");
  } catch (error) {
    await reportFailure(error.message);
    return null;
  }
}

module.exports = {
  ALLOWED_SKIPPED_CHECKS,
  ALWAYS_REQUIRED_CHECKS,
  AUTHORIZED_USER_ID,
  assertExpectedBranchUpdate,
  assertUpToDate,
  BOT_MARKER,
  evaluateChecks,
  expectedChecks,
  isMergeCommand,
  isWorkflowChange,
  latestReviewsByUser,
  normalizeCheckRuns,
  normalizeStatuses,
  requirement,
  run,
  statusBody,
};
