package guard_test

import (
	"slices"
	"testing"

	"github.com/codedogapp/baton/internal/guard"
)

// kinds are the words that the tests write for each kind.
var kinds = map[guard.Kind]string{
	guard.Outward: "outward",
	guard.Control: "control",
	guard.Agent:   "agent",
	guard.Baton:   "baton",
	guard.Unknown: "unknown",
}

// wantCommands checks the watched commands that each script runs, each written as its kind and
// name, such as "outward: git push".
func wantCommands(t *testing.T, cases map[string][]string) {
	t.Helper()

	for script, want := range cases {
		t.Run(script, func(t *testing.T) {
			found, err := guard.Commands(script)
			if err != nil {
				t.Fatalf("Commands(%q) says %v", script, err)
			}

			var got []string
			for _, command := range found {
				got = append(got, kinds[command.Kind]+": "+command.Name)
			}

			if !slices.Equal(got, want) {
				t.Errorf("Commands(%q) = %q, want %q", script, got, want)
			}
		})
	}
}

func TestCommandsFindsGitsCommandsThatSendToAnotherHost(t *testing.T) {
	wantCommands(t, map[string][]string{
		"git push":                                            {"outward: git push"},
		"git push --force origin :old":                        {"outward: git push"},
		"git send-email 0001.patch":                           {"outward: git send-email"},
		"git send-pack host:repo main":                        {"outward: git send-pack"},
		"git http-push https://h/r main":                      {"outward: git http-push"},
		"git imap-send < 0001.patch":                          {"outward: git imap-send"},
		"git -C x push":                                       {"outward: git push"},
		"git -c x push":                                       {"outward: git push"},
		"git --attr-source x push":                            {"outward: git push"},
		"git --config-env x push":                             {"outward: git push"},
		"git --git-dir x push":                                {"outward: git push"},
		"git --namespace x push":                              {"outward: git push"},
		"git --shallow-file x push":                           {"outward: git push"},
		"git --super-prefix x push":                           {"outward: git push"},
		"git --work-tree x push":                              {"outward: git push"},
		"git --git-dir=.git -p --bare push":                   {"outward: git push"},
		"git -- push":                                         {"outward: git push"},
		"GIT push":                                            {"outward: git push"},
		"/opt/homebrew/bin/git push":                          {"outward: git push"},
		"git-push origin main":                                {"outward: git push"},
		"/usr/libexec/git-core/git-send-email":                {"outward: git send-email"},
		`git "$command" origin`:                               {"outward: git …"},
		`git pu$sh`:                                           {"outward: git pu…"},
		`git $options push`:                                   {"outward: git …"},
		`git -$letter x push`:                                 {"outward: git -…"},
		`git --$long=x status`:                                {"outward: git --…=x"},
		`"$HOME/bin/git" push`:                                {"outward: git push"},
		"git add . && git commit -m x && git push && git log": {"outward: git push"},
	})
}

func TestCommandsLetsGitsOtherCommandsBe(t *testing.T) {
	wantCommands(t, map[string][]string{
		"git":                          nil,
		"git status":                   nil,
		"git --no-pager log -p":        nil,
		"git -C push status":           nil,
		"git -c push fetch":            nil,
		"git --git-dir push pull":      nil,
		"git PUSH":                     nil,
		"git-receive-pack repo":        nil,
		"git -C":                       nil,
		"git --work-tree":              nil,
		"git commit -m 'git push'":     nil,
		"echo git push; printf push":   nil,
		"command -v tmux git; type gh": nil,
		"git fetch && git rebase":      nil,
		"git remote add origin x:y/z":  nil,
	})
}

func TestCommandsFindsGhsCommandsThatMayWrite(t *testing.T) {
	wantCommands(t, map[string][]string{
		"gh pr create --fill":               {"outward: gh pr create"},
		"gh pr merge 3 --squash":            {"outward: gh pr merge"},
		"gh issue comment 3 -b x":           {"outward: gh issue comment"},
		"gh workflow run ci.yml":            {"outward: gh workflow run"},
		"gh auth token":                     {"outward: gh auth token"},
		"gh pr":                             {"outward: gh pr"},
		"gh co 3":                           {"outward: gh co 3"},
		`gh "$area" view`:                   {"outward: gh … view"},
		`gh pr "$verb" 3`:                   {"outward: gh pr …"},
		"gh pr view 3 && gh pr review 3 -a": {"outward: gh pr review"},
	})
}

