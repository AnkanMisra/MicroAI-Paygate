# GitHub Automation

This repository keeps automation narrow: deterministic workflows own mechanical checks, while maintainers own difficulty, validity, security severity, and program approval.

## Label ownership

- `.github/labels.json` is the source of truth for labels managed by automation.
- The PR labeler owns path-derived language, documentation, DevOps, and testing labels.
- The issue triage bot reads exact Issue Form fields. It does not classify free-form prose by broad keywords.
- `triage` is added only when a new or reopened issue has no ready or terminal state.
- `good first issue`, `level:*`, `gssoc:approved`, `invalid`, `wontfix`, and security severity remain maintainer decisions.
- Automatic claims are allowed only on issues labeled `good first issue`, `help wanted`, or `gssoc:approved`. Contributors can use `/unclaim` to release their own assignment.

## Required post-merge repository settings

Create a repository ruleset targeting `main` with:

1. Require changes through a pull request.
2. Require at least one approving review.
3. Require review from Code Owners.
4. Require all review conversations to be resolved.
5. Block force pushes and branch deletion.
6. Require `dependency-review`, `check-branch-status`, `Analyze (go)`, `Analyze (javascript-typescript)`, and `Analyze (rust)` after each has completed successfully at least once.
7. Dismiss stale approvals when new commits are pushed.

Service-specific checks are path-filtered. Keep them visible on relevant PRs, but do not make a skipped path-filtered check globally required until CI exposes one always-on aggregate status.

After the ruleset is active, enable automatic head-branch updates, automatic deletion of merged branches, and auto-merge if the repository's merge policy allows them.

## Added security and quality checks

- Dependency Review rejects newly introduced dependencies with known vulnerabilities of moderate severity or higher.
- OpenSSF Scorecard publishes supply-chain findings to code scanning.
- Codecov receives separate gateway, web, and SDK coverage flags and enforces patch coverage.
- Automation Validation runs the issue-triage unit tests and `actionlint` for workflow changes.
- Dependabot groups minor/patch updates, limits update noise, and includes the TypeScript SDK workspace.

## External setup

- Connect the repository to Codecov and authorize OIDC uploads. Upload failures are informational during onboarding; change `fail_ci_if_error` to `true` only after a successful default-branch upload.
- Review the first OpenSSF Scorecard run in the Security tab.
- Release automation is intentionally deferred until the SDK packaging and npm scope decisions are complete.
- Use one conversational review bot at a time. CodeQL and Dependency Review are complementary security checks, not conversational reviewers.

## Rollback

If an automation change blocks contributions:

1. Disable the affected workflow from the Actions page.
2. Revert the automation PR on `main`.
3. Remove the affected required check from the ruleset while the revert runs.
4. Re-run the last known-good workflow and confirm new PRs can report checks normally.

Do not delete labels during rollback; disabling label workflows is sufficient and preserves issue history.
