package plugin_test

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/plugin"
)

// agents is the folder of the plugin's agents, which only Copilot CLI reads (fact 24).
const agents = "com.github.copilot/agents"

// roles are the roles with an agent in the plugin: all of them (decision 37).
var roles = []flow.Role{
	flow.RoleOrchestrator, flow.RoleLead, flow.RoleEngineer, flow.RoleQA, flow.RoleReviewer,
}

// TestThePluginIsBatons checks the plugin.json that Copilot CLI needs to load the plugin.
func TestThePluginIsBatons(t *testing.T) {
	data, err := fs.ReadFile(written(t), "plugin.json")
	if err != nil {
		t.Fatal(err)
	}

	var manifest struct {
		Schema string `json:"$schema"`
		Name   string `json:"name"`
	}

	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json: %v", err)
	}

	schema := "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	if manifest.Schema != schema || manifest.Name != plugin.Name {
		t.Errorf("plugin.json is %+v, want the $schema %s and the name %s",
			manifest, schema, plugin.Name)
	}
}

// TestTheAgentsAreTheRoles checks that the plugin has the roles' agents and no others.
func TestTheAgentsAreTheRoles(t *testing.T) {
	entries, err := fs.ReadDir(written(t), agents)
	if err != nil {
		t.Fatal(err)
	}

	var got, want []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	for _, role := range roles {
		want = append(want, string(role)+".agent.md")
	}

	if slices.Sort(want); !slices.Equal(got, want) {
		t.Errorf("the agents are %v, want %v", got, want)
	}
}

// TestEachAgent checks each role's agent: that its ID names it, since the ID is the plugin's
// name and the agent file's (fact 24), and that its prompt says how to move the work and wait.
func TestEachAgent(t *testing.T) {
	tree := written(t)

	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			name, found := strings.CutPrefix(plugin.Agent(role), plugin.Name+":")
			if !found {
				t.Fatalf("Agent(%s) = %s, want it to start with %s:", role, plugin.Agent(role),
					plugin.Name)
			}

			fields, prompt := readAgent(t, tree, agents+"/"+name+".agent.md")
			if fields["name"] != string(role) || fields["description"] == "" {
				t.Errorf("the frontmatter is %v, want the name %s and a description", fields, role)
			}

			for _, command := range commands(role) {
				if !strings.Contains(prompt, command) {
					t.Errorf("the prompt never says %q", command)
				}
			}
		})
	}
}

// written writes the plugin into a new cache, and returns its folder.
func written(t *testing.T) fs.FS {
	t.Helper()

	dir, err := plugin.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return os.DirFS(dir)
}

// readAgent returns the fields in the agent file's frontmatter, and its prompt.
func readAgent(t *testing.T, tree fs.FS, path string) (map[string]string, string) {
	t.Helper()

	data, err := fs.ReadFile(tree, path)
	if err != nil {
		t.Fatal(err)
	}

	rest, opens := strings.CutPrefix(string(data), "---\n")

	head, prompt, closes := strings.Cut(rest, "\n---\n")
	if !opens || !closes {
		t.Fatalf("%s has no frontmatter between --- lines", path)
	}

	fields := map[string]string{}

	for line := range strings.SplitSeq(head, "\n") {
		key, value, _ := strings.Cut(line, ": ")
		if strings.Contains(value, ": ") {
			t.Errorf("YAML can't take %q unquoted: its value holds a colon and a space", line)
		}

		fields[key] = value
	}

	return fields, prompt
}

// commands returns what the role's prompt must say for the role to move the work, to wait, and to
// ask or answer: the orchestrator starts features, answers questions and hands features back, and
// the others hand over and ask.
func commands(role flow.Role) []string {
	said := []string{"start `baton wait`", "in async mode"}
	if role == flow.RoleOrchestrator {
		return append(said, "baton feature start <slug> --brief", "baton handover <slug> --to",
			"`baton answer <slug> \"<answer>\"`")
	}

	said = append(said, "baton handover --note", "start `baton ask \"<question>\"`")
	if role == flow.RoleQA || role == flow.RoleReviewer {
		said = append(said, "--verdict approve", "--verdict reject")
	}

	return said
}
