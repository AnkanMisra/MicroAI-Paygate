const assert = require("node:assert/strict");
const test = require("node:test");

const {
  WELCOME_MARKER,
  run,
  shouldWelcome,
  welcomeBody,
} = require("./welcome-contributor");

function pullRequest(overrides = {}) {
  return {
    number: 42,
    author_association: "FIRST_TIME_CONTRIBUTOR",
    user: { login: "new-contributor", type: "User" },
    ...overrides,
  };
}

function harness({ pullRequestOverrides = {}, comments = [] } = {}) {
  const created = [];
  return {
    context: {
      payload: { pull_request: pullRequest(pullRequestOverrides) },
      repo: { owner: "AnkanMisra", repo: "MicroAI-Paygate" },
    },
    created,
    github: {
      paginate: async () => comments,
      rest: {
        issues: {
          listComments: async () => ({ data: comments }),
          createComment: async (input) => created.push(input),
        },
      },
    },
  };
}

test("only first contributions from people receive a welcome", () => {
  assert.equal(shouldWelcome(pullRequest()), true);
  assert.equal(
    shouldWelcome(pullRequest({ author_association: "FIRST_TIMER" })),
    true,
  );
  assert.equal(
    shouldWelcome(pullRequest({ author_association: "CONTRIBUTOR" })),
    false,
  );
  assert.equal(
    shouldWelcome(pullRequest({ user: { login: "dependabot[bot]", type: "Bot" } })),
    false,
  );
});

test("welcome is contributor-focused and keeps the star request optional", () => {
  const body = welcomeBody("new-contributor");
  assert.match(body, /thanks for making your first contribution/);
  assert.match(body, /optional \[GitHub star\]/);
  assert.ok(body.startsWith(WELCOME_MARKER));
});

test("run creates one welcome comment", async () => {
  const state = harness();
  assert.equal(await run(state), "welcomed");
  assert.equal(state.created.length, 1);
  assert.equal(state.created[0].issue_number, 42);
  assert.match(state.created[0].body, /@new-contributor/);
});

test("run does not duplicate an existing bot welcome", async () => {
  const state = harness({
    comments: [{ body: WELCOME_MARKER, user: { type: "Bot" } }],
  });
  assert.equal(await run(state), "already-welcomed");
  assert.equal(state.created.length, 0);
});

test("run skips maintainers and returning contributors", async () => {
  for (const authorAssociation of ["OWNER", "MEMBER", "CONTRIBUTOR", "NONE"]) {
    const state = harness({
      pullRequestOverrides: { author_association: authorAssociation },
    });
    assert.equal(await run(state), "skipped");
    assert.equal(state.created.length, 0);
  }
});
