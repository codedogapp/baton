package tmux_test

import (
	"testing"

	"github.com/codedogapp/baton/internal/tmux"
)

func TestNameReplacesWhatASessionsNameCantHold(t *testing.T) {
	for text, want := range map[string]string{
		"baton-my.repo-demo-1": "baton-my_repo-demo-1",
		"a b:c#d=é\n":          "a_b_c_d___",
		"Repo_2-x":             "Repo_2-x",
	} {
		if got := tmux.Name(text); got != want {
			t.Errorf("Name(%q) = %q, want %q", text, got, want)
		}
	}
}
