package state_test

import (
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestUpdateKeepsTheQuestionAndLogsItsText has the lead ask a question and the orchestrator
// answer it: the feature's file keeps both at its top, and each event keeps its text.
func TestUpdateKeepsTheQuestionAndLogsItsText(t *testing.T) {
	store := created(t)
	question := state.Question{From: flow.RoleLead, Text: "Which file?"}
	changes := []struct {
		event  state.Event
		change func(*state.Question)
	}{
		{
			state.Event{Kind: state.EventQuestion, Role: flow.RoleLead, Text: "Which file?"},
			func(asked *state.Question) { *asked = question },
		},
		{
			state.Event{Kind: state.EventAnswer, Role: flow.RoleOrchestrator, Text: "demo.txt"},
			func(asked *state.Question) { asked.Picked, asked.Answer = true, "demo.txt" },
		},
	}

	for _, step := range changes {
		_, err := store.Update(slug, state.Entry{Event: step.event}, func(f *state.Feature) error {
			step.change(&f.Question)

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	want := `  "question": {
    "from": "lead",
    "text": "Which file?",
    "picked": true,
    "answer": "demo.txt"
  }
}
`
	if got := read(t, featureFile(store)); !strings.HasSuffix(got, want) {
		t.Errorf("the feature file holds\n%s\nwant it to end with\n%s", got, want)
	}

	question.Picked, question.Answer = true, "demo.txt"
	if got, err := store.Read(slug); err != nil || got.Question != question {
		t.Errorf("Read = %+v, %v; want the question %+v", got.Question, err, question)
	}

	lines := strings.Split(read(t, eventsFile(store)), "\n")
	for i, want := range []string{
		`"event":"question","role":"lead","text":"Which file?","flow"`,
		`"event":"answer","role":"orchestrator","text":"demo.txt","flow"`,
	} {
		if !strings.Contains(lines[i+1], want) {
			t.Errorf("event %d is %s, want it to hold %s", i+1, lines[i+1], want)
		}
	}
}

func TestAQuestionIsOpenUntilItsAnswerComes(t *testing.T) {
	questions := map[state.Question]bool{
		{}:                             false,
		{From: flow.RoleQA}:            false,
		{Text: "Which file?"}:          true,
		{Text: "Which?", Picked: true}: true,
		{Text: "Which file?", Answer: "demo.txt"}: false,
	}

	for question, want := range questions {
		if got := question.Open(); got != want {
			t.Errorf("%+v.Open() = %t, want %t", question, got, want)
		}
	}
}
