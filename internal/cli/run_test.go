package cli_test

import "testing"

// usage is baton's usage, as it lists the commands.
const usage = `Usage: baton <command> [flags]

Commands:
  handover  hand the feature over to the next role, with a note
  pane      wait for the role's first work, then become the role's copilot
  status    list the repo's features, with their holder, round and status
  wait      wait for the role's next work, then pick it up and print it

Run baton <command> -h for the command's flags.
`

// handoverUsage is the usage of baton handover, as it lists the flags.
const handoverUsage = `Usage of baton handover:
  -note file
    	the file with the note for the next role (required)
  -verdict verdict
    	QA's or the reviewer's verdict: approve or reject
`

func TestRunReportsTheUsage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want result
	}{
		{"without a command", nil, result{code: 2, stderr: usage}},
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
			[]string{"handover", "--to", "qa"},
			result{code: 2, stderr: "flag provided but not defined: -to\n" + handoverUsage},
		},
		{"with status's -h", []string{"status", "-h"}, result{stderr: "Usage of baton status:\n"}},
		{"with wait's -h", []string{"wait", "-h"}, result{stderr: "Usage of baton wait:\n"}},
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
