package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/codedogapp/baton/internal/config"
	"github.com/codedogapp/baton/internal/flow"
)

// The remote of the repo that the tests load the settings for, and its key in the config file.
const (
	remote = "git@github.com:codedogapp/baton.git"
	repo   = "github.com/codedogapp/baton"
)

func TestLoadGivesBatonsOwnWithoutSettings(t *testing.T) {
	for name, dir := range map[string]string{
		"without baton's folder": t.TempDir(),
		"without the file":       withFolder(t),
		"with no settings":       withFile(t, `{}`),
		"with empty sections":    withFile(t, `{"roles": {}, "repos": {"`+repo+`": {}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := config.Load(dir, remote)
			if err != nil {
				t.Fatal(err)
			}

			if want := own(); !reflect.DeepEqual(got, want) {
				t.Errorf("Load gives\n%+v\nwant baton's own\n%+v", got, want)
			}
		})
	}
}

func TestLoadLaysTheFileOverBatonsOwn(t *testing.T) {
	got := load(t, `{
		"roundCap": 2,
		"branchPrefix": "team/",
		"roles": {
			"lead": {"model": "", "reasoningEffort": "high"},
			"qa": {"model": "claude-sonnet-5.5", "maxAiCredits": 42.5},
			"orchestrator": {"extraTools": []}
		}
	}`, remote)

	want := own()
	want.RoundCap = 2
	want.BranchPrefix = "team/"
	want.Roles[flow.RoleLead] = config.Role{Model: model, ReasoningEffort: "high"}
	want.Roles[flow.RoleQA] = config.Role{Model: "claude-sonnet-5.5", MaxAICredits: 42.5}
	want.Roles[flow.RoleOrchestrator] = config.Role{Model: model, ExtraTools: []string{}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load gives\n%+v\nwant\n%+v", got, want)
	}
}

func TestLoadLaysTheRepoOverTheFile(t *testing.T) {
	const content = `{
		"roundCap": 2,
		"branchPrefix": "team/",
		"roles": {
			"lead": {"model": "claude-sonnet-5.5", "reasoningEffort": "high"},
			"engineer": {"maxAiCredits": 30},
			"qa": {"maxAiCredits": 42.5},
			"orchestrator": {"extraTools": ["nudge-ask"]}
		},
		"repos": {
			"` + repo + `": {
				"roundCap": 4,
				"branchPrefix": "baton/",
				"roles": {
					"lead": {"model": "gpt-6-sol", "reasoningEffort": "low"},
					"engineer": {"model": "gpt-6-sol", "maxAiCredits": 0},
					"qa": {"maxAiCredits": 200},
					"orchestrator": {"extraTools": []}
				}
			},
			"github.com/codedogapp/nudge": {}
		}
	}`

	file := own()
	file.RoundCap = 2
	file.BranchPrefix = "team/"
	file.Roles[flow.RoleLead] = config.Role{Model: "claude-sonnet-5.5", ReasoningEffort: "high"}
	file.Roles[flow.RoleEngineer] = config.Role{Model: model, MaxAICredits: 30}
	file.Roles[flow.RoleQA] = config.Role{Model: model, MaxAICredits: 42.5}
	file.Roles[flow.RoleOrchestrator] = config.Role{Model: model, ExtraTools: []string{"nudge-ask"}}

	repoOwn := own()
	repoOwn.RoundCap = 4
	repoOwn.BranchPrefix = "baton/"
	repoOwn.Roles[flow.RoleLead] = config.Role{Model: "gpt-6-sol", ReasoningEffort: "low"}
	repoOwn.Roles[flow.RoleEngineer] = config.Role{Model: "gpt-6-sol", MaxAICredits: 30}
	repoOwn.Roles[flow.RoleQA] = config.Role{Model: model, MaxAICredits: 200}
	repoOwn.Roles[flow.RoleOrchestrator] = config.Role{Model: model, ExtraTools: []string{}}

	for name, test := range map[string]struct {
		remote string
		want   config.Settings
	}{
		"with its section":        {remote: remote, want: repoOwn},
		"with an empty section":   {remote: "https://github.com/codedogapp/nudge", want: file},
		"without a section":       {remote: "https://github.com/codedogapp/tap", want: file},
		"without a remote at all": {remote: "", want: file},
	} {
		t.Run(name, func(t *testing.T) {
			if got := load(t, content, test.remote); !reflect.DeepEqual(got, test.want) {
				t.Errorf("Load gives\n%+v\nwant\n%+v", got, test.want)
			}
		})
	}
}

func TestLoadFindsTheRepoByItsRemotesURL(t *testing.T) {
	const content = `{"repos": {
		"` + repo + `": {"roundCap": 5},
		"/srv/git/baton": {"roundCap": 6},
		"/srv/git:baton": {"roundCap": 7}
	}}`

	for given, want := range map[string]int{
		"https://github.com/codedogapp/baton":            5,
		"https://github.com/codedogapp/baton.git":        5,
		"https://user@github.com/codedogapp/baton/":      5,
		"https://github.com:443/codedogapp/baton.git":    5,
		"ssh://git@github.com/codedogapp/baton.git":      5,
		"ssh://git@github.com:22/codedogapp/baton":       5,
		"git://github.com/codedogapp/baton.git":          5,
		"git@github.com:codedogapp/baton.git":            5,
		"git@github.com:/codedogapp/baton.git/":          5,
		"me@example.com@github.com:codedogapp/baton.git": 5,
		"github.com:codedogapp/baton":                    5,
		"/srv/git/baton.git":                             6,
		"file:///srv/git/baton/":                         6,
		"/srv/git:baton":                                 7,
		"https://github.com/codedogapp/baton-x":          3,
		"https://github.com/codedogapp/baton/x":          3,
		"https://gitlab.com/codedogapp/baton":            3,
		"git@github.com:codedogapp/baton.git.git/x":      3,
		"https://github.com/codedogapp/baton.github":     3,
	} {
		t.Run(given, func(t *testing.T) {
			if got := load(t, content, given).RoundCap; got != want {
				t.Errorf("the round cap is %d, want %d", got, want)
			}
		})
	}
}

// model is baton's own model for every role.
const model = "claude-opus-5.5"

// own returns baton's own settings.
func own() config.Settings {
	return config.Settings{
		Roles: map[flow.Role]config.Role{
			flow.RoleOrchestrator: {
				Model:      model,
				ExtraTools: []string{"nudge-ask", "nudge-report"},
			},
			flow.RoleLead:     {Model: model},
			flow.RoleEngineer: {Model: model},
			flow.RoleQA:       {Model: model},
			flow.RoleReviewer: {Model: model},
		},
		RoundCap:     3,
		BranchPrefix: "feature/",
	}
}

// load returns the settings for the remote from a config file with the content.
func load(t *testing.T, content, remote string) config.Settings {
	t.Helper()

	got, err := config.Load(withFile(t, content), remote)
	if err != nil {
		t.Fatal(err)
	}

	return got
}

// withFile returns a config folder of the test's own, with baton's config file holding the
// content.
func withFile(t *testing.T, content string) string {
	t.Helper()

	dir := withFolder(t)

	if err := os.WriteFile(filePath(dir), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return dir
}

// withFolder returns a config folder of the test's own, with baton's folder in it.
func withFolder(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.Mkdir(filepath.Dir(filePath(dir)), 0o700); err != nil {
		t.Fatal(err)
	}

	return dir
}

// filePath returns the path of baton's config file in the config folder.
func filePath(dir string) string {
	return filepath.Join(dir, "baton", "config.json")
}
