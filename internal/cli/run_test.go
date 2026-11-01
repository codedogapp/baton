package cli_test

import "testing"

// usage is baton's usage, as it lists the commands.
const usage = `Usage: baton [<command>] [flags]

Commands:
  start          open the repo's orchestrator in its tmux session, as plain baton does
  answer         answer the question of a feature's role, in the orchestrator's pane
  ask            ask the orchestrator a question, in a role's pane, then wait for the answer
  feature start  start a feature from a brief: its branch, its worktree and its tmux session
  handover       hand a feature over to the next role, with a note
  hook stop      copilot's stop hook: send a pane's copilot that stops too soon back to work
  pane           become the pane's copilot: the orchestrator's, or the role's once it has work
  status         list the repo's features, with their holder, round and status
  wait           wait for the role's next work, then pick it up and print it

Run baton <command> -h for the command's flags.
`

// startUsage is the usage of baton feature start, as it lists the flags.
const startUsage = `Usage of baton feature start:
  -brief file
    	the file with the brief for the lead (required)
`

// handoverUsage is the usage of baton handover, as it lists the flags.
const handoverUsage = `Usage of baton handover:
  -note file
    	the file with the note for the next role (required)
  -to role
    	the orchestrator's pick of the next role: lead or engineer
  -verdict verdict
    	QA's or the reviewer's verdict: approve or reject
`

func TestRunReportsTheUsage(t *testing.T) {
	wantStart := "baton feature: want start\n\n" + usage
	wantStop := "baton hook: want stop\n\n" + usage
	cases := []struct {
		name string
		args []string
		want result
	}{
		{"with help", []string{"help"}, result{stdout: usage}},
		{"with -h", []string{"-h"}, result{stdout: usage}},
		{"with -help", []string{"-help"}, result{stdout: usage}},
		{"with --help", []string{"--help"}, result{stdout: usage}},
		{
			"with an unknown command",
			[]string{"hand-over", "--note", "note.md"},
			result{code: 2, stderr: "baton: unknown command \"hand-over\"\n\n" + usage},
		},
		{"with a command's -h", []string{"handover", "-h"}, result{stderr: handoverUsage}},
		{
			"with a command's unknown flag",
			[]string{"handover", "demo-1", "--next", "qa"},
			result{code: 2, stderr: "flag provided but not defined: -next\n" + handoverUsage},
		},
		{"with feature alone", []string{"feature"}, result{code: 2, stderr: wantStart}},
		{
			"with feature's unknown command",
			[]string{"feature", "end"},
			result{code: 2, stderr: wantStart},
		},
		{"with feature start's -h", []string{"feature", "start", "-h"}, result{stderr: startUsage}},
		{"with hook alone", []string{"hook"}, result{code: 2, stderr: wantStop}},
		{
			"with hook's unknown command",
			[]string{"hook", "start"},
			result{code: 2, stderr: wantStop},
		},
		{
			"with hook stop's -h",
			[]string{"hook", "stop", "-h"},
			result{stderr: "Usage of baton hook stop:\n"},
		},
		{
			"with hook stop's unknown flag",
			[]string{"hook", "stop", "--role", "qa"},
			result{
				code:   2,
				stderr: "flag provided but not defined: -role\nUsage of baton hook stop:\n",
			},
		},
		{
			"with feature start's unknown flag before the slug",
			[]string{"feature", "start", "--note", "brief.md", "demo-1"},
			result{code: 2, stderr: "flag provided but not defined: -note\n" + startUsage},
		},
		{
			"with feature start's unknown flag after the slug",
			[]string{"feature", "start", "demo-1", "--note", "brief.md"},
			result{code: 2, stderr: "flag provided but not defined: -note\n" + startUsage},
		},
		{"with status's -h", []string{"status", "-h"}, result{stderr: "Usage of baton status:\n"}},
		{"with start's -h", []string{"start", "-h"}, result{stderr: "Usage of baton start:\n"}},
		{
			"with start's unknown flag",
			[]string{"start", "--attach"},
			result{
				code:   2,
				stderr: "flag provided but not defined: -attach\nUsage of baton start:\n",
			},
		},
		{"with wait's -h", []string{"wait", "-h"}, result{stderr: "Usage of baton wait:\n"}},
		{"with ask's -h", []string{"ask", "-h"}, result{stderr: "Usage of baton ask:\n"}},
		{"with answer's -h", []string{"answer", "-h"}, result{stderr: "Usage of baton answer:\n"}},
		{
			"with ask's unknown flag",
			[]string{"ask", "--to", "orchestrator", "Which file?"},
			result{
				code:   2,
				stderr: "flag provided but not defined: -to\nUsage of baton ask:\n",
			},
		},
		{
			"with answer's unknown flag",
			[]string{"answer", "--to", "lead", "demo-1", "demo.txt"},
			result{
				code:   2,
				stderr: "flag provided but not defined: -to\nUsage of baton answer:\n",
			},
		},
		{"with pane's -h", []string{"pane", "-h"}, result{stderr: "Usage of baton pane:\n"}},
		{
			"with status's unknown flag",
			[]string{"status", "--all"},
			result{
				code:   2,
				stderr: "flag provided but not defined: -all\nUsage of baton status:\n",
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := run(t, t.TempDir(), nil, test.args...); got != test.want {
				t.Errorf("got %+v; want %+v", got, test.want)
			}
		})
	}
}
