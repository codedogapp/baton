// Package config reads baton's config file, config.json in baton's folder in the user's config
// folder: the settings for every repo, and a section for each repo, keyed by its remote's URL, so
// that nothing goes into a work repo (decision 16).
package config

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/codedogapp/baton/internal/flow"
)

// The config file's folder in the user's config folder, and its name there.
const (
	folder = "baton"
	file   = "config.json"
)

// baton's own settings, where the config file gives none (decisions 7, 13 and 41).
const (
	model        = "claude-opus-5.5"
	roundCap     = 3
	branchPrefix = "feature/"
)

// Settings are baton's settings for a repo. A setting that a layer leaves out, or gives its zero
// value, keeps the one below it: the repo's section lies over the file's settings for every repo,
// and those over baton's own (decision 49).
type Settings struct {
	// Roles holds each role's settings for its copilot.
	Roles map[flow.Role]Role `json:"roles"`
	// RoundCap is the engineer's passes that a brief gets (decision 13).
	RoundCap int `json:"roundCap"`
	// BranchPrefix comes before the slug in the name of a feature's branch (decision 41).
	BranchPrefix string `json:"branchPrefix"`
}

// Role is a role's settings for its copilot, each one a flag of copilot's (fact 31).
type Role struct {
	Model string `json:"model"`
	// ReasoningEffort is empty for copilot's own.
	ReasoningEffort string `json:"reasoningEffort"`
	// MaxAICredits caps the AI credits that the role's session spends: 30 or more, or 0 for no
	// cap (fact 10).
	MaxAICredits float64 `json:"maxAiCredits"`
	// ExtraTools are the orchestrator's tools besides baton's, the user's own, such as nudge's
	// for when the user is away. A nil list keeps the one below it, and an empty one gives none.
	ExtraTools []string `json:"extraTools"`
}

// defaults returns baton's own settings: every role on the same model, and the orchestrator with
// the user's nudge tools (decision 45).
func defaults() Settings {
	roles := map[flow.Role]Role{flow.RoleOrchestrator: {
		Model: model, ExtraTools: []string{"nudge-ask", "nudge-report"},
	}}

	for _, role := range []flow.Role{
		flow.RoleLead, flow.RoleEngineer, flow.RoleQA, flow.RoleReviewer,
	} {
		roles[role] = Role{Model: model}
	}

	return Settings{Roles: roles, RoundCap: roundCap, BranchPrefix: branchPrefix}
}

// Load reads the config file in the user's config folder, and returns the settings for the repo
// whose remote has the URL. A missing file gives baton's own. It checks the whole file, the other
// repos' sections too, so that a mistake shows at once.
func Load(configDir, remote string) (Settings, error) {
	path := filepath.Join(configDir, folder, file)

	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		return defaults(), nil
	}

	if err != nil {
		return Settings{}, fmt.Errorf("read the config file: %w", err)
	}

	var content layers

	err = json.Unmarshal(data, &content, json.RejectUnknownMembers(true))
	if err == nil {
		err = check(content.Settings, content.Repos)
	}

	if err != nil {
		return Settings{}, fmt.Errorf("config file %s: %w", path, err)
	}

	return defaults().with(content.Settings).with(content.Repos[key(remote)]), nil
}

// layers are the config file's content: its settings for every repo, and each repo's section, by
// the repo's key.
type layers struct {
	Settings

	Repos map[string]Settings `json:"repos"`
}

// with returns the settings with the layer over them. The settings below hold every role, and the
// layer, once checked, no other.
func (s Settings) with(layer Settings) Settings {
	roles := make(map[flow.Role]Role, len(s.Roles))
	for role, below := range s.Roles {
		roles[role] = below.with(layer.Roles[role])
	}

	return Settings{
		Roles:        roles,
		RoundCap:     cmp.Or(layer.RoundCap, s.RoundCap),
		BranchPrefix: cmp.Or(layer.BranchPrefix, s.BranchPrefix),
	}
}

// with returns the role's settings with the layer over them.
func (r Role) with(layer Role) Role {
	if layer.ExtraTools != nil {
		r.ExtraTools = layer.ExtraTools
	}

	r.Model = cmp.Or(layer.Model, r.Model)
	r.ReasoningEffort = cmp.Or(layer.ReasoningEffort, r.ReasoningEffort)
	r.MaxAICredits = cmp.Or(layer.MaxAICredits, r.MaxAICredits)

	return r
}
