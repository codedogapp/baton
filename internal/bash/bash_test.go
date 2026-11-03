package bash_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/bash"
)

// calls returns the commands that the script runs, each as its words joined by spaces, with $
// for each dynamic part.
func calls(t *testing.T, script string) []string {
	t.Helper()

	found, err := bash.Calls(script)
	if err != nil {
		t.Fatalf("Calls(%q) says %v", script, err)
	}

	lines := make([]string, len(found))
	for i, words := range found {
		lines[i] = strings.ReplaceAll(strings.Join(words, " "), bash.Dynamic, "$")
	}

	return lines
}

// wantCalls checks the commands that each script runs.
func wantCalls(t *testing.T, cases map[string][]string) {
	t.Helper()

	for script, want := range cases {
		t.Run(script, func(t *testing.T) {
			if got := calls(t, script); !slices.Equal(got, want) {
				t.Errorf("Calls(%q) = %q, want %q", script, got, want)
			}
		})
	}
}

func TestCallsFindsEveryCommand(t *testing.T) {
	wantCalls(t, map[string][]string{
		"":                                    nil,
		"# git push":                          nil,
		"git push":                            {"git push"},
		"cd x && git -C y push; make || true": {"cd x", "git -C y push", "make", "true"},
		"echo 'git push'":                     {"echo git push"},
		"echo \"$(gh pr create)\" `tmux ls`":  {"echo $ $", "gh pr create", "tmux ls"},
		"diff <(curl -d x u) >(tee y)":        {"diff $ $", "curl -d x u", "tee y"},
		"echo ${X:-$(gh pr merge)} $((1 + $(wc -l < f)))": {
			"echo $ $", "gh pr merge", "wc -l",
		},
		"cat <<EOF\n$(git push)\nEOF":                    {"cat", "git push"},
		"cat <<'EOF'\n$(git push)\nEOF":                  {"cat"},
		"f() { git push; }":                              {"git push"},
		"x=$(tmux ls) y=1":                               {"tmux ls"},
		"GIT_DIR=x git push":                             {"git push"},
		"local a=$(copilot)":                             {"copilot"},
		"[[ $(git push) ]]":                              {"git push"},
		"time git push; ! git push":                      {"git push", "git push"},
		"coproc git push":                                {"git push"},
		"case $(baton answer) in x) tmux;; esac":         {"baton answer", "tmux"},
		"for f in $(git ls-files); do echo \"$f\"; done": {"git ls-files", "echo $"},
	})
}

