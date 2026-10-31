# baton

Hand a ticket to a team of Copilot CLI agents in tmux: lead, engineer, QA and reviewer pass the baton, and you get the final handover.

You talk to one agent, the orchestrator. It reads the ticket, plans the work as features and, once you approve the plan, starts each feature in its own git worktree and its own tmux session, with one visible, interactive Copilot CLI pane per role. The `baton` binary enforces the flow between the roles: who works next, the rounds of QA and review, and when to stop and ask you. Nothing leaves your Mac until you say so: the last handover is yours, and only your word pushes the branch and opens the pull request.

**Status**: in development; nothing to install yet. The design and the steps are in [docs/plan.md](docs/plan.md), and the rules for working on baton in [AGENTS.md](AGENTS.md).

## Requirements

- macOS, git 2.31 or newer, and tmux 3.5 or newer.
- [GitHub Copilot CLI](https://docs.github.com/copilot/how-tos/copilot-cli).
- `gh` for GitHub pull requests; a Bitbucket Cloud API token for Bitbucket pull requests; a read-only Jira API token for Jira tickets.
