package application

import (
	"testing"

	"github.com/shanejonas/cyclo/domain"
)

func TestWaitForChangeAnswersWhenTheRevisionAdvances(t *testing.T) {
	model := waitTestModel()
	reply := make(chan controlReply, 1)
	next, timeout := model.Update(controlCommand{
		action:        waitForChangeAction,
		afterRevision: model.revision,
		timeoutMs:     1_000,
		reply:         reply,
	})
	model = next.(Model)
	if timeout == nil {
		t.Fatal("wait has no timeout command")
	}

	focusReply := make(chan controlReply, 1)
	next, _ = model.Update(controlCommand{action: setFocusAction, pane: functionsPane, reply: focusReply})
	model = next.(Model)
	state := (<-reply).result.(ControlState)
	if state.Revision != 5 || state.Focus != "functions" {
		t.Fatalf("wait state = %+v", state)
	}
	if len(model.changeWaiters) != 0 {
		t.Fatalf("change waiters = %d, want 0", len(model.changeWaiters))
	}
}

func TestWaitForChangeAnswersAtTimeout(t *testing.T) {
	model := waitTestModel()
	reply := make(chan controlReply, 1)
	next, timeout := model.Update(controlCommand{
		action:        waitForChangeAction,
		afterRevision: model.revision,
		timeoutMs:     1,
		reply:         reply,
	})
	model = next.(Model)

	next, _ = model.Update(timeout())
	model = next.(Model)
	state := (<-reply).result.(ControlState)
	if state.Revision != 4 {
		t.Fatalf("timeout revision = %d, want 4", state.Revision)
	}
	if len(model.changeWaiters) != 0 {
		t.Fatalf("change waiters = %d, want 0", len(model.changeWaiters))
	}
}

func waitTestModel() Model {
	return Model{
		revision: 4,
		report: domain.Report{Files: []domain.File{{
			Path: "sample.go",
			Functions: []domain.Function{{
				Name: "Sample", Source: "func Sample() {}",
			}},
		}}},
	}
}
