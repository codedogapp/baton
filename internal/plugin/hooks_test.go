package plugin_test

import (
	"encoding/json/v2"
	"io/fs"
	"maps"
	"slices"
	"testing"
)

// TestTheStopHookRunsBaton checks the plugin's hooks: the agentStop hook alone, which runs baton
// hook stop with the program that baton pane names in the pane's copilot (decision 47).
func TestTheStopHookRunsBaton(t *testing.T) {
	data, err := fs.ReadFile(written(t), "com.github.copilot/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}

	type hook struct {
		Type string `json:"type"`
		Bash string `json:"bash"`
	}

	var config struct {
		Version int               `json:"version"`
		Hooks   map[string][]hook `json:"hooks"`
	}

	if err := json.Unmarshal(data, &config, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("hooks.json: %v", err)
	}

	want := map[string][]hook{"agentStop": {{Type: "command", Bash: `"$BATON_BIN" hook stop`}}}
	if config.Version != 1 || !maps.EqualFunc(config.Hooks, want, slices.Equal) {
		t.Errorf("hooks.json is %+v, want the version 1 and the hooks %+v", config, want)
	}
}
