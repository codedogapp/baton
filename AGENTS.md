# AGENTS.md

The canonical rules for everyone who works on baton: the user and the coding agent. If anything else disagrees with this file, this file wins. It changes only with the user's approval.

- What we build, and in which order: [`docs/plan.md`](docs/plan.md).
- How we build it: this file.

## Working agreement

- **Bit by bit.** A step is one small, focused change: one package or one component, about 150 hand-written lines or fewer, plus its tests. Never generate large batches of code.
- **Scope discipline.** Do exactly the step. No drive-by refactors. Ideas go into `docs/plan.md`, not into the code.
- **Decide, flag, go on.** The lead makes the design calls that aren't decided here or in the plan, records each one as a decision in `docs/plan.md`, and flags it for the user's end review. Ask the user only for what only the user can give or decide: a token or an account, a change outside the repo, a product choice that's costly to undo, and the stop after three rounds.
- **Verify facts.** Check versions, APIs and tool options against their source (registry, docs, `--help`) before using them. Never guess.
- **Stay in the repo.** Don't change anything outside it without the user's OK: not the user's Copilot CLI config (`~/.copilot/`), not their tmux sessions, not other repos.
- **Spikes live outside the repo**, with the user's standing OK. A spike's code is throwaway: it lives in `../baton-spikes/<name>/`, next to the repo, and is never committed. Its findings reach the repo through a docs PR on the spike's `step/<id>` branch.

## Pipeline

The main session does all the work itself, in the open, so the user can follow it. No sub-agents. Every step goes through these roles, in order:

