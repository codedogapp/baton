package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/codedogapp/baton/internal/guard"
)

// featureStart is the guard's name for baton feature start, which the user approves while plan
// approval is on (decision 54).
const featureStart = "baton feature start"

// orchestratorCommands are baton's commands that the orchestrator runs unasked, by the names
// that the guard gives them.
var orchestratorCommands = slices.Concat(
	[]string{"baton answer", "baton handover", "baton status", "baton wait"}, helpCommands)

// orchestratorWhy says why the guard denies the orchestrator a command of each kind that it
// denies, and what the orchestrator does instead.
var orchestratorWhy = map[guard.Kind]string{
	guard.Control: "drives the user's tmux panes or the Mac's apps, which the orchestrator " +
		"never does: baton runs the panes, and `baton status` shows the features.",
	guard.Agent: "starts another agent, which the orchestrator never does: start a feature, " +
		"and its team does the work.",
	guard.Baton: "isn't the orchestrator's command: it runs `baton feature start`, " +
		"`baton wait`, `baton answer`, `baton handover` and `baton status`.",
	guard.Unknown: why[guard.Unknown],
}

// guardAsks starts each question that baton hook guard asks the user in the orchestrator's pane,
// which copilot shows the user (decision 54).
const guardAsks = "baton's guard asks you: "

// orchestratorRules are the guard's rules in the orchestrator's pane, which the user watches
// (decision 54). It runs baton's commands for the orchestrator, asks the user before each command
// that acts outside the Mac and, while plan approval is on, before each baton feature start, and
// denies the other commands that it watches. It denies a write outside the copilot's session
// folder, which the input's session names.
func orchestratorRules(ctx context.Context, env Env, dir, session string) paneRules {
	return paneRules{
		script: func(commands []guard.Command) (guardOutput, error) {
			approval, err := planApproval(ctx, dir, commands)
			if err != nil {
				return guardOutput{}, err
			}

			return firstAnswer(commands, func(command guard.Command) guardOutput {
				return orchestratorCommand(command, approval)
			}), nil
		},
		write: func(path string) (string, error) {
			folder, err := sessionFolder(env, session)
			if err != nil || within(path, folder) {
				return "", err
			}

			return fmt.Sprintf("%s%s is outside your session's folder %s: the team writes the "+
				"repo's files, and you write your briefs and notes in its files/ folder.", stopped,
				path, folder), nil
		},
	}
}

// planApproval reports whether the user approves each baton feature start, by the settings of
// the folder's repo, which it loads only for a script that starts a feature.
func planApproval(ctx context.Context, dir string, commands []guard.Command) (bool, error) {
	if !slices.ContainsFunc(commands, func(command guard.Command) bool {
		return command.Name == featureStart
	}) {
		return false, nil
	}

	settings, err := loadSettings(ctx, dir)

	return settings.PlanApprovalOn(), err
}

// orchestratorCommand returns the guard's answer to a command that it watches in the
// orchestrator's pane, where approval says whether the user approves each feature start.
func orchestratorCommand(command guard.Command, approval bool) guardOutput {
	switch {
	case slices.Contains(orchestratorCommands, command.Name):
		return guardOutput{}
	case command.Name == featureStart && approval:
		return guardOutput{"ask", guardAsks + "`baton feature start` starts a feature's team on " +
			"its brief. Allow it if you approved the plan; planApproval in baton's config file " +
			"turns this question off."}
	case command.Name == featureStart:
		return guardOutput{}
	case command.Kind == guard.Outward:
		return guardOutput{"ask", fmt.Sprintf("%s`%s` can act outside the Mac. Allow it only "+
			"if you want it to.", guardAsks, command.Name)}
	}

	return deny(fmt.Sprintf("%s`%s` %s", stopped, command.Name, orchestratorWhy[command.Kind]))
}

// sessionFolder returns the copilot session's folder, in which the orchestrator writes its briefs
// and notes: session-state/<session> in copilot's home folder, which COPILOT_HOME names in the
// pane, or else .copilot in the user's home folder.
func sessionFolder(env Env, session string) (string, error) {
	// filepath.Base gives "." for "", so an empty ID fails too.
	if session == "." || session == ".." || session != filepath.Base(session) {
		return "", fmt.Errorf("read the hook's input: sessionId %q: want a session's ID", session)
	}

	home := env.Getenv("COPILOT_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find the home folder: %w", err)
		}

		home = filepath.Join(user, ".copilot")
	}

	return filepath.Join(home, "session-state", session), nil
}
