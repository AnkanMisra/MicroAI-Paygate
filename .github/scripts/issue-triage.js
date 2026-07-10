const labelDefinitions = require("../labels.json");

const TERMINAL_OR_READY_STATES = new Set([
  "good first issue",
  "help wanted",
  "gssoc:approved",
  "invalid",
  "wontfix",
]);

const CLAIMABLE_LABELS = new Set([
  "good first issue",
  "help wanted",
  "gssoc:approved",
]);

const INFERRED_LABELS = new Set([
  "bug",
  "type:bug",
  "enhancement",
  "type:feature",
  "documentation",
  "type:docs",
  "type:devops",
  "go",
  "rust",
  "TypeScript",
  "type:testing",
]);

const COMPONENT_LABELS = new Map([
  ["gateway", ["go"]],
  ["verifier", ["rust"]],
  ["web", ["TypeScript"]],
  ["e2e tests", ["type:testing"]],
  ["docker/compose", ["type:devops"]],
  ["deployment", ["type:devops"]],
  ["documentation", ["documentation", "type:docs"]],
]);

function labelNames(issue) {
  return new Set((issue.labels || []).map((label) =>
    typeof label === "string" ? label : label.name,
  ));
}

function escapeRegex(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function sectionValue(body, heading) {
  const expression = new RegExp(
    `^###\\s+${escapeRegex(heading)}\\s*\\n+([\\s\\S]*?)(?=\\n###\\s+|$)`,
    "im",
  );
  const match = expression.exec(body || "");
  return match ? match[1].trim() : null;
}

function inferStructuredLabels(issue) {
  const labels = new Set();
  const title = issue.title || "";
  const component = sectionValue(issue.body, "Affected component");
  const current = labelNames(issue);

  if (/^\[bug\]\s*:/i.test(title) || current.has("bug")) {
    labels.add("bug");
    labels.add("type:bug");
  } else if (/^\[feature\]\s*:/i.test(title) || current.has("enhancement")) {
    labels.add("enhancement");
    labels.add("type:feature");
  }

  if (component) {
    for (const label of COMPONENT_LABELS.get(component.toLowerCase()) || []) {
      labels.add(label);
    }
  }

  return { labels, isStructured: component !== null };
}

function shouldAddTriage(action, issue) {
  if (action !== "opened" && action !== "reopened") return false;
  const current = labelNames(issue);
  return ![...TERMINAL_OR_READY_STATES].some((label) => current.has(label));
}

function isClaimable(issue) {
  const current = labelNames(issue);
  return [...CLAIMABLE_LABELS].some((label) => current.has(label));
}

function requestedClaim(body) {
  return (
    /^\s*\/(?:claim|assign)\b/im.test(body || "") ||
    /\bassign me\b/i.test(body || "")
  );
}

function requestedUnclaim(body) {
  return /^\s*\/unclaim\b/im.test(body || "");
}

async function run({ github, context }) {
  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const issue = context.payload.issue;

  const repositoryLabels = await github.paginate(
    github.rest.issues.listLabelsForRepo,
    { owner, repo, per_page: 100 },
  );
  const existingLabels = new Set(repositoryLabels.map((label) => label.name));

  for (const label of labelDefinitions) {
    if (existingLabels.has(label.name)) continue;
    try {
      await github.rest.issues.createLabel({ owner, repo, ...label });
      existingLabels.add(label.name);
    } catch (error) {
      if (error.status !== 422) throw error;
    }
  }

  async function fetchIssue() {
    const { data } = await github.rest.issues.get({
      owner,
      repo,
      issue_number: issue.number,
    });
    return data;
  }

  async function addLabels(labels) {
    const filtered = [...new Set(labels)].filter((label) => existingLabels.has(label));
    if (filtered.length === 0) return;
    await github.rest.issues.addLabels({
      owner,
      repo,
      issue_number: issue.number,
      labels: filtered,
    });
  }

  async function removeLabels(labels) {
    for (const label of labels) {
      try {
        await github.rest.issues.removeLabel({
          owner,
          repo,
          issue_number: issue.number,
          name: label,
        });
      } catch (error) {
        if (error.status !== 404) throw error;
      }
    }
  }

  async function comment(body) {
    await github.rest.issues.createComment({
      owner,
      repo,
      issue_number: issue.number,
      body,
    });
  }

  async function assign(login) {
    try {
      await github.rest.issues.addAssignees({
        owner,
        repo,
        issue_number: issue.number,
        assignees: [login],
      });
      return true;
    } catch (error) {
      if (error.status === 403 || error.status === 422) return false;
      throw error;
    }
  }

  if (context.eventName === "issues") {
    const inferred = inferStructuredLabels(issue);
    if (inferred.isStructured && context.payload.action === "edited") {
      const current = labelNames(await fetchIssue());
      const stale = [...INFERRED_LABELS].filter(
        (label) => current.has(label) && !inferred.labels.has(label),
      );
      await removeLabels(stale);
    }

    await addLabels(inferred.labels);
    if (shouldAddTriage(context.payload.action, issue)) {
      await addLabels(["triage"]);
    }

    const wantsAssignment = requestedClaim(issue.body);
    if (!wantsAssignment) return;

    const currentIssue = await fetchIssue();
    if (!isClaimable(currentIssue)) {
      await comment(
        `@${issue.user.login} this issue must be marked \`good first issue\`, ` +
        "`help wanted`, or `gssoc:approved` before it can be assigned automatically.",
      );
      return;
    }
    if (currentIssue.assignees.length > 0) return;

    const assigned = await assign(issue.user.login);
    await comment(
      assigned
        ? `Assigned @${issue.user.login}.`
        : `@${issue.user.login} GitHub did not allow automatic assignment. A maintainer will need to assign this issue manually.`,
    );
    return;
  }

  const issueComment = context.payload.comment;
  if (issueComment.user.type === "Bot") return;
  if (!requestedClaim(issueComment.body) && !requestedUnclaim(issueComment.body)) return;

  const commenter = issueComment.user.login;
  const currentIssue = await fetchIssue();
  const assignees = currentIssue.assignees.map((assignee) => assignee.login);

  if (requestedUnclaim(issueComment.body)) {
    if (!assignees.includes(commenter)) return;
    await github.rest.issues.removeAssignees({
      owner,
      repo,
      issue_number: issue.number,
      assignees: [commenter],
    });
    await comment(`Unassigned @${commenter}.`);
    return;
  }

  if (!isClaimable(currentIssue)) {
    await comment(
      `@${commenter} this issue is not ready for automatic assignment. ` +
      "A maintainer must first add `good first issue`, `help wanted`, or `gssoc:approved`.",
    );
    return;
  }
  if (assignees.includes(commenter)) return;
  if (assignees.length > 0) {
    await comment(
      `@${commenter} this issue is already assigned to ${assignees
        .map((login) => `@${login}`)
        .join(", ")}.`,
    );
    return;
  }

  const assigned = await assign(commenter);
  await comment(
    assigned
      ? `Assigned @${commenter}.`
      : `@${commenter} GitHub did not allow automatic assignment. A maintainer will need to assign this issue manually.`,
  );
}

module.exports = {
  inferStructuredLabels,
  isClaimable,
  labelNames,
  requestedClaim,
  requestedUnclaim,
  sectionValue,
  shouldAddTriage,
  run,
};
