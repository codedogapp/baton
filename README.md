# baton

Hand a ticket to a team of Copilot CLI agents in tmux: lead, engineer, QA and reviewer pass the baton, and you get the final handover.

You talk to one agent, the orchestrator. It reads the ticket, plans the work as features and, once you approve the plan, starts each feature in its own git worktree and its own tmux session, with one visible, interactive Copilot CLI pane per role. The `baton` binary enforces the flow between the roles: who works next, that your repo's checks pass, the rounds of QA and review, and when to stop and ask you. Nothing leaves your Mac until you say so: the last handover is yours, and only your word pushes the branch and opens the pull request.

**Status**: in development; nothing to install yet. The design and the steps are in [docs/plan.md](docs/plan.md), and the rules for working on baton in [AGENTS.md](AGENTS.md).

## Requirements

- macOS, git 2.36 or newer, and tmux 3.5 or newer.
- [GitHub Copilot CLI](https://docs.github.com/copilot/how-tos/copilot-cli), logged in with `copilot login`, or with a token in `COPILOT_GITHUB_TOKEN`, `GH_TOKEN` or `GITHUB_TOKEN`: `gh` doesn't work in the roles' panes, so their `copilot` can't take its login from `gh`.
- `gh` for GitHub pull requests; a Bitbucket Cloud API token for Bitbucket pull requests; a read-only Jira API token for Jira tickets.

## Config

baton needs a config file, `config.json` in `~/Library/Application Support/baton/`, for one setting: the gate that checks each repo's code. Its top holds the settings for every repo, and `repos` a section per repo, keyed by the host and path of the repo's `origin` remote, such as `github.com/codedogapp/baton` for `git@github.com:codedogapp/baton.git`. A repo's section lies over the top, one setting at a time.

- `roles`: a section per role, `orchestrator`, `lead`, `engineer`, `qa` or `reviewer`, with its `model` (by default `claude-opus-5.5`), its `reasoningEffort` (`none`, `minimal`, `low`, `medium`, `high`, `xhigh` or `max`; by default copilot's own), its `maxAiCredits` (30 or more; by default no cap) and, for the orchestrator alone, its `extraTools`: your own tools that it may use, by default nudge's `nudge-ask` and `nudge-report`.
- `roundCap`: the engineer's passes per brief before the orchestrator asks you (by default 3).
- `branchPrefix`: what goes before a feature's slug in its branch's name (by default `feature/`).
- `planApproval`: whether baton asks you before each feature starts, so that you can check the orchestrator's plan first (by default `true`).
- `gate`: the command that checks the repo's code, such as `mise run check`, with no default: baton starts neither the orchestrator nor a feature in a repo without one. The engineer's handover, QA's approval and the lead's final report run it with `/bin/sh -c` in the feature's worktree, and wait until it passes. `"true"` checks nothing.
- `jira`: your Jira Cloud site's host name and your email address at Atlassian, for `baton jira`, such as `{"site": "example.atlassian.net", "email": "you@example.com"}`, with no default.

For example, one gate and QA on another model with a cap, for every repo, and two rounds with another branch prefix for one repo:

```json
{
  "gate": "mise run check",
  "roles": {"qa": {"model": "claude-sonnet-5.5", "maxAiCredits": 60}},
  "repos": {"github.com/codedogapp/baton": {"roundCap": 2, "branchPrefix": "feat/"}}
}
```

baton checks the whole file each time it reads it, and refuses a file with a mistake, naming the file and the mistake.

## Jira

When you name a Jira Cloud ticket, the orchestrator reads it with `baton jira <KEY>`, such as `baton jira DEMO-1`, which prints the ticket and its comments as Markdown; you can run it too. baton only reads Jira, as you, with an API token that can only read:

1. Set `jira` in the config file to your site and your email address at Atlassian.
2. At [id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens), create an API token with scopes, for Jira, with the scope `read:jira-work` alone.
3. Put it in your login Keychain, as baton's item for your email: `security add-generic-password -s baton-jira -a you@example.com -w`, which asks for the token, so that it stays out of your shell's history. When the token expires, put the new one in its place with the same command and `-U`, which `baton jira` gives when Jira refuses your login.

baton sends the token to Atlassian's gateway, `api.atlassian.com`, alone, and never prints it. Any program that runs as you can read the item, though, the roles' commands included, so give the token no other scope.