1. **Lead**: writes the step brief (goal, scope, out of scope, acceptance criteria). The brief goes into the PR description.
2. **Engineer**: implements the step with unit tests and runs every gate until green.
3. **QA**: checks every acceptance criterion, runs every gate, probes edge cases and failure paths in our code and config, and adds missing tests. It doesn't hunt for ways around the upstream tools.
4. **Reviewer**: reviews the diff against this file (separation of concerns, package boundaries, naming, KISS, YAGNI, error handling, test quality, fit with the architecture) and marks each finding blocking or non-blocking.
5. **Lead**: keeps the docs in sync, commits, opens the PR, and merges it once its CI is green.
6. **User**: reviews the merged PRs at the end and asks for any changes (the user's standing OK of 2026-10-31). Each PR body says it was merged under that OK.

- QA and review run once per step. Defects and blocking findings go back to the engineer. A small fix (a few lines of config or docs) then needs only a green `mise run check`; a bigger one goes through QA and review again. After three rounds, stop and ask the user.
- Non-blocking findings go into the PR description, not into another round.
- Known upstream gaps go into plan §5 without asking the user.
- Progress notes to the user are short and factual: what changed, the commands run and their results, open issues.

## Definition of done

1. `mise run check` passes locally.
2. The PR's CI is green, including the SonarQube Cloud quality gate.
3. The lead has merged the PR.

Never report a step as done before `mise run check` has passed, and say which gates actually ran.

## Git and pull requests

- Repo: [`codedogapp/baton`](https://github.com/codedogapp/baton), public. Default branch `main`.
- One step, one branch (`step/<id>`, ids from plan §0) and one PR. The lead squash-merges once CI is green, with an explicit subject and body: `gh pr merge --squash --subject "…" --body "…"`.
- Commit messages and PR titles follow Conventional Commits (`feat(flow): …`, `fix: …`, `build: …`, `ci: …`, `docs: …`): an imperative subject of 72 characters or fewer, and a body that says why when it isn't obvious.
- **No AI attribution.** Never add `Co-authored-by` trailers for Copilot or any other AI agent.
- Never write close, fix or resolve next to an issue or PR number unless the PR is meant to close it.
- Never force-push `main` or rewrite pushed history.
- Secrets never go into git or chat. Tokens live in the login Keychain on the Mac; local settings in `mise.local.toml` (gitignored); CI secrets in GitHub Actions secrets.
- The repo is public: no personal identifiers, work-project names, real Jira keys or third-party material in tracked files, PR bodies or screenshots. Tests and docs use made-up tickets such as `DEMO-1`.

## Engineering principles

- Clean code, KISS, YAGNI and separation of concerns.
- **One package, one job.** The package name says what it does. No `util`, `common`, `helpers` or `misc` packages; no god packages or god structs.
- **One concern per file**, and files stay short (see the gates).
- Small functions, early returns, shallow nesting.
- Domain states and enums get their own types, never bare strings.
- Prefer the standard library; never re-implement it.
- No speculative abstractions: an interface needs two implementations or a test seam; shared code needs two users.
- Pass dependencies explicitly. No package-level mutable state.
- Wrap errors with context (`fmt.Errorf("load feature: %w", err)`) and handle each error once.
- Comment only where the code needs clarification. No heavy commenting.
- Tests check behaviour, not implementation details. Table-driven where it helps; no sleeps: wait on a condition with a deadline.

## Architecture

- **One binary, `baton`**, with a subcommand per job: `baton start` opens the orchestrator, `baton feature start` gives a feature its worktree and its panes, `baton handover`, `baton wait`, `baton ask` and `baton answer` move the work between the roles, `baton hook` serves Copilot CLI's hooks, `baton deliver` pushes and opens the pull request. Plan §3 has the design.
- **One Go module** at the root: `github.com/codedogapp/baton`. The `main` package stays thin wiring; everything else lives in `internal/` packages.
- **The binary owns the flow.** Who works next, the round count, the cap and the checks on a handover live in Go code with tests, never in a prompt. Prompts tell a role how to do its job, not whom to call.
- **Every role is a visible, interactive Copilot CLI pane.** Never `--mode autopilot`, never headless, never `-p` for a role. The user can read and type into any pane, and talks to the orchestrator only.
- **Outward actions wait for the user's word.** Pushes, pull requests, merges and Jira writes go through `baton deliver` in the orchestrator's pane, where the guard hook asks the user; the role panes can't make them.
- **State lives on disk under the git common directory**, never in the repo's tree and never in a commit, so a run can resume.

## Toolchain

- **mise** manages every tool and task, except tools that macOS and the CI runners already ship (git, curl, jq); those run at the OS's version. Every task has a description.
- **Latest versions.** Everything we install runs on its newest release.
  - mise tools use `latest` in `mise.toml`. mise skips releases younger than 24 h (its `minimum_release_age` default), and mise-action does the same for mise itself. CI installs mise and the tools afresh on every run, with mise-action's cache off. Update local tools with `mise upgrade --local`; a plain `mise upgrade` also changes the user's global tools.
  - The rest is pinned exactly at its newest release, checked against its registry: GitHub Actions by commit SHA with a `# vX.Y.Z` comment, Go modules in `go.mod`. Renovate keeps these current.
- **Nothing deprecated.** Never use a deprecated tool, API, endpoint, parameter, field or flag; use its replacement. Check the source's deprecation notes before picking one.
- **Go**: the latest release. Before editing Go code, run the `use-modern-go` skill's `list` for the file and follow its guidelines.
- **Go lint and format**: golangci-lint, based on the user's labrys config; gofumpt and golines at 100 columns.
- **Mirror the user's repos** before inventing a convention: nudge (this file, CI, gates), Bakhyard (the pipeline that baton runs) and labrys (Go lint).

## Tests and live checks

- **No model calls in tests.** Unit and integration tests run a stand-in `copilot` (a script on `PATH`, such as `testdata/copilot`, which plays each role in the walking skeleton), never the real one.
- **tmux on its own socket.** Every tmux server that a test, a smoke run or a live check starts gets a config file of its own (`-f /dev/null`, or a file of the test's), so the user's `~/.tmux.conf` and its plugins never load, and runs without `TMUX` and `TMUX_PANE` (`env -u TMUX -u TMUX_PANE`), so a slip can't reach the user's server. Tests run tmux with `-L baton-test-<n>`, in a `TMUX_TMPDIR` and a `HOME` of their own, and kill only their servers. The walking skeleton (`TestSkeleton`) runs baton itself, so it runs tmux as a smoke run does, on the default socket, in a `TMUX_TMPDIR` (`/tmp/baton-test-skeleton-*`) and a `HOME` of its own. Smoke runs and live checks use a socket of their own, such as `-L baton-live`. baton itself uses tmux's default socket, or `TMUX`'s, so a smoke run of the binary gets a `TMUX_TMPDIR` and a `HOME` of its own, and starts that server first, with `-f /dev/null`; it never runs on the user's default server without the user's OK. The tmux tests' `TestMain` ends every server whose socket is in its `TMUX_TMPDIR`, even one that a test started on another socket, before it removes that folder. A test run that fails or stops early skips its cleanup, so after one, look for leftover servers (`pgrep -lf '^tmux .*(baton-test-|/Test)'`: a test's server shows its `-L baton-test-<n>`, or its config file in the test's temporary folder), end each on its own socket, or with `kill <pid>` once its socket is gone, and remove leftover `baton-tmux-<n>` folders in `/tmp` and `baton-test-skeleton-*` folders in `/tmp` and `$TMPDIR`. Never kill the user's tmux server, and never touch a session or window that baton didn't create.
- **git without the user's config.** Tests run git, theirs and baton's, with `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, `LC_ALL=C` and a test author, and without the `GIT_*` variables that a hook of git's sets, such as `GIT_DIR`: each package's `TestMain` sets them. A test spells out git's arguments, so it runs no command that it doesn't show.
- **Copilot CLI config stays untouched.** Develop with `--plugin-dir`, flags or a throwaway `COPILOT_HOME`, never by changing `~/.copilot/`.
- **Live checks are rare and small**: a throwaway repo in the spikes folder, the cheapest model that can do the job, and a `--max-ai-credits` cap (30 at least, the CLI's minimum). The PR records each live check's cost. On a Mac, a live check's `copilot` keeps the user's `HOME`, whose login keychain holds its login, and gets a throwaway `COPILOT_HOME`; its tmux server and the commands sent to it run in a clean environment (`env -i`), since a pane takes the `PATH` of the client that makes it (fact 30 in the plan).

## Quality gates

Measure everything we can with lightweight static tools. Thresholds change only with the user's OK. Every suppression (`//nolint`, a Sonar "won't fix") needs a reason next to it and the user's OK.

| Measure | Limit | Tool |
| --- | --- | --- |
| File length | ≤ 200 lines, not counting blank lines and comments | revive `file-length-limit` |
| Function length | ≤ 50 lines and ≤ 30 statements; tests exempt | `funlen` |
| Cognitive complexity | ≤ 15 per function | `gocognit` |
| Cyclomatic complexity | ≤ 10 per function | `gocyclo` |
| Nesting | ≤ 3 levels of control flow | revive `max-control-nesting` |
| Parameters, results | ≤ 4 parameters, ≤ 3 results | revive `argument-limit`, `function-result-limit` |
| Duplication | no clone of 100 tokens or more; tests exempt | `dupl` |
| Test coverage | ≥ 80% total, ≥ 70% per package; `main` package wiring excluded | go-test-coverage |
| Security | no findings | `gosec` in golangci-lint |
| Vulnerabilities | none known | `govulncheck` |
| Formatting | no diff | gofumpt, golines |
| Suppressions | every `//nolint` names its linters and every `//revive:disable` its rules, each with a reason; `#nosec` is ignored; every `coverage-ignore` gives a reason | `nolintlint`, revive directives, gosec, go-test-coverage |
| Package names | no `util`, `common`, `helpers`, `misc` or similar names (`api` is allowed), and no clash with a common standard-library package | revive `package-naming` |
| GitHub workflows | no findings | jactionlint (with shellcheck for `run:` scripts) |
| Dependencies | current | Renovate (GitHub App) |
| New code overall | quality gate passes | SonarQube Cloud |

The local gates run through `mise run check`. CI runs the same task and, in a parallel job, SonarQube Cloud.

### SonarQube Cloud

- It runs only in CI, on every PR and every push to `main`, and fails the build when the quality gate is red (`-Dsonar.qualitygate.wait=true`). Its verdict is final: `mise run check` is necessary but not sufficient.
- Never run a scanner locally. Scanning an uncommitted tree publishes it as a `main` analysis and corrupts the new-code baseline.
- Fix a red gate. Never explain it away, and never widen the exclusions in `sonar-project.properties` to turn it green.
- `mise run sonar-issues [pr]` lists the open issues, on `main` or on a pull request. Security hotspots show only in the SonarQube Cloud UI, because their API is deprecated.

## Communication

- With the user: terse (caveman style). Keep the review load small: what changed, where and why, in a few lines.
- The user may be away from the Mac: then every question goes through nudge, with two to four options and the lead's pick first, and each question stands on its own.
- Code, docs, commits and PRs: plain, normal prose.
- Docs live in `docs/`; diagrams use Mermaid. Refer to issues in other repos as `owner/repo#123`.
