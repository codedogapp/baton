// Package tmux makes baton's tmux sessions: a session per feature, with a titled pane per role
// (decisions 14 and 32).
package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Server is a tmux server. The zero value is the user's: the server that baton runs in, or tmux's
// default one.
type Server struct {
	// Socket names a server of its own (tmux -L), for tests and live checks.
	Socket string
	// Config is the file that the server reads if baton starts it (tmux -f); empty means the
	// user's.
	Config string
}

// Session is a tmux session to make.
type Session struct {
	// Name is letters, digits, hyphens and underscores, which tmux keeps as they are and finds
	// exactly.
	Name string
	// Dir is the absolute path of the folder that the panes start in.
	Dir string
	// Panes are tiled in order, and the first one is the active one.
	Panes []Pane
}

// Pane is a pane of a session.
type Pane struct {
	// Title shows in the pane's top border, and programs in the pane can't change it.
	Title string
	// Env holds NAME=value entries for this pane's command only.
	Env []string
	// Command is the program and its arguments, run as they are; tmux runs a lone string with
	// the shell.
	Command []string
}

// A session's name, and a character that a name can't hold.
var (
	sessionName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	notInName   = regexp.MustCompile(`[^A-Za-z0-9_-]`)
)

// Name returns the text as a session's name, with an underscore for each character that a name
// can't hold, such as a dot.
func Name(text string) string {
	return notInName.ReplaceAllString(text, "_")
}

// NewSession makes the session, detached, with a window of its own that tiles the panes and shows
// their titles in their top borders. It sets options on that window alone. If anything fails once
// tmux has made the session, a hook of the user's included, NewSession ends the session.
func (s Server) NewSession(ctx context.Context, session Session) error {
	if err := session.check(); err != nil {
		return err
	}

	head := []string{"new-session", "-s", session.Name, "-F", "#{window_id} #{pane_id}"}

	// tmux prints the IDs once it has made the session, even if a hook of the user's then fails.
	out, err := s.run(ctx, session.spawn(session.Panes[0], head...)...)
	window, first, made := strings.Cut(line(out), " ")

	if err == nil {
		err = s.fill(ctx, session, window, first)
	}

	if err != nil && made {
		// The session is the one tmux made a moment ago, so its exact name finds it, even after
		// Ctrl-C.
		_, kill := s.run(context.WithoutCancel(ctx), "kill-session", "-t", "="+session.Name)
		err = errors.Join(err, kill)
	}

	if err != nil {
		return fmt.Errorf("make the tmux session %s: %w", session.Name, err)
	}

	return nil
}

// fill sets the window's options and takes the first pane's environment out of the session's,
// splits the other panes off the last one in order, tiling the window each time, and titles them.
func (s Server) fill(ctx context.Context, session Session, window, first string) error {
	setUp := []string{
		"set-option", "-w", "-t", window, "pane-border-status", "top", ";",
		"set-option", "-w", "-t", window, "allow-set-title", "off",
	}

	// new-session keeps its -e entries in the session's environment, for every later pane
	// (fact 22).
	for _, env := range session.Panes[0].Env {
		name, _, _ := strings.Cut(env, "=")
		setUp = append(setUp, ";", "set-environment", "-t", "="+session.Name, "-u", quote(name))
	}

	if _, err := s.run(ctx, setUp...); err != nil {
		return err
	}

	ids := []string{first}

	for _, pane := range session.Panes[1:] {
		split := session.spawn(pane, "split-window", "-t", ids[len(ids)-1], "-F", "#{pane_id}")

		out, err := s.run(ctx, append(split, ";", "select-layout", "-t", window, "tiled")...)
		if err != nil {
			return err
		}

		ids = append(ids, line(out))
	}

	var titles []string
	for i, id := range ids {
		titles = append(titles, ";", "select-pane", "-t", id, "-T", text(session.Panes[i].Title))
	}

	_, err := s.run(ctx, titles[1:]...)

	return err
}

// spawn returns the tmux command that starts the pane: detached, in the session's folder, with the
// pane's environment and command, printing the pane's IDs.
func (session Session) spawn(pane Pane, head ...string) []string {
	args := slices.Concat(head, []string{"-d", "-P", "-c", text(session.Dir)})
	for _, env := range pane.Env {
		args = append(args, "-e", quote(env))
	}

	args = append(args, "--")
	for _, arg := range pane.Command {
		args = append(args, quote(arg))
	}

	return args
}

// check refuses a session that tmux would make wrong: a name that it would change or find by
// mistake, a folder that it would quietly swap for the home folder, or no pane (fact 22).
func (session Session) check() error {
	if !sessionName.MatchString(session.Name) {
		return fmt.Errorf(
			"tmux session name %q: want letters, digits, hyphens and underscores",
			session.Name,
		)
	}

	if len(session.Panes) == 0 {
		return fmt.Errorf("tmux session %s: want a pane", session.Name)
	}

	if !filepath.IsAbs(session.Dir) {
		return fmt.Errorf("tmux session %s: want an absolute folder, got %q", session.Name,
			session.Dir)
	}

	info, err := os.Stat(session.Dir)
	if err != nil {
		return fmt.Errorf("tmux session %s: %w", session.Name, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("tmux session %s: %s is not a folder", session.Name, session.Dir)
	}

	return nil
}

// run runs tmux on the server and returns what it printed, even if it failed.
func (s Server) run(ctx context.Context, args ...string) (string, error) {
	var global []string
	if s.Socket != "" {
		global = append(global, "-L", s.Socket)
	}

	if s.Config != "" {
		global = append(global, "-f", s.Config)
	}

	// Sonar's go:S4036 wants tmux at a fixed path, but baton runs the user's own tmux, found on
	// the user's PATH as the user's shell finds it (decision 27). gosec's G204 wants constant
	// arguments, but tmux needs the session's own; they reach it as text, which no shell reads
	// (decision 34).
	//nolint:gosec // G204: the arguments are tmux's IDs, a checked name and escaped text (fact 22).
	cmd := exec.CommandContext(ctx, "tmux", slices.Concat(global, args)...) // NOSONAR

	out, err := cmd.Output()

	return string(out), failure(ctx, args[0], err)
}

// failure says why running tmux for the command failed, or returns nil if it didn't.
func failure(ctx context.Context, command string, err error) error {
	if err == nil {
		return nil
	}

	if cause := context.Cause(ctx); cause != nil {
		return fmt.Errorf("tmux %s: %w", command, cause)
	}

	if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
		return fmt.Errorf("tmux %s: %s", command, bytes.TrimSpace(exit.Stderr))
	}

	return fmt.Errorf("tmux %s: %w", command, err)
}

// line returns the first line that tmux printed: the command's own, which the user's hooks may
// follow with theirs (fact 22).
func line(out string) string {
	first, _, _ := strings.Cut(out, "\n")

	return first
}

// quote keeps tmux from taking an argument's last ";" for the end of a command (fact 22).
func quote(arg string) string {
	if rest, ok := strings.CutSuffix(arg, ";"); ok {
		return rest + `\;`
	}

	return arg
}

// text keeps tmux from expanding formats, such as #S or #(command), in an argument (fact 22).
func text(arg string) string {
	return quote(strings.ReplaceAll(arg, "#", "##"))
}
