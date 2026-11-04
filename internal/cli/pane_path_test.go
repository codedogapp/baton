package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestPanePutsItsOwnFolderFirstOnThePath has the orchestrator's pane give its copilot the PATH
// with the folder of baton's own program first, so that the agents' baton commands run that
// program, unless the PATH finds it as baton already, itself or through a link, or the folder's
// name holds the separator of a PATH's folders, which no PATH can name (decision 62).
func TestPanePutsItsOwnFolderFirstOnThePath(t *testing.T) {
	tests := map[string]struct {
		program func(t *testing.T) string
		first   bool
	}{
		"a baton that the PATH lacks": {program: aside, first: true},
		"another baton first on the PATH": {
			program: func(t *testing.T) string {
				onPath(t, "baton")

				return aside(t)
			},
			first: true,
		},
		"this baton on the PATH": {
			program: func(t *testing.T) string { return onPath(t, "baton") },
		},
		"this baton through a link on the PATH": {
			program: func(t *testing.T) string {
				program, err := filepath.EvalSymlinks(onPath(t, "baton"))
				if err != nil {
					t.Fatal(err)
				}

				return program
			},
		},
		"a folder whose name holds the separator": {
			program: func(t *testing.T) string {
				folder := "a" + string(filepath.ListSeparator) + "b"

				return filepath.Join(t.TempDir(), folder, "baton")
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			standIn(t)

			mainDir, _ := repos(t)
			program := test.program(t)

			want := os.Getenv("PATH")
			if test.first {
				want = filepath.Dir(program) + string(filepath.ListSeparator) + want
			}

			env, _ := inPane(t, mainDir, orchestratorPane())
			env.Executable = func() (string, error) { return program, nil }

			got := runExec(t.Context(), env, nil)
			if got.result != (result{}) || len(got.execs) != 1 {
				t.Fatalf("got %+v; want copilot, with no output", got)
			}

			if paths := pathsIn(got.execs[0].env); !slices.Equal(paths, []string{want}) {
				t.Errorf("copilot's PATH is %q; want %q", paths, want)
			}
		})
	}
}

// aside returns the path of a program of the test's own, named baton, in a folder that the PATH
// lacks.
func aside(t *testing.T) string {
	t.Helper()

	program := filepath.Join(t.TempDir(), "baton")
	if err := os.WriteFile(program, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	return program
}

// pathsIn returns the values of the PATH in the environment.
func pathsIn(env []string) []string {
	var paths []string

	for _, variable := range env {
		if path, found := strings.CutPrefix(variable, "PATH="); found {
			paths = append(paths, path)
		}
	}

	return paths
}

// orchestratorEnv returns the environment that the orchestrator's copilot gets: baton's, with
// BATON_BIN naming this baton, for the stop hook, and the PATH with this baton's folder first,
// each in place of the user's own value (decisions 47 and 62).
func orchestratorEnv() []string {
	return append(withoutVars("BATON_BIN", "PATH"), "BATON_BIN="+self, "PATH="+selfFirst())
}

// selfFirst returns the PATH that a pane's copilot gets when this baton isn't on the PATH, as the
// tests' baton isn't: the PATH, with this baton's folder first (decision 62).
func selfFirst() string {
	return filepath.Dir(self) + string(filepath.ListSeparator) + os.Getenv("PATH")
}
