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

// fromPane returns the role and the feature of the pane that runs baton. It refuses a role that
// baton doesn't know, which no feature would ever pass to.
func fromPane(getenv func(string) string) (flow.Role, string, error) {
	role, slug := flow.Role(getenv(envRole)), getenv(envFeature)

	switch {
	case role == "" || slug == "":
		return "", "", fmt.Errorf(
			"run it in a role's pane, where %s and %s name the role and the feature",
			envRole, envFeature)
	case !role.Known():
		return "", "", fmt.Errorf("unknown role %q in %s", role, envRole)
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
