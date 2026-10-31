# baton work plan

Version 1.0, 2026-10-31.

baton hands a ticket to a team of Copilot CLI agents in tmux. The user talks to one agent, the **orchestrator**: it reads the ticket, plans the work as features and, once the user approves the plan, starts each feature. A feature gets its own git worktree and its own tmux session with four visible panes, one interactive Copilot CLI session per role: **lead, engineer, QA and reviewer**. They pass the work to each other in a fixed flow that the `baton` binary enforces, and the last handover is the user's: the orchestrator reports, and only the user's word pushes the branch and opens the pull request.

This document is the source of truth for scope and the work breakdown. How we work (rules, pipeline, quality gates) is canonical in [AGENTS.md](../AGENTS.md). Update the status markers as work lands.

**Contents**

0. [Status](#0-status)
1. [Goal and scope](#1-goal-and-scope)
2. [Facts](#2-facts)
3. [Design](#3-design)
4. [Decisions](#4-decisions)
5. [Risks](#5-risks)
6. [Open questions](#6-open-questions)
7. [Sources](#7-sources)

## 0. Status

The first milestone is a walking skeleton (`skeleton`): one feature on a throwaway repo, four panes, one full round of handovers and the final report, all with a stand-in `copilot`. Then a live run, then Jira, the guard and the pull request hosts, then Homebrew.

| Id | Step | Status |
| --- | --- | --- |
| `agent-wake` | Spike: wake-ups in an `--agent` session, a plugin's agent IDs, a plugin's hooks, the usage events (facts 3 to 10) | Done, findings in this plan |
| `agents-md` | AGENTS.md, this plan and the README | Done (#1) |
| `ci-baseline` | CI with mise and jactionlint; Renovate's config; the `main` ruleset and branch deletion on merge (decisions 22 and 23) | In review (#2) |
| `sonar` | SonarQube Cloud in CI, `mise run sonar-issues`, and the Sonar checks in the ruleset (decision 23) | Waits for the user (open question 2) |
| `go-toolchain` | The Go module, golangci-lint with the gates, the coverage gate and `mise run check`, with the first package: `internal/flow`, the roles and who works next (decision 13) | Todo |
| `state` | `internal/state`: a feature's state file, event log and notes under the git common directory, behind a lock (decision 15) | Todo |
| `handover` | `baton handover` and `baton status`, with the role and the feature from the pane's environment | Todo |
| `wait` | `baton wait`: blocks until the role has work, then prints it and records the pickup (decision 12) | Todo |
| `tmux` | `internal/tmux`: sessions, panes, titles and options on a chosen socket, tested on `-L baton-test-<n>`; tmux through mise (fact 11) | Todo |
| `worktree` | `internal/worktree`: the feature's branch and its worktree next to the repo | Todo |
| `plugin` | The baton plugin: the five agents and the hooks, embedded in the binary and written to the cache for `--plugin-dir` (decision 10) | Todo |
| `pane` | `baton pane <role>`: waits for the role's first work, then runs `copilot` with the role's flags (decisions 9 to 11) | Todo |
| `feature-start` | `baton feature start <slug> --brief <file>`: the state, the worktree, the tmux session with four titled panes, and the first handover, to the lead | Todo |
| `skeleton` | The walking skeleton, end to end with the stand-in `copilot` | Todo |
| `orchestrator` | `baton start`: the orchestrator's session for the repo, and its `baton wait` for features that are ready or escalated and for questions | Todo |
| `ask` | `baton ask` and `baton answer`: a role's question goes to the orchestrator, and the answer comes back (decision 18) | Todo |
| `stop-hook` | `baton hook stop`: a role that ends its turn with work waiting, or without a `baton wait`, gets one more turn | Todo |
| `live-run` | A live run on a throwaway repo with a cheap model; the findings in a docs PR | Todo |
| `config` | The config file: the model per role, the round cap, the gate command, plan approval, the pull request host and base branch, per repo (decision 16) | Todo |
| `guard` | `baton hook guard`: role panes can't make outward actions; the orchestrator's pane asks the user for them and can't edit the code (decisions 17 and 19) | Todo |
| `gates` | The engineer's handover runs the repo's gate command and refuses a red one | Todo |
| `jira` | `baton jira <KEY>`: the ticket as text, from Jira's REST API with a read-only token in the Keychain (decision 6) | Todo |
| `deliver-github` | `baton deliver` for GitHub: push the branch and open the pull request with `gh`, the lead's final report as its body | Todo |
| `close` | `baton feature close <slug>`: ends the feature's session; keeps its branch and, unless asked, its worktree | Todo |
| `deliver-bitbucket` | `baton deliver` for Bitbucket Cloud: push the branch and open the pull request with the REST API, a token in the Keychain | Todo |
| `cost` | AI credits per role and per feature in `baton status`, from each session's events (decision 20) | Todo |
| `resume` | A pane whose `copilot` ended starts it again with `--resume`; the features carry on after a restart of the Mac | Todo |
| `homebrew` | A release tag and a formula in `codedogapp/homebrew-tap`; the docs install baton with Homebrew (decision 5) | Todo |

Ideas for later, not planned: a read-only page of the runs, like zeroshot's; approvals through nudge when the user is away (open question 1); other agent CLIs; Linux.

## 1. Goal and scope

**Goal.** The user gives the orchestrator a Jira ticket, or a plain request, in any repo on any of their Macs. The orchestrator plans it, the roles build each feature in its own worktree in the open, and the user gets a final report and decides what happens to the branch.

**In scope**

- One Go binary, `baton`, that needs only git, tmux and Copilot CLI (and `gh` for GitHub pull requests).
- The orchestrator's tmux session per repo, and a tmux session per feature with a pane per role.
- The flow, the round cap and the escalation to the user, enforced by the binary.
- Questions from the roles, answered by the orchestrator or, through it, by the user.
- The guard against outward actions, and plan approval.
- Jira tickets as input; GitHub and Bitbucket Cloud pull requests as output.
- The state on disk, so a run survives a restart; AI credits per role.
- Installation with Homebrew.

**Out of scope**

- Autopilot, headless or `-p` sessions for a role.
- A web page or any UI beyond tmux and the CLI (an idea for later).
- A postmortem after a feature.
- Agent CLIs other than Copilot CLI, and operating systems other than macOS. Linux runs only the tests, in CI.
- Writes to Jira.

## 2. Facts

Checked on 2026-10-31 with Copilot CLI 1.0.105 and tmux 3.8, unless a fact says otherwise. Facts 3 to 10 come from the `agent-wake` spike, which spent 5.27 AI credits with Claude Haiku 4.5.

1. **Flags** (`copilot --help`): `-i <prompt>` starts an interactive session with a first prompt; `--agent <id>`; `--plugin-dir <dir>`; `--session-id <uuid>` sets a new session's ID; `--resume`; `--model`, `--reasoning-effort`; `--allow-all` (tools, paths and URLs), `--allow-tool`, `--deny-tool`; `--available-tools` and `--excluded-tools`; `--no-ask-user`; `--disable-mcp-server`; `--max-ai-credits`; `--usage-output-file`; `--log-dir`. Environment: `COPILOT_HOME` (default `~/.copilot`).
2. **`-p` sessions** exit after one prompt and have no `ask_user` tool. MCP tools are named `<server>-<tool>`.
3. **Plugins.** `--plugin-dir` loads a plugin in the Agent Plugins 1.0 layout (`plugin.json` with `$schema`, agents in `com.github.copilot/agents/`, hooks in `com.github.copilot/hooks/hooks.json`) and in the legacy one (agents in `agents/`). A plugin's agent has the ID `<plugin name>:<agent name>`, for example `baton:lead`. A bare name fails, and the error lists the IDs.
4. **Hooks.** A plugin's hooks fire under `--plugin-dir`, and their commands inherit the pane's environment. `sessionStart` fires with the first prompt (its input has `source` and `initialPrompt`). `preToolUse` gets `toolName` (such as `bash`) and `toolArgs` (such as `command` and `mode`). Under `--allow-all` and `--no-ask-user`, a `preToolUse` answer of `ask` still shows a "Hook permission request" with the reason, the tool and its arguments, and the `notification` hook fires with the reason. An `agentStop` answer of `{"decision":"block","reason":…}` makes the reason the next prompt. Its input has `transcriptPath` and `stop_hook_active`. From the hooks reference: a command hook that fails or exits non-zero denies, but one that times out allows; the default timeout is 30 s; the CLI ends a turn after 8 blocks in a row.
5. **Wake-ups.** An idle interactive session started with `--agent` wakes when a shell it started in the background ends: the CLI shows "Waiting for background shells", the shell's end fires the `notification` hook and starts a turn, and the agent reads the output.
6. **`-i` with `--agent`** submits the prompt at once.
7. **Tools.** The default tool list includes `task`, `run_dynamic_workflow`, `dynamic_workflows_manage`, `read_agent`, `write_agent` and `list_agents`, which run agents the user can't see, plus `skill`, `sql` and the user's own MCP tools. `--available-tools` is an allow-list: the model sees only the listed tools, and an unknown name only prints "Unknown tool name in the tool allowlist". A short list also makes each turn cheaper: the first turn cost 0.91 AI credits with 9 tools against 2.78 with the default 29.
8. **Usage.** A session's events go to `$COPILOT_HOME/session-state/<session id>/events.jsonl`. Each model call adds a `session.usage_record`; each turn adds a `session.usage_checkpoint` whose `data.totalNanoAiu` is the session's total so far; `session.shutdown` has the final total, and `--usage-output-file` writes it as JSON at exit. One AI credit is 10⁹ nano AIU.
9. **Folder trust.** With `--allow-all`, a session in a new folder showed no trust prompt and added nothing to the trusted folders.
10. **Limits and models.** `--max-ai-credits` takes 30 at least, and the session tells the model its remaining credits. The `/model` picker shows no prices; it calls Claude Haiku 4.5, Claude Haiku 5.5 and GPT-6 Luna lightweight.
11. **tmux.** tmux 3.8 runs on the Mac (Homebrew). GitHub's `ubuntu-24.04` runner image doesn't list tmux; mise installs it from `aqua:tmux/tmux-builds`, static builds for Linux and macOS (newest: v3.7c).
12. **Typing into a pane** works (`tmux send-keys -l`, then `Enter`), but it mixes with the user's typing and with an open form in the pane.
13. **Jira.** Jira Cloud's REST API reads an issue at `https://api.atlassian.com/ex/jira/<cloudId>/rest/api/3/issue/<KEY>` with basic auth (email and a scoped API token). Atlassian's `acli` and its remote MCP server are the alternatives; some of the MCP server's tools spend Rovo credits.
14. **Bitbucket Cloud** opens a pull request with `POST /2.0/repositories/{workspace}/{repo}/pullrequests` and a token that has `write:pullrequest`. No Bitbucket CLI is installed.
15. **Lessons from an earlier private attempt** (a tmux launcher with an MCP dispatcher): orchestrators told to "only dispatch" did the work themselves; the `task` tool ran agents nobody could see; bash 3.2 and stacked panes caused trouble.
16. **Lessons from zeroshot** (an open-source meta-harness that the user tried and set aside; read, never used here): the flow, gates and round caps belong in the program, not in prompts; QA and review verdicts carry evidence; a run needs an event log and state on disk; Claude Haiku ignored a clear rejection twice, so the engineer needs a strong model.
17. **macOS.** `sleep` pauses while the Mac sleeps, so waits compare wall-clock times; macOS has no `timeout`. A binary replaced on disk stays stale in the processes that run it.

## 3. Design

```mermaid
flowchart LR
  user((user)) <--> orchestrator
  orchestrator -- brief --> lead
  lead -- step brief --> engineer
  engineer --> qa
  qa -- reject --> engineer
  qa -- approve --> reviewer
  reviewer -- reject --> engineer
  reviewer -- approve --> lead
  lead -- final report --> orchestrator
```

**Commands.** `baton start` (or plain `baton`) opens the orchestrator for the current repo. The orchestrator runs `baton jira <KEY>`, `baton feature start <slug> --brief <file>`, `baton wait`, `baton answer`, `baton deliver <slug>` and `baton feature close <slug>`. A role runs `baton handover`, `baton wait` and `baton ask`. Copilot CLI runs `baton hook guard` and `baton hook stop`. tmux runs `baton pane <role>`. Anyone can run `baton status`.

**The flow** (decision 13). Each feature has one holder, the role whose turn it is. Only the holder hands over, and the binary picks the next role:

- the orchestrator's brief goes to the lead, who writes the step brief for the engineer;
- the engineer hands over to QA;
- QA approves to the reviewer, or rejects to the engineer;
- the reviewer approves to the lead, or rejects to the engineer;
- the lead commits on the feature's branch and hands the final report to the orchestrator, and the feature is ready.

A round is one pass of the engineer. A rejection starts the next round, with the rejecting note attached. The third rejection doesn't go to the engineer: the feature is escalated, and the orchestrator gets it and asks the user. On the user's word, the orchestrator hands it back to the engineer for one more round, or to the lead with a new brief, or closes it. QA and the reviewer must hand over with `--verdict approve|reject`; their note gives the evidence (the commands they ran and what came out, each criterion checked) and, for a rejection, the findings, each marked blocking or non-blocking.

**The final handover is the user's.** When a feature is ready, the orchestrator tells the user its branch, the lead's report and the open findings. The user decides: deliver it (`baton deliver` pushes the branch and opens the pull request), send it back with changes (a new brief to the lead, and the round count starts again), or close it.

**Panes** (decisions 9 to 11). `baton feature start` makes the worktree `../<repo>-<slug>/` on a new branch, and a tmux session `baton-<repo>-<slug>` with four tiled panes titled lead, engineer, qa and reviewer (`pane-border-status top`), set on that session only. Each pane runs `baton pane <role>`, which waits without a model until the role's first work arrives, then runs:

```sh
copilot --agent baton:<role> --plugin-dir <cache>/plugin --model <model> \
  --session-id <uuid> --allow-all --no-ask-user \
  --available-tools bash read_bash stop_bash list_bash view create edit grep glob web_fetch \
  -i "<the role's work: read the note at …>"
```

with `BATON_ROLE` and `BATON_FEATURE` in its environment. baton keeps each role's session ID, so it can find the role's events and resume the session. The orchestrator's pane runs the same way, as `baton:orchestrator`, but keeps `ask_user` (the user talks to it) and the user's nudge tools (`nudge-ask`, `nudge-report`) for when the user is away.

**Handover and wake-up** (decision 12). `baton handover --note <file> [--verdict …]` checks the move, stores the note as `notes/NN-<from>-to-<to>.md`, logs the event and makes the next role the holder. The role then starts `baton wait` in a background shell and ends its turn. `baton wait` polls the feature's state every second until the role has work, prints the work (the note's path, the verdict, the round), records the pickup and exits; its end wakes the role (fact 5). The `agentStop` hook is the safety net: a role that ends its turn with work waiting, or without a running `baton wait`, gets one more turn that tells it what to do, once per stop (`stop_hook_active`). Typing into the pane stays the fallback (fact 12).

**Questions** (decision 18). A role never asks the user. `baton ask "…"` stores the question and waits for its answer; the orchestrator's `baton wait` wakes it, and it answers from the plan with `baton answer <id> "…"`, or asks the user first, in its pane or through nudge.

**Safety** (decisions 9, 17 and 19). The roles run with every permission (`--allow-all`) and no `ask_user`, so nothing waits on a prompt in a pane nobody watches, and with the tool allow-list, so no hidden agent and none of the user's skills or MCP tools. The guard hook (`preToolUse` → `baton hook guard`) denies outward actions in a role's pane: `git push`, pull request and merge commands, `gh api` writes, HTTP writes with `curl`, Jira writes, `baton deliver` and `tmux`. In the orchestrator's pane, it answers `ask` for outward actions, so the user approves each one, and for `baton feature start` while plan approval is on (decision 8); and it denies `edit` and `create` outside baton's state, so the orchestrator plans and dispatches but doesn't write the code. The guard only matches text, quickly, because a hook that times out allows (fact 4).

**State** (decision 15). Everything lives in `$(git rev-parse --git-common-dir)/baton/`, which every worktree of the repo shares and git never commits:

- `features/<slug>/feature.json`: the slug, the branch and worktree, the holder, the round and its cap, the status (`running`, `escalated`, `ready`, `delivered`, `closed`), each role's session ID, and the notes not yet delivered;
- `features/<slug>/events.ndjson`: one JSON line per event (start, handover, pickup, question, answer, escalation, push and pull request), with its time;
- `features/<slug>/notes/`: the briefs, the handover notes and the final report;
- `questions/<id>.json`: open questions and their answers.

Every change takes an exclusive lock on the feature (`flock`), writes a new file and renames it over the old one, so a reader sees the old state or the new one.

**Config** (decision 16). `baton` reads `config.json` in its folder under `os.UserConfigDir()` (`~/Library/Application Support/baton/` on a Mac): defaults, plus a section per repo, keyed by the remote's URL, so nothing goes into a work repo. The settings: the model and reasoning effort per role, an AI credits cap per role, the round cap (3), plan approval (on), the gate command, the branch prefix, the base branch (by default the remote's default branch), the pull request host (by default from the remote's URL), and Jira's site and email. Tokens never go into the file: they live in the login Keychain.

**Jira** (decision 6). `baton jira DEMO-1` prints the ticket's summary, type, status, description (Atlassian Document Format turned into Markdown), subtasks, links and comments. It reads the token from the Keychain with `/usr/bin/security`, and the site's `cloudId` from the site.

**Delivery.** `baton deliver <slug>` pushes the branch and opens the pull request against the base branch, titled from the brief, with the lead's final report as its body: on GitHub with `gh pr create`, on Bitbucket Cloud with its REST API and a token from the Keychain. It never merges.

**Cost** (decision 20). `baton status` sums each role's AI credits from the last `session.usage_checkpoint` in its events (fact 8) and shows them per feature, with the holder, the round and the status.

## 4. Decisions

Decisions marked **lead's call** are the lead's, for the user's end review. The user can change any of them before the code depends on it; the repo is cheap to rename or make private now.

1. **baton is its own tool**: the user's choice (2026-10-31), after a spike of zeroshot. Not built on zeroshot or Omnigent; their ideas are welcome, their code isn't (fact 16).
2. **`codedogapp/baton`, public** (lead's call): like nudge, it holds no secrets and nothing about any work project.
3. **The name `baton`** (lead's call): the roles pass the baton, and the user takes the last handover.
4. **One Go binary** (lead's call): no bash scripts and their bash 3.2 trouble (fact 15); one file to install; the flow in tested code.
5. **Homebrew, `codedogapp/tap/baton`** (lead's call): a formula that builds baton from its tag's source, as nudge's did. The tap's state is to be checked at the `homebrew` step, since nudge left it.
6. **Jira's REST API with a scoped, read-only token** (lead's call): nothing else to install, and no Rovo credits. The token (`read:jira-work`) lives in the login Keychain, which the user fills.
7. **Claude Opus 5.5 for every role** (lead's call), configurable per role. A weaker engineer ignored rejections in zeroshot's spike (fact 16).
8. **Plan approval on** (lead's call), configurable. The guard makes the user approve each `baton feature start` in the orchestrator's pane, so an orchestrator can't skip it.
9. **The roles run with `--allow-all` and `--no-ask-user`, guarded by a hook** (lead's call, flagged): the CLI's own prompts would stop a role in a pane nobody watches, and a role can't approve its own tools. The guard keeps outward actions out of the role panes (fact 4), and the orchestrator, whom the user watches, keeps `ask_user`.
10. **A tool allow-list per role (`--available-tools`)** (lead's call): it removes `task` and every other tool that runs hidden agents, the user's skills and MCP tools, and it fails closed when a new CLI version adds such a tool (fact 7). An exclude list would fail open.
11. **The agents and hooks ship as a plugin** (lead's call): the binary embeds the plugin, writes it to its cache and passes it with `--plugin-dir`, so `~/.copilot/` stays untouched and the agents are `baton:<role>` (fact 3), clear of the user's own agents.
12. **Wake-ups through `baton wait`** (lead's call): a background shell's end wakes the role (fact 5), and nobody types into a pane. The first work starts the pane's `copilot` (`baton pane`), so a role spends nothing before it has work. The stop hook catches a role that forgets to wait; typing into the pane is only the fallback.
13. **The binary owns the flow** (lead's call): the next role, the round count, the cap of three rounds and the escalation to the user live in `internal/flow`, with tests. After an escalation, the user's word buys one more round at a time.
14. **A session per feature, on the user's tmux server** (lead's call): `baton-<repo>-<slug>`, next to the orchestrator's `baton-<repo>`. baton changes only the sessions it made, and only their own options. Tests and live checks use their own sockets.
15. **The state lives under the git common directory** (lead's call): shared by the repo's worktrees, never committed, no file in the work repo; a lock and atomic renames keep it whole.
16. **The config is JSON** (lead's call), where the proposal had TOML: Go's standard library reads JSON, and nudge's settings file is JSON too. It lives outside the repo, keyed by the remote's URL.
17. **Outward actions only through `baton deliver`, in the orchestrator's pane, on the user's approval** (lead's call). The roles never push; the lead commits locally.
18. **Roles ask the orchestrator, never the user** (the user's rule): `baton ask` blocks the role until `baton answer`.
19. **The orchestrator can't write the code** (lead's call): the guard denies its `edit` and `create` outside baton's state (fact 15).
20. **Cost from the session's own events** (lead's call): the last `session.usage_checkpoint` per role's session (fact 8), read while the feature runs; no extra flags.
21. **Waits poll every second and compare wall-clock times** (lead's call): simple, and right after the Mac sleeps (fact 17). No file-watching dependency.
22. **Branches are deleted on merge** (lead's call): like Bakhyard, unlike nudge, as the user asked when baton started. The `main` ruleset is nudge's: no deletion, no force push, changes only through pull requests (no approvals needed), and the required checks; the wiki is off, as in nudge.
23. **SonarQube Cloud joins CI in its own step** (lead's call): it needs a project and a `SONAR_TOKEN` secret that only the user can make (nudge's token is a secret of nudge's repo, not of the organisation). Until then, CI runs `mise run check` alone, the ruleset requires `check` alone, and the definition of done's SonarQube Cloud part waits. No job that skips itself without the token: a gate that can turn itself off would hide a lost secret later.

## 5. Risks

1. **Wake-ups aren't a documented contract** (fact 5). A CLI update could break them. The stop hook and typing into the pane are the fallbacks, and `live-run` checks the whole loop.
2. **The guard matches text** (decision 9): a role can reach the network in ways no pattern catches (a script, another tool). It is a seatbelt, not a sandbox. Taking the role panes' credentials away (an empty `GH_CONFIG_DIR`, a git `pushInsteadOf` that fails) would add a second belt; `guard` checks whether that works.
3. **`--allow-all` lets a role read and write any path** on the Mac. Roles work in their worktree by their instructions, and the guard watches outward actions only.
4. **A hook that times out allows** (fact 4): the guard must answer within its timeout and never wait on anything.
5. **AI credits**: four Opus sessions per feature, three of them in every round, plus the orchestrator. The per-role cap (`--max-ai-credits`, decision 16) and `baton status` keep it visible; the round cap bounds it.
6. **The user's own instructions apply in every pane**: Copilot CLI loads the user's and the repo's custom instructions, which may disagree with a role's prompt.
7. **The CLI changes fast**: flags, tool names and event fields can change. The facts carry the version they were checked with; `live-run` repeats the checks.
8. **Processes outlive an upgrade** (fact 17): a running `baton wait` or `baton pane` keeps the old binary, so the state's format carries a version and stays readable.
9. **Floating tool versions**: mise tools use `latest` with no lockfile, so a new release (a new jactionlint rule, say) can turn CI red with no change in the repo, and a local run may use an older version than CI until `mise upgrade --local`. When CI turns red for no reason, check the tool versions first.

## 6. Open questions

1. **Approvals when the user is away**: the guard's `ask` shows only in the orchestrator's pane. Should `baton deliver` also ask through nudge, so the user can approve from the phone? Until decided, outward actions wait for the user at the Mac.
2. **Things only the user can do**: add the repo to the Renovate GitHub App; make the SonarQube Cloud project, with automatic analysis off, and the repo's `SONAR_TOKEN` secret (asked through nudge on 2026-10-31, for step `sonar`); the Jira token and site (`jira`); the Bitbucket token and workspace (`deliver-bitbucket`).

## 7. Sources

- Copilot CLI: `copilot --help` and `copilot help config|environment|permissions` (1.0.105); the hooks reference, the plugins overview and the CLI plugin reference on docs.github.com; the `agent-wake` spike (facts 3 to 10).
- tmux 3.8 manual; [tmux/tmux-builds](https://github.com/tmux/tmux-builds); GitHub's [runner images](https://github.com/actions/runner-images).
- Jira Cloud REST API v3 and Bitbucket Cloud REST API 2.0 reference.
- Bakhyard's AGENTS.md (the pipeline baton runs), nudge (this repo's rules, CI and gates), zeroshot (ideas only, fact 16).
