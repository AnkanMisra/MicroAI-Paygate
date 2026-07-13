const WELCOME_MARKER = "<!-- microai-first-contribution-welcome -->";
const FIRST_CONTRIBUTION_ASSOCIATIONS = new Set([
  "FIRST_TIMER",
  "FIRST_TIME_CONTRIBUTOR",
]);

function shouldWelcome(pullRequest) {
  const author = pullRequest?.user;
  return Boolean(
    author?.login &&
    author.type !== "Bot" &&
    FIRST_CONTRIBUTION_ASSOCIATIONS.has(pullRequest.author_association),
  );
}

function welcomeBody(author) {
  return [
    WELCOME_MARKER,
    `Hi @${author}, thanks for making your first contribution to MicroAI Paygate!`,
    "",
    "A maintainer will review the pull request when available. In the meantime, please make sure the relevant checks pass and that the PR description explains the change.",
    "",
    "If the project has been useful to you, an optional [GitHub star](https://github.com/AnkanMisra/MicroAI-Paygate) helps other contributors discover it.",
  ].join("\n");
}

async function run({ github, context }) {
  const pullRequest = context.payload.pull_request;
  if (!shouldWelcome(pullRequest)) return "skipped";

  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const issueNumber = pullRequest.number;
  const comments = await github.paginate(
    github.rest.issues.listComments,
    { owner, repo, issue_number: issueNumber, per_page: 100 },
  );
  const alreadyWelcomed = comments.some((comment) =>
    comment.user?.type === "Bot" && comment.body?.includes(WELCOME_MARKER),
  );
  if (alreadyWelcomed) return "already-welcomed";

  await github.rest.issues.createComment({
    owner,
    repo,
    issue_number: issueNumber,
    body: welcomeBody(pullRequest.user.login),
  });
  return "welcomed";
}

module.exports = {
  FIRST_CONTRIBUTION_ASSOCIATIONS,
  WELCOME_MARKER,
  run,
  shouldWelcome,
  welcomeBody,
};
