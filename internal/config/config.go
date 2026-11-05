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

// baton's own settings, where the config file gives none (decisions 7, 8, 13 and 41).
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
	// PlanApproval makes the user approve each feature start in the orchestrator's pane (decision
	// 8). A nil one keeps the one below it, so that false can turn it off.
	PlanApproval *bool `json:"planApproval"`
	// Gate is the command that checks the repo's code: /bin/sh runs it in the feature's worktree
	// before each handover that it guards. baton has none of its own, and a feature can't start
	// without one (decision 56).
	Gate string `json:"gate"`
	// Jira is the Jira site that baton jira reads tickets from (decision 60).
	Jira Jira `json:"jira"`
	// Bitbucket is the user's account on Bitbucket Cloud, which baton deliver opens a repo's pull
	// requests there as (decision 65).
	Bitbucket Bitbucket `json:"bitbucket"`
}

// Jira is a Jira Cloud site, and the user's account there, whose API token the login keychain
// holds (decision 60).
type Jira struct {
	// Site is the site's host name, such as example.atlassian.net.
	Site string `json:"site"`
	// Email is the user's email address at Atlassian, which names their account.
	Email string `json:"email"`
}

// Bitbucket is the user's account on Bitbucket Cloud, whose API token the login keychain holds
// (decision 65).
type Bitbucket struct {
	// Email is the user's email address at Atlassian, which names their account.
	Email string `json:"email"`
}

// PlanApprovalOn reports whether the user approves each feature start in the orchestrator's pane.
func (s Settings) PlanApprovalOn() bool {
	return s.PlanApproval == nil || *s.PlanApproval
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

// defaults returns baton's own settings: every role on the same model, the orchestrator with the
// user's nudge tools (decision 45), and plan approval on.
func defaults() Settings {
	roles := map[flow.Role]Role{flow.RoleOrchestrator: {
		Model: model, ExtraTools: []string{"nudge-ask", "nudge-report"},
	}}

	for _, role := range []flow.Role{
		flow.RoleLead, flow.RoleEngineer, flow.RoleQA, flow.RoleReviewer,
	} {
		roles[role] = Role{Model: model}
	}

	return Settings{
		Roles: roles, RoundCap: roundCap, BranchPrefix: branchPrefix, PlanApproval: new(true),
	}
}

// Folder returns baton's folder in the user's config folder, which holds the config file.
func Folder(configDir string) string {
	return filepath.Join(configDir, folder)
}

// File returns the path of the config file in the user's config folder.
func File(configDir string) string {
	return filepath.Join(Folder(configDir), file)
}

// Load reads the config file in the user's config folder, and returns the settings for the repo
// whose remote has the URL. A missing file gives baton's own. It checks the whole file, the other
// repos' sections too, so that a mistake shows at once.
func Load(configDir, remote string) (Settings, error) {
	path := File(configDir)

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

	return defaults().with(content.Settings).with(content.Repos[Key(remote)]), nil
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
		PlanApproval: cmp.Or(layer.PlanApproval, s.PlanApproval),
		Gate:         cmp.Or(layer.Gate, s.Gate),
		Jira:         s.Jira.with(layer.Jira),
		Bitbucket:    Bitbucket{Email: cmp.Or(layer.Bitbucket.Email, s.Bitbucket.Email)},
	}
}

// with returns the Jira settings with the layer over them.
func (j Jira) with(layer Jira) Jira {
	return Jira{Site: cmp.Or(layer.Site, j.Site), Email: cmp.Or(layer.Email, j.Email)}
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
