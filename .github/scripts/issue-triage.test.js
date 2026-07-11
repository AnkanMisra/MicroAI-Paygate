const assert = require("node:assert/strict");
const test = require("node:test");

const {
  inferStructuredLabels,
  isClaimable,
  requestedClaim,
  requestedUnclaim,
  sectionValue,
  shouldAddTriage,
  staleInferredLabels,
} = require("./issue-triage");

function issue(overrides = {}) {
  return {
    title: "A focused issue",
    body: "",
    labels: [],
    ...overrides,
  };
}

test("sectionValue reads exact issue-form sections", () => {
  const body = [
    "### Affected component",
    "",
    "Gateway",
    "",
    "### Summary",
    "",
    "Something happened",
  ].join("\n");
  assert.equal(sectionValue(body, "Affected component"), "Gateway");
  assert.equal(sectionValue(body, "Missing"), null);
});

test("structured issue classification uses title and component fields only", () => {
  const result = inferStructuredLabels(issue({
    title: "[Bug]: Gateway rejects a valid request",
    body: "### Affected component\n\nGateway\n\n### Summary\n\nREADME is mentioned here",
  }));
  assert.equal(result.isStructured, true);
  assert.deepEqual([...result.labels].sort(), ["bug", "go", "type:bug"]);
});

test("free-form keywords do not create labels", () => {
  const result = inferStructuredLabels(issue({
    body: "Please update gateway/openapi.yaml and README documentation",
  }));
  assert.equal(result.isStructured, false);
  assert.deepEqual([...result.labels], []);
});

test("unstructured edits remove stale inferred labels only", () => {
  const currentIssue = issue({ body: "Free-form issue text" });
  const stale = staleInferredLabels(
    currentIssue,
    { body: { from: "### Affected component\n\nGateway" } },
    new Set(["go", "type:bug", "good first issue"]),
  );
  assert.deepEqual(stale, ["go"]);
});

test("free-form edits preserve maintainer-applied inferred labels", () => {
  const currentIssue = issue({ body: "Updated free-form issue text" });
  const stale = staleInferredLabels(
    currentIssue,
    { body: { from: "Original free-form issue text" } },
    new Set(["go", "documentation"]),
  );
  assert.deepEqual(stale, []);
});

test("structured type edits replace stale category labels", () => {
  const currentIssue = issue({
    title: "[Feature]: Add a gateway option",
    body: "### Affected component\n\nGateway",
    labels: [{ name: "bug" }, { name: "type:bug" }],
  });
  const inferred = inferStructuredLabels(currentIssue);
  const stale = staleInferredLabels(
    currentIssue,
    { title: { from: "[Bug]: Gateway rejects a valid request" } },
    new Set(["bug", "type:bug", "go"]),
  );
  assert.deepEqual([...inferred.labels].sort(), ["enhancement", "go", "type:feature"]);
  assert.deepEqual(stale.sort(), ["bug", "type:bug"]);
});

test("structured titles replace conflicting type labels without edit history", () => {
  const currentIssue = issue({
    title: "[Feature]: Add a gateway option",
    body: "### Affected component\n\nGateway",
  });
  const stale = staleInferredLabels(
    currentIssue,
    undefined,
    new Set(["bug", "type:bug", "go"]),
  );
  assert.deepEqual(stale.sort(), ["bug", "type:bug"]);
});

test("body-only structured edits preserve maintainer type labels", () => {
  const currentIssue = issue({
    title: "Gateway request fails in one environment",
    body: "### Affected component\n\nGateway",
  });
  const stale = staleInferredLabels(
    currentIssue,
    { body: { from: "Free-form issue body" } },
    new Set(["bug"]),
  );
  assert.deepEqual(stale, []);
});

test("triage is skipped for ready or terminal issues", () => {
  assert.equal(shouldAddTriage("opened", issue()), true);
  for (const name of [
    "good first issue",
    "help wanted",
    "gssoc:approved",
    "invalid",
    "wontfix",
  ]) {
    assert.equal(
      shouldAddTriage("opened", issue({ labels: [{ name }] })),
      false,
    );
  }
  assert.equal(shouldAddTriage("edited", issue()), false);
});

test("claims require an explicit contributor-ready label", () => {
  assert.equal(isClaimable(issue({ labels: [{ name: "triage" }] })), false);
  for (const name of ["good first issue", "help wanted", "gssoc:approved"]) {
    assert.equal(isClaimable(issue({ labels: [{ name }] })), true);
  }
});

test("claim and unclaim commands are recognized without matching normal prose", () => {
  assert.equal(requestedClaim("/claim"), true);
  assert.equal(requestedClaim("Please assign me"), true);
  assert.equal(requestedClaim("assignment behavior docs"), false);
  assert.equal(requestedUnclaim("/unclaim"), true);
  assert.equal(requestedUnclaim("Please unclaim this"), false);
});
