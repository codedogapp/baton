package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The tests of a question's end: the holder's handover clears it, since it belongs to the holder's
// turn (decision 46), and so does the end of its feature.

// TestAskInTheNextTurnAsksAnew has the lead hand over once its question has its answer, which
// clears the question, and ask the same one in its next turn: a new question, which waits for its
// own answer.
func TestAskInTheNextTurnAsksAnew(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())

		asked := make(chan result, 1)
		go func() { asked <- leadAsks(t, mainDir, question) }()

		synctest.Wait()
		answer(t, mainDir, "demo.txt")
		<-asked

		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)
		wantQuestion(t, store, state.Question{})
		handOver(t, mainDir, flow.RoleEngineer, flow.VerdictNone)
		handOver(t, mainDir, flow.RoleQA, flow.VerdictApprove)
		handOver(t, mainDir, flow.RoleReviewer, flow.VerdictApprove)

		if got := askAMinute(t, mainDir, question); got != (result{code: 1, stderr: stopped}) {
			t.Errorf("the lead's question in its next turn: got %+v; want it to wait", got)
		}

		wantQuestion(t, store, state.Question{From: flow.RoleLead, Text: question})
	})
}

// TestAskEndsWithItsTurn has the lead hand over as soon as its answer comes, as another ask of
// the question that read the answer first would let it: the ask that didn't read it fails, rather
// than wait for an answer that's gone.
func TestAskEndsWithItsTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, asked := answeredThenHandedOver(t)

		want := result{
			code: 1, stderr: "baton ask: demo-1 was handed over before this ask read the answer\n",
		}
		if got := <-asked; got != want {
			t.Errorf("got %+v; want %+v", got, want)
		}
	})
}

// TestAskEndsWithTheNextTurnsQuestion has the engineer, in its turn, ask the lead's question
// before the lead's ask read its answer: the lead's ask fails, rather than print the answer to the
// engineer's question.
func TestAskEndsWithTheNextTurnsQuestion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, asked := answeredThenHandedOver(t)

		next := make(chan result, 1)
		go func() { next <- askAMinuteAs(t, mainDir, flow.RoleEngineer, question) }()

		synctest.Wait()

		want := result{code: 1, stderr: "baton ask: the engineer's question \"Which file?\" " +
			"has taken the place of yours\n"}
		if got := <-asked; got != want {
			t.Errorf("the lead's ask: got %+v; want %+v", got, want)
		}

		if got := <-next; got != (result{code: 1, stderr: stopped}) {
			t.Errorf("the engineer's ask: got %+v; want it to wait", got)
		}
	})
}

// TestAskEndsWhenItsFeatureGoes fails the lead's ask once its feature is gone, in place of waiting
// for an answer that can't come.
func TestAskEndsWhenItsFeatureGoes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())

		asked := make(chan result, 1)
		go func() { asked <- leadAsks(t, mainDir, question) }()

		synctest.Wait()

		if err := os.RemoveAll(filepath.Join(store.Dir, "features", slug)); err != nil {
			t.Fatal(err)
		}

		want := result{code: 1, stderr: "baton ask: no feature demo-1\n"}
		if got := <-asked; got != want {
			t.Errorf("got %+v; want %+v", got, want)
		}
	})
}

// answeredThenHandedOver starts demo-1, has the lead ask its question and the orchestrator answer
// it, then has the lead hand over before its ask read the answer. It returns the main worktree,
// and the lead's ask, which ends when it next reads the feature.
func answeredThenHandedOver(t *testing.T) (string, <-chan result) {
	t.Helper()

	mainDir, _ := repos(t)
	started(t, mainDir, fresh())

	asked := make(chan result, 1)
	go func() { asked <- leadAsks(t, mainDir, question) }()

	synctest.Wait()
	answer(t, mainDir, "demo.txt")
	handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

	return mainDir, asked
}
