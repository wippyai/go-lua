package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
)

func TestSubjectFlowPublishesFixedLocalStorageAliases(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{Name: "subject-flow-storage.lua", Text: []byte(`
local value = 1
value = 2
return value
`)})
	if err != nil {
		t.Fatal(err)
	}
	projection := program.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("SubjectFlow was unavailable for a local-storage fixture")
	}
	aliases := 0
	for index := 0; index < projection.EventCount(); index++ {
		event, ok := projection.EventAt(index)
		if !ok {
			t.Fatalf("EventAt(%d) failed", index)
		}
		if event.Kind != subjectflow.EventAlias {
			continue
		}
		aliases++
		valueToCell := event.Subject.Kind == subjectflow.SubjectValue && event.Related.Kind == subjectflow.SubjectCell
		cellToValue := event.Subject.Kind == subjectflow.SubjectCell && event.Related.Kind == subjectflow.SubjectValue
		if !valueToCell && !cellToValue ||
			!event.Subject.ID.Available() || !event.Related.ID.Available() || !event.Path.Available() {
			t.Fatalf("alias %d = %#v, want Value -> Cell with issued paths", aliases, event)
		}
	}
	if aliases == 0 {
		t.Fatal("fixed local bind/assign produced no exact Alias fact")
	}
}
