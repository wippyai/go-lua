package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSubjectFlowKeepsNestedValuesInAggregatePlane(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{Name: "subject-flow-nested-values.lua", Text: []byte(`
type Row = { id: string, subject: string, unread: boolean }

local function count_unread(rows: {Row}): number
    local n = 0
    for _, r in ipairs(rows) do
        if r.unread then n = n + 1 end
    end
    return n
end

return count_unread
`)})
	if err != nil {
		t.Fatal(err)
	}
	projection := program.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("SubjectFlow was unavailable for a nested-Values fixture")
	}

	nestedValues := 0
	check := func(label string, subject subjectflow.Subject) {
		if subject.Term == 0 {
			return
		}
		var want subjectflow.SubjectKind
		switch keyspace.TermFamily(subject.Term) {
		case keyspace.FamilyBody:
			want = subjectflow.SubjectRoot
		case keyspace.FamilyCell:
			want = subjectflow.SubjectCell
		case keyspace.FamilyValues:
			want = subjectflow.SubjectValues
			nestedValues++
		default:
			want = subjectflow.SubjectValue
		}
		if subject.Kind != want {
			t.Fatalf("%s term %v has subject kind %v, want %v", label, subject.Term, subject.Kind, want)
		}
	}
	for index := 0; index < projection.EventCount(); index++ {
		event, ok := projection.EventAt(index)
		if !ok {
			t.Fatalf("EventAt(%d) failed", index)
		}
		check("event subject", event.Subject)
		check("event related", event.Related)
	}
	for index := 0; index < projection.LivenessCount(); index++ {
		row, ok := projection.LivenessAt(index)
		if !ok {
			t.Fatalf("LivenessAt(%d) failed", index)
		}
		check("liveness subject", row.Subject)
	}
	if nestedValues == 0 {
		t.Fatal("fixture did not publish a nested Values subject")
	}
}