func TestCallsReadsTheWordsAsBashPassesThem(t *testing.T) {
	for script, want := range map[string][]string{
		`'git push' x`:           {"git push", "x"},
		`g'i't "p"u\sh`:          {"git", "push"},
		"\\git pu\\\nsh":         {"git", "push"},
		`"a\$b\c\"d\\e\` + "`\"": {"a$b\\c\"d\\e`"},
		`"$HOME/bin/git" push`:   {bash.Dynamic + "/bin/git", "push"},
		`$'push' $"push" x$y`:    {bash.Dynamic, "push", "x" + bash.Dynamic},
		`git {push,pull}`:        {"git", "{push,pull}"},
		`echo a\`:                {"echo", `a\`},
	} {
		t.Run(script, func(t *testing.T) {
			found, err := bash.Calls(script)
			if err != nil || len(found) != 1 || !slices.Equal(found[0], want) {
				t.Errorf("Calls(%q) = %q, %v, want one command, %q", script, found, err, want)
			}
		})
	}
}

func TestCallsUnwrapsTheWrappers(t *testing.T) {
	wantCalls(t, map[string][]string{
		"nohup git push":                                  {"git push"},
		"NOHUP git push":                                  {"git push"},
		"env -i -u HOME A=1 B=2 git push":                 {"git push"},
		"/usr/bin/env -- git push":                        {"git push"},
		"env - git push":                                  {"git push"},
		"command -p git push":                             {"git push"},
		"command -v tmux; command -V tmux":                nil,
		"builtin exec -a name git push":                   {"git push"},
		"nice -n 5 git push; nice -5 git push":            {"git push", "git push"},
		"sudo -u root -- A=1 git push":                    {"git push"},
		"/usr/bin/time -o out git push":                   {"git push"},
		"timeout -k 5 -s9 10 git push":                    {"git push"},
		"gtimeout 1m git push":                            {"git push"},
		"xargs -0 -n1 -I{} git push {}":                   {"git push {}"},
		"stdbuf -oL git push":                             {"git push"},
		"caffeinate -i -t 60 git push":                    {"git push"},
		"nohup env A=1 timeout 5 git push":                {"git push"},
		"nohup; timeout; timeout 5; env -i; env -u; exec": nil,
		"xargs grep tmux":                                 {"grep tmux"},
		"xargs --verbose git push":                        {"git push"},
		"git -C x push":                                   {"git -C x push"},
	})
}

func TestCallsSkipsTheValuesOfTheWrappersOptions(t *testing.T) {
	for wrapper, letters := range map[string]string{
		"caffeinate":    "tw",
		"env":           "CLPSUu",
		"exec":          "a",
		"nice":          "n",
		"stdbuf":        "eio",
		"sudo":          "CDgpRrTtUu",
		"/usr/bin/time": "fo", // bash reads a plain time as its keyword.
		"timeout":       "ks",
		"xargs":         "EIJLPRSadns",
	} {
		for _, letter := range letters {
			script := fmt.Sprintf("%s -%c value 5 git push", wrapper, letter)
			if wrapper != "timeout" {
				script = strings.Replace(script, " 5", "", 1)
			}

			wantCalls(t, map[string][]string{script: {"git push"}})
		}
	}
}

func TestCallsReadsTheScriptsOfShellsAndEval(t *testing.T) {
	wantCalls(t, map[string][]string{
		"bash -c 'git push'":              {"git push"},
		`sh -lc "git push && tmux ls"`:    {"git push", "tmux ls"},
		"bash -o pipefail -c 'git push'":  {"git push"},
		"bash -O extglob -c 'git push'":   {"git push"},
		"dash -c 'git push'; ksh -c tmux": {"git push", "tmux"},
		"zsh -oclobber script.sh":         {"zsh -oclobber script.sh"},
		"bash --rcfile rc script.sh":      {"bash --rcfile rc script.sh"},
		"bash -c 'git push' sh x":         {"git push"},
		"zsh -ec -- 'git push'":           {"git push"},
		"/bin/bash --norc -c 'git push'":  {"git push"},
		"eval git push":                   {"git push"},
		`eval "git push; tmux"`:           {"git push", "tmux"},
		`nohup sh -c 'eval "git push"'`:   {"git push"},
		`bash -c "git push $REMOTE"`:      {"git push $"},
		`bash -c "$CMD"`:                  {"$"},
		"xargs -I{} sh -c 'git push {}'":  {"git push {}"},
		"bash script.sh; bash; bash -c; bash -C x": {
			"bash script.sh",
			"bash",
			"bash -c",
			"bash -C x",
		},
	})
}

func TestCallsRefusesAScriptThatDoesntParse(t *testing.T) {
	for script, want := range map[string]string{
		`echo "x`:              "1:6: reached EOF without closing quote `\"`",
		"if true; then":        "1:10: `then` must be followed by a statement list",
		`bash -c 'echo "x'`:    "bash's script: 1:6: reached EOF without closing quote `\"`",
		`eval 'git push "'`:    "eval's script: 1:10: reached EOF without closing quote `\"`",
		`echo $(sh -c 'a "b')`: "sh's script: 1:3: reached EOF without closing quote `\"`",
	} {
		t.Run(script, func(t *testing.T) {
			_, err := bash.Calls(script)
			if err == nil || err.Error() != want {
				t.Errorf("Calls(%q) says %v, want %s", script, err, want)
			}
		})
	}
}

func TestName(t *testing.T) {
	for word, want := range map[string]string{
		"git":                      "git",
		"/usr/bin/GIT":             "git",
		bash.Dynamic + "/bin/Tmux": "tmux",
		"":                         ".",
	} {
		if got := bash.Name(word); got != want {
			t.Errorf("Name(%q) = %q, want %q", word, got, want)
		}
	}
}