func TestCommandsLetsGhsReadsBe(t *testing.T) {
	wantCommands(t, map[string][]string{
		"gh":                         nil,
		"gh --help":                  nil,
		"gh --version":               nil,
		"gh -h":                      nil,
		"gh help pr":                 nil,
		"gh search issues baton":     nil,
		"gh status":                  nil,
		"gh version":                 nil,
		"gh pr --help":               nil,
		"gh pr -h":                   nil,
		"gh pr checks 3 --watch":     nil,
		"gh pr checkout 3":           nil,
		"gh repo clone o/r":          nil,
		"gh pr diff 3":               nil,
		"gh run download 9":          nil,
		"gh pr list --state all":     nil,
		"gh pr status":               nil,
		"gh issue view 3 --comments": nil,
		"gh run watch 9":             nil,
	})
}

func TestCommandsWatchesEveryUseOfSomePrograms(t *testing.T) {
	wantCommands(t, map[string][]string{
		"acli jira workitem view X-1": {"outward: acli"},
		"jira issue list":             {"outward: jira"},
		"scp a host:b":                {"outward: scp"},
		"sftp host":                   {"outward: sftp"},
		"ssh host true":               {"outward: ssh"},
		"osascript -e 'beep'":         {"control: osascript"},
		"tmux ls":                     {"control: tmux"},
		"aider --yes":                 {"agent: aider"},
		"claude -p x":                 {"agent: claude"},
		"codex exec x":                {"agent: codex"},
		"copilot -p x --allow-all":    {"agent: copilot"},
		"cursor-agent -p x":           {"agent: cursor-agent"},
		"gemini -p x":                 {"agent: gemini"},
		"opencode run x":              {"agent: opencode"},
		"/usr/bin/SSH host":           {"outward: ssh"},
	})
}

func TestCommandsNamesBatonsCommand(t *testing.T) {
	wantCommands(t, map[string][]string{
		"baton":                          {"baton: baton"},
		"baton wait":                     {"baton: baton wait"},
		"baton handover qa --file x.md":  {"baton: baton handover"},
		"/opt/homebrew/bin/baton status": {"baton: baton status"},
		`baton "$verb" x`:                {"baton: baton …"},
		"baton --help":                   {"baton: baton --help"},
		"baton feature start demo-1":     {"baton: baton feature start"},
		"baton hook guard":               {"baton: baton hook guard"},
		"baton feature":                  {"baton: baton feature"},
		`baton feature "$verb"`:          {"baton: baton feature …"},
		"baton help feature":             {"baton: baton help"},
	})
}

func TestCommandsWatchesProgramsThatOnlyTheRunDecides(t *testing.T) {
	wantCommands(t, map[string][]string{
		`"$GIT" push`:           {"unknown: … push"},
		`$tool`:                 {"unknown: …"},
		`$(which git) push x`:   {"unknown: … push"},
		`./"$script"`:           {"unknown: …"},
		`eval "$line"`:          {"unknown: …"},
		`bash -c "$script"`:     {"unknown: …"},
		`"${PYTHON}3" x.py`:     {"unknown: …3 x.py"},
		`ls "$dir"; "$@" && ls`: {"unknown: …"},
	})
}

func TestCommandsFollowsWrappersAndNestedScripts(t *testing.T) {
	wantCommands(t, map[string][]string{
		"env GIT_DIR=x git push":           {"outward: git push"},
		"nohup gh pr merge 3 &":            {"outward: gh pr merge"},
		"bash -c 'git push && tmux ls'":    {"outward: git push", "control: tmux"},
		"echo $(gh pr create --fill)":      {"outward: gh pr create"},
		"xargs -n1 ssh < hosts":            {"outward: ssh"},
		"f() { git push; }; f":             {"outward: git push"},
		"eval 'copilot -p x'":              {"agent: copilot"},
		"git status; baton wait; tmux ls":  {"baton: baton wait", "control: tmux"},
		"ls; go test ./...; make; echo ok": nil,
	})
}

func TestCommandsSaysWhyAScriptDoesntParse(t *testing.T) {
	for _, script := range []string{"echo 'open", "git push (", `bash -c "echo 'open"`} {
		commands, err := guard.Commands(script)
		if err == nil || commands != nil {
			t.Errorf("Commands(%q) = %v, %v, want an error", script, commands, err)
		}
	}
}
