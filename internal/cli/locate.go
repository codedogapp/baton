package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codedogapp/baton/internal/config"
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

// loadSettings returns baton's settings for the repo that holds the folder, from the config file
// in the user's config folder, by the URL of the repo's origin remote (decision 49).
func loadSettings(ctx context.Context, dir string) (config.Settings, error) {
	remote, err := repo.Remote(ctx, dir)
	if err != nil {
		return config.Settings{}, err
	}

	return settingsOf(remote)
}

// settingsOf returns baton's settings for the repo whose origin remote has the URL, from the
// config file in the user's config folder (decision 49).
func settingsOf(remote string) (config.Settings, error) {
	configDir, err := userConfigDir()
	if err != nil {
		return config.Settings{}, err
	}

	return config.Load(configDir, remote)
}

// userConfigDir returns the user's config folder, which holds baton's, with the config file
// (decision 49).
func userConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the config folder: %w", err)
	}

	return dir, nil
}

// configFile returns the path of the config file, which an error can name, or a name for it where
// the user's config folder is unknown.
func configFile() string {
	file := "baton's config file"
	if configDir, err := os.UserConfigDir(); err == nil {
		file = config.File(configDir)
	}

	return file
}
