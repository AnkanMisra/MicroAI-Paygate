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

After the ruleset is active, enable automatic head-branch updates and automatic deletion of merged branches. Keep repository auto-merge disabled when using the merge command bot below.

## Merge command bot

Maintainer `AnkanMisra` can post an exact `/merge` comment on a ready pull request. The bot binds the command to one head commit, updates an out-of-date branch when possible, waits for a current write-access approval, resolved review threads, every applicable CI job, all CodeQL jobs, and Vercel, then squash-merges that exact commit.

### Required setup

1. Create a classic personal access token owned by `AnkanMisra` with only the `public_repo` scope and a 90-day expiration.
2. Store it as the Actions repository secret `MERGE_BOT_TOKEN` and record its expiration in the maintainer calendar. Never expose it as a repository variable.
3. Update the `Protect main` ruleset to require one approval and the `Vercel` context in addition to the existing required checks. Keep strict branch freshness and resolved conversations enabled, and do not grant `AnkanMisra` or the token a bypass. The bot verifies these protections before waiting and again immediately before merging.
4. Keep repository auto-merge disabled; the command bot performs the guarded squash merge.

The privileged `issue_comment` workflow checks out only the default branch and never the contributor branch. It uses the short-lived `GITHUB_TOKEN` for reads and status comments; `MERGE_BOT_TOKEN` is used only for the update-branch and exact-SHA merge API requests. Fork authors must enable maintainer edits for automatic branch updates. Pull requests changing `.github/workflows/` or the privileged `.github/scripts/merge-command.js` runtime are rejected by v1 and must be merged manually.

If Vercel reports `Authorization required to deploy`, use its authorization link. The bot treats that state as pending and continues only after Vercel reports success. A failed check, a requested-changes review, a new contributor commit, or a change to `main` cancels the authorization and requires a fresh `/merge`.

### Merge bot rollback

1. Disable the `Merge Command Bot` workflow.
2. Delete or rotate `MERGE_BOT_TOKEN` immediately.
3. Revert the workflow, script, tests, and this documentation together.
4. If manual merging is blocked, temporarily remove the added `Vercel` requirement while keeping the other main-branch protections intact.

## Added security and quality checks

- Dependency Review rejects newly introduced dependencies with known vulnerabilities of moderate severity or higher.
- OpenSSF Scorecard publishes supply-chain findings to code scanning.
- Codecov receives separate gateway, web, and SDK coverage flags and enforces patch coverage.
- Automation Validation runs the issue-triage unit tests and `actionlint` for workflow changes.
- Dependabot groups minor/patch updates, limits update noise, and includes the TypeScript SDK workspace.

## First-time contributor welcome

- `.github/workflows/welcome-contributor.yml` runs only when a pull request is opened and uses GitHub's `FIRST_TIMER` and `FIRST_TIME_CONTRIBUTOR` associations.
- Bots, maintainers, members, and returning contributors are not greeted.
- The message prioritizes contribution guidance. The repository star link is optional and secondary.
- A hidden marker prevents duplicate comments when a workflow run is retried.
- The workflow checks out only the trusted default branch and never executes code from the contributor's pull request.
- To roll it back without affecting other automation, disable the `Welcome First-Time Contributors` workflow or revert its workflow and script files.

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
