package gate_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/gate"
)

// TestRunRunsTheCommandInTheFolder runs a command line with sh in the folder, which shows both its
// output and its errors, in the order that it writes them.
func TestRunRunsTheCommandInTheFolder(t *testing.T) {
	dir := t.TempDir()

	var out bytes.Buffer
	if err := gate.Run(t.Context(), dir, `echo "in ${PWD##*/}"; echo err >&2; echo out`,
		&out); err != nil {
		t.Fatal(err)
	}

	if got, want := out.String(), "in "+filepath.Base(dir)+"\nerr\nout\n"; got != want {
		t.Errorf("the gate wrote %q; want %q", got, want)
	}
}

// TestRunReportsAFailure returns the command's exit status when it fails, after its output.
func TestRunReportsAFailure(t *testing.T) {
	var out bytes.Buffer

	err := gate.Run(t.Context(), t.TempDir(), "echo broken >&2; exit 3", &out)

	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 3 {
		t.Errorf("Run returned %v; want exit status 3", err)
	}

	if got := out.String(); got != "broken\n" {
		t.Errorf("the gate wrote %q; want %q", got, "broken\n")
	}
}

// TestRunEndsWithTheContext stops a command that runs on once the context ends.
func TestRunEndsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := gate.Run(ctx, t.TempDir(), "exec sleep 30", io.Discard)

	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("Run took %s", took)
	}

	if err == nil || err.Error() != "signal: killed" {
		t.Errorf("Run returned %v; want signal: killed", err)
	}
}
