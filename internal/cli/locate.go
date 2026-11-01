package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/repo"
	"github.com/codedogapp/baton/internal/state"
)

// The environment variables that name a pane's role and its feature (decision 29).
const (
	envRole    = "BATON_ROLE"
	envFeature = "BATON_FEATURE"
)

// stateFolder is baton's folder in the git common folder.
const stateFolder = "baton"

// fromPane returns the role and the feature of the pane that runs baton. The orchestrator's pane
// names no feature, since the orchestrator works on them all (decision 44). It refuses a role that
// baton doesn't know, which no feature would ever pass to.
func fromPane(getenv func(string) string) (flow.Role, string, error) {
	role, slug := flow.Role(getenv(envRole)), getenv(envFeature)

	switch {
	case role == "":
		return "", "", fmt.Errorf(
			"want %s: run it in a pane that baton started, which names its role there", envRole)
	case !role.Known():
		return "", "", fmt.Errorf("unknown role %q in %s", role, envRole)
	case role == flow.RoleOrchestrator && slug != "":
		return "", "", fmt.Errorf("the orchestrator's pane names no feature, but %s names %s",
			envFeature, slug)
	case role != flow.RoleOrchestrator && slug == "":
		return "", "", fmt.Errorf("want %s: the %s's pane names its feature there", envFeature,
			role)
	default:
		return role, slug, nil
	}
}

// openStore returns the store in baton's folder of the repo that holds the folder.
func openStore(ctx context.Context, dir string) (state.Store, error) {
	common, err := repo.CommonDir(ctx, dir)
	if err != nil {
		return state.Store{}, err
	}

	return state.Store{Dir: filepath.Join(common, stateFolder)}, nil
}
