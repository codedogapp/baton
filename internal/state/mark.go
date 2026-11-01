package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/codedogapp/baton/internal/flow"
)

// A role's wait mark is a shared flock on a file that baton never removes: <role>.wait in the
// feature's folder, or orchestrator.wait in baton's folder for the orchestrator, who waits in no
// feature. Each process that waits for the role's work holds it, and the kernel releases it when
// the process ends, so a wait that crashed leaves no mark (decision 47).
const markSuffix = ".wait"

// markFile returns the name of the role's wait mark in baton's folder: in the feature's folder,
// or in baton's folder itself for the orchestrator, whatever the slug.
func markFile(slug string, role flow.Role) (string, error) {
	name := string(role) + markSuffix
	if role == flow.RoleOrchestrator {
		return name, nil
	}

	if err := checkSlug(slug); err != nil {
		return "", err
	}

	return filepath.Join(folder(slug), name), nil
}

// MarkWaiting marks the role as waiting in the feature, or the orchestrator as waiting, until the
// caller calls the release that it returns. The mark's folder must exist.
func (s Store) MarkWaiting(slug string, role flow.Role) (func(), error) {
	name, err := markFile(slug, role)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("mark the %s waiting: %w", role, err)
	}
	defer func() { _ = root.Close() }()

	file, err := openLock(root, name)
	if err != nil {
		return nil, fmt.Errorf("mark the %s waiting: %w", role, err)
	}

	if err := flock(file, syscall.LOCK_SH); err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("mark the %s waiting: %w", role, err)
	}

	return func() { _ = file.Close() }, nil
}

// Waiting reports whether a process holds the role's wait mark in the feature, or the
// orchestrator's. It tries the mark's lock for itself, and so reports a role as
// waiting while another Waiting tries it too: a hook that asks then lets the copilot stop.
func (s Store) Waiting(slug string, role flow.Role) (bool, error) {
	name, err := markFile(slug, role)
	if err != nil {
		return false, err
	}

	file, err := os.OpenInRoot(s.Dir, name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("look for the %s's wait: %w", role, err)
	}

	defer func() { _ = file.Close() }()

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("look for the %s's wait: %w", role, err)
	}

	return false, nil
}
