package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/config"
)

func TestLoadRefusesSettingsThatBatonCantUse(t *testing.T) {
	for content, want := range map[string]string{
		`{"roundCap": -1}`:                         "roundCap -1: want 1 or more",
		`{"roles": {"boss": {}}}`:                  "roles: boss: want lead, engineer, qa, reviewer or orchestrator",
		`{"roles": {"lead": {"model": "--yolo"}}}`: `roles: lead: model "--yolo": want a model's name`,
		`{"roles": {"qa": {"reasoningEffort": "extreme"}}}`: `roles: qa: reasoningEffort "extreme": ` +
			"want one of none, minimal, low, medium, high, xhigh, max",
		`{"roles": {"engineer": {"maxAiCredits": 29.5}}}`: "roles: engineer: maxAiCredits 29.5: " +
			"want 30 or more, or 0 for no cap",
		`{"roles": {"engineer": {"maxAiCredits": -30}}}`: "roles: engineer: maxAiCredits -30: " +
			"want 30 or more, or 0 for no cap",
		`{"roles": {"reviewer": {"extraTools": []}}}`: "roles: reviewer: extraTools: " +
			"only the orchestrator gets tools of the user's",
		`{"roles": {"orchestrator": {"extraTools": ["nudge-ask", ""]}}}`: `roles: orchestrator: ` +
			`extraTools: "": want a tool's name`,
		`{"roles": {"orchestrator": {"extraTools": ["--yolo"]}}}`: `roles: orchestrator: ` +
			`extraTools: "--yolo": want a tool's name`,
		`{"roles": {"qa": {"model": "-q"}, "lead": {"model": "-l"}}}`: `roles: lead: model "-l": ` +
			"want a model's name",
		`{"repos": {"": {}}}`: "repos: an empty key: want a remote's URL",
		`{"repos": {"` + remote + `": {}}}`: `repos: "` + remote + `": want "` + repo +
			`", the URL's host and path`,
		`{"repos": {"github.com/codedogapp/tap": {"roundCap": -2}}}`: "repos: " +
			"github.com/codedogapp/tap: roundCap -2: want 1 or more",
		`{"repos": {"github.com/codedogapp/tap": {"roles": {"qa": {"model": "-q"}}}}}`: "repos: " +
			`github.com/codedogapp/tap: roles: qa: model "-q": want a model's name`,
		`{"repos": {"github.com/b/b": {"roundCap": -2}, "github.com/a/a": {"roundCap": -1}}}`: "repos: " +
			"github.com/a/a: roundCap -1: want 1 or more",
		`{"roundCap": -3, "repos": {"github.com/a/a": {"roundCap": -1}}}`: "roundCap -3: " +
			"want 1 or more",
	} {
		t.Run(want, func(t *testing.T) {
			dir := withFile(t, content)

			_, err := config.Load(dir, remote)
			if err == nil {
				t.Fatalf("Load took %s", content)
			}

			if want := "config file " + filePath(dir) + ": " + want; err.Error() != want {
				t.Errorf("Load says %q, want %q", err, want)
			}
		})
	}
}

func TestLoadRefusesAFileThatIsntSettings(t *testing.T) {
	for content, want := range map[string]string{
		``:                       "unexpected EOF",
		`{`:                      "unexpected EOF",
		`[]`:                     "unmarshal JSON array",
		`{"roundCap": "3"}`:      `within "/roundCap"`,
		`{"planApproval": "no"}`: `within "/planApproval"`,
		`{"rounds": 2}`:          `unknown object member name "rounds"`,
		`{"RoundCap": 2}`:        `unknown object member name "RoundCap"`,
		`{"roles": {"lead": {"effort": "high"}}}`: `unknown object member name "effort" within "/roles/lead"`,
		`{"repos": {"github.com/a/a": {"repos": {}}}}`: `unknown object member name "repos" ` +
			`within "/repos/github.com~1a~1a"`,
		`{"roundCap": 2, "roundCap": 3}`:      `duplicate object member name "roundCap"`,
		`{"roles": {"lead": {}, "lead": {}}}`: `duplicate object member name "lead" within "/roles"`,
	} {
		t.Run(content, func(t *testing.T) {
			dir := withFile(t, content)

			_, err := config.Load(dir, remote)
			if err == nil {
				t.Fatalf("Load took %q", content)
			}

			if prefix := "config file " + filePath(dir) + ": "; !strings.HasPrefix(err.Error(),
				prefix) || !strings.Contains(err.Error(), want) {
				t.Errorf("Load says %q, want %q after %q", err, want, prefix)
			}
		})
	}
}

func TestLoadFailsOnAFileThatItCantRead(t *testing.T) {
	dir := withFolder(t)

	if err := os.Mkdir(filePath(dir), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(dir, remote)
	if want := "read the config file: "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Load says %v, want an error after %q", err, want)
	}
}
