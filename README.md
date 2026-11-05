# baton

Hand a ticket to a team of Copilot CLI agents in tmux: lead, engineer, QA and reviewer pass the baton, and you get the final handover.

You talk to one agent, the orchestrator. It reads the ticket, plans the work as features and, once you approve the plan, starts each feature in its own git worktree and its own tmux session, with one visible, interactive Copilot CLI pane per role. The `baton` binary enforces the flow between the roles: who works next, that your repo's checks pass, the rounds of QA and review, and when to stop and ask you. Nothing leaves your Mac until you say so: the last handover is yours, and only your word pushes the branch and opens the pull request.

**Status**: in development. The design and the steps are in [docs/plan.md](docs/plan.md), and the rules for working on baton in [AGENTS.md](AGENTS.md).

## Install

Each [release](https://github.com/codedogapp/baton/releases) holds baton for macOS on Apple silicon. Install the latest with [mise](https://mise.jdx.dev):

```sh
mise use -g github:codedogapp/baton
```

mise checks the archive against the digest that GitHub gives for it and against its attestation, which shows that this repo's release workflow built it from the release's tag. As for every tool, mise holds a release back for its first 24 hours (its `minimum_release_age`); to install one sooner, name its version, as in `mise use -g github:codedogapp/baton@0.1.0`. Apple hasn't notarized baton, so macOS blocks a copy that a browser downloaded until you allow it under Privacy & Security in System Settings; mise's copy runs as it is.

## Requirements

- macOS, git 2.36 or newer, and tmux 3.5 or newer.
- [GitHub Copilot CLI](https://docs.github.com/copilot/how-tos/copilot-cli), logged in with `copilot login`, or with a token in `COPILOT_GITHUB_TOKEN`, `GH_TOKEN` or `GITHUB_TOKEN`: `gh` doesn't work in the roles' panes, so their `copilot` can't take its login from `gh`.
- `gh`, logged in with `gh auth login`, for GitHub pull requests; a Bitbucket Cloud API token for Bitbucket pull requests; a read-only Jira API token for Jira tickets.

## Config

baton needs a config file, `config.json` in `~/Library/Application Support/baton/`, for one setting: the gate that checks each repo's code. Its top holds the settings for every repo, and `repos` a section per repo, keyed by the host and path of the repo's `origin` remote, such as `github.com/codedogapp/baton` for `git@github.com:codedogapp/baton.git`. A repo's section lies over the top, one setting at a time.

- `roles`: a section per role, `orchestrator`, `lead`, `engineer`, `qa` or `reviewer`, with its `model` (by default `claude-opus-5.5`), its `reasoningEffort` (`none`, `minimal`, `low`, `medium`, `high`, `xhigh` or `max`; by default copilot's own), its `maxAiCredits` (30 or more; by default no cap) and, for the orchestrator alone, its `extraTools`: your own tools that it may use, by default nudge's `nudge-ask` and `nudge-report`.
- `roundCap`: the engineer's passes per brief before the orchestrator asks you (by default 3).
- `branchPrefix`: what goes before a feature's slug in its branch's name (by default `feature/`).
- `planApproval`: whether baton asks you before each feature starts, so that you can check the orchestrator's plan first (by default `true`).
- `gate`: the command that checks the repo's code, such as `mise run check`, with no default: baton starts neither the orchestrator nor a feature in a repo without one. The engineer's handover, QA's approval and the lead's final report run it with `/bin/sh -c` in the feature's worktree, and wait until it passes. `"true"` checks nothing.
- `jira`: your Jira Cloud site's host name and your email address at Atlassian, for `baton jira`, such as `{"site": "example.atlassian.net", "email": "you@example.com"}`, with no default.
- `bitbucket`: your email address at Atlassian, for the pull requests that baton opens on Bitbucket Cloud, such as `{"email": "you@example.com"}`, with no default.

For example, one gate and QA on another model with a cap, for every repo, and two rounds with another branch prefix for one repo:

```json
{
  "gate": "mise run check",
  "roles": {"qa": {"model": "claude-sonnet-5.5", "maxAiCredits": 60}},
  "repos": {"github.com/codedogapp/baton": {"roundCap": 2, "branchPrefix": "feat/"}}
}
```

baton checks the whole file each time it reads it, and refuses a file with a mistake, naming the file and the mistake.

## Pull requests

When a feature is ready, the orchestrator reports it to you, with a title for its pull request. On your word, it delivers the feature with `baton deliver <slug> --title "<title>"`, which baton's guard asks you about first; you can run it too. baton pushes the feature's branch to `origin`, with your git, then opens its pull request into the repo's default branch, with your `gh` on GitHub, or as below on Bitbucket Cloud, with the lead's final report as its description, as it is: read the report first, at the path that the orchestrator gives, if you want to see what goes out. If you hand the feature back for changes, deliver it again once it's ready, without `--title`: baton pushes the new commits and comments the lead's new report on the same pull request. baton never merges.

gh picks the GitHub repo from the repo's remotes, as `gh pr create` does, so baton doesn't handle a fork with an `upstream` remote yet.

### Bitbucket Cloud

For a repo whose `origin` remote names `bitbucket.org` in its URL, such as `git@bitbucket.org:acme/demo.git`, baton opens the pull request, and comments on it, with Bitbucket's API instead, as you. It needs your email and an API token, which it checks for before it pushes anything:

1. Set `bitbucket` in the config file to your email address at Atlassian.
2. At [id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens), create an API token with scopes, for Bitbucket, with the scopes `read:pullrequest:bitbucket` and `write:pullrequest:bitbucket` alone, kept to your workspace.
3. Put it in your login Keychain, as baton's item for your email: `security add-generic-password -s baton-bitbucket -a you@example.com -w`, which asks for the token, so that it stays out of your shell's history. When the token expires, put the new one in its place with the same command and `-U`, which `baton deliver` gives when Bitbucket refuses your login.

baton sends the token to `api.bitbucket.org` alone, and never prints it; git pushes the branch with your own login, as for GitHub. The token can also approve, decline and merge pull requests wherever you can, and any program that runs as you can read the item, the roles' commands included, so give it a short expiry.

baton finds Bitbucket by `bitbucket.org` in `origin`'s URL alone: a remote with an SSH alias from `~/.ssh/config` in its place, such as `bb:acme/demo.git`, is a GitHub repo to baton, and gh fails after the push. Name `bitbucket.org` in `origin`'s URL instead.

## AI credits

`baton status` lists each feature with its holder, round and status, and the AI credits that its roles have spent so far, in all and for each role, from the events that each role's `copilot` logs in its home folder (`COPILOT_HOME`, or `~/.copilot`). A `-` stands for a role whose `copilot` hasn't started yet, or whose events baton can't find, such as under another `COPILOT_HOME`. The orchestrator's own credits aren't counted, since it serves every feature.

## When a pane ends

When a role's `copilot` ends, its pane stays, with what it printed, until `baton start`, in your own shell in the repo, starts it again: the role carries on where it was, in its session. After a restart of your Mac, `baton start` makes the tmux sessions of the open features again too, all but the closed ones, before it shows you the orchestrator. If it can't start a feature's team again, as when the feature's worktree is gone, it says which feature and why, and doesn't show you the orchestrator: close the feature with `baton feature close <slug>`, then run `baton start` again. tmux's own `respawn-pane` starts a role's pane without the role's settings, so the pane fails; use `baton start`. The orchestrator's tmux session ends when its `copilot` does, and `baton start` opens it again, in its last session.

## Closing a feature

When you're done with a feature, delivered or not, tell the orchestrator to close it, which baton's guard asks you about first, or run `baton feature close <slug>` yourself. baton ends the feature's tmux session, with its team's panes, even mid-work, and keeps its branch and its worktree: add `--remove-worktree` to remove the worktree too, which git refuses while it holds changes that no commit has, and which takes the files that git ignores with it. The closed feature stays in `baton status`, and nothing moves it again. Delete its branch yourself once you no longer need it.

## Jira

When you name a Jira Cloud ticket, the orchestrator reads it with `baton jira <KEY>`, such as `baton jira DEMO-1`, which prints the ticket and its comments as Markdown; you can run it too. baton only reads Jira, as you, with an API token that can only read:

1. Set `jira` in the config file to your site and your email address at Atlassian.
2. At [id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens), create an API token with scopes, for Jira, with the scope `read:jira-work` alone.
3. Put it in your login Keychain, as baton's item for your email: `security add-generic-password -s baton-jira -a you@example.com -w`, which asks for the token, so that it stays out of your shell's history. When the token expires, put the new one in its place with the same command and `-U`, which `baton jira` gives when Jira refuses your login.

baton sends the token to Atlassian's gateway, `api.atlassian.com`, alone, and never prints it. Any program that runs as you can read the item, though, the roles' commands included, so give the token no other scope.

## License

baton is under the [MIT license](LICENSE). Each release's archive holds it, and in `THIRD_PARTY_LICENSES` the licenses of the code built into baton: Go's and mvdan.cc/sh's, both BSD-3-Clause.
