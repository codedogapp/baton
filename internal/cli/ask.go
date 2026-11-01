package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// errAsked means that the question is the role's last one already.
var errAsked = errors.New("asked already")

// ask asks the orchestrator the question of the pane's role, then waits for the answer and prints
// it (decisions 18 and 46). Only the holder asks, since only the holder works; the orchestrator
// asks the user instead. The role's last question isn't asked again: its ask waits for the answer
// that the first one waited for, so a role whose ask ended early gets the answer by asking again.
func ask(ctx context.Context, env Env, args []string) error {
	texts, err := parseTexts(newFlags(env, "ask"), args, `the question: baton ask "<question>"`)
	if err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	if role == flow.RoleOrchestrator {
		return errors.New("the roles ask the orchestrator; the orchestrator asks the user")
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	question := state.Question{From: role, Text: texts[0]}
	entry := state.Entry{
		Event: state.Event{Kind: state.EventQuestion, Role: role, Text: question.Text},
	}

	_, err = store.Update(slug, entry, asked(question))
	if err != nil && !errors.Is(err, errAsked) {
		return err
	}

	answer, err := awaitAnswer(ctx, store, slug, question)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(env.Stdout, "The orchestrator's answer:\n%s\n", answer)

	return err
}

// asked returns the change that makes the question the holder's last one. It fails with errAsked
// if the question is that one already, and refuses another while that one waits for its answer.
// Since the holder's handover clears its question, the last one is the asking holder's, if any.
func asked(question state.Question) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		switch last := feature.Question; {
		case feature.Flow.Holder != question.From:
			return fmt.Errorf("%s holds %s, and only the holder asks", feature.Flow.Holder,
				feature.Slug)
		case last.Text == question.Text:
			return errAsked
		case last.Open():
			return fmt.Errorf("your question %q waits for its answer: ask one question at a time",
				last.Text)
		default:
			feature.Question = question

			return nil
		}
	}
}

// awaitAnswer reads the feature every second, without its lock, until the question has its
// answer, and returns the answer. It fails if another question has taken the question's place, or
// if the holder's handover has cleared it before this ask read the answer, as when another ask of
// the question read it first.
func awaitAnswer(ctx context.Context, store state.Store, slug string, question state.Question) (
	string, error,
) {
	tick := time.Tick(poll)

	for {
		feature, err := store.Read(slug)
		if err != nil {
			return "", err
		}

		switch last := feature.Question; {
		case last.Text == "":
			return "", fmt.Errorf("%s was handed over before this ask read the answer", slug)
		case last.From != question.From || last.Text != question.Text:
			return "", fmt.Errorf("the %s's question %q has taken the place of yours", last.From,
				last.Text)
		case last.Answer != "":
			return last.Answer, nil
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("stopped waiting: %w", context.Cause(ctx))
		case <-tick:
		}
	}
}

// answer answers the open question of a feature's holder, in the orchestrator's pane; the
// holder's baton ask prints the answer (decision 46).
func answer(ctx context.Context, env Env, args []string) error {
	texts, err := parseTexts(newFlags(env, "answer"), args,
		`the feature's slug: baton answer <slug> "<answer>"`,
		`the answer: baton answer <slug> "<answer>"`)
	if err != nil {
		return err
	}

	role, _, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	if role != flow.RoleOrchestrator {
		return fmt.Errorf("only the orchestrator answers: the %s asks with baton ask", role)
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	slug, text := texts[0], texts[1]
	entry := state.Entry{Event: state.Event{Kind: state.EventAnswer, Role: role, Text: text}}

	feature, err := store.Update(slug, entry, answered(text))
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(env.Stdout, "Answered the %s's question in %s.\n",
		feature.Question.From, slug)

	return err
}

// answered returns the change that answers the feature's open question.
func answered(text string) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		if !feature.Question.Open() {
			return fmt.Errorf("no question waits for its answer in %s", feature.Slug)
		}

		feature.Question.Answer = text

		return nil
	}
}
