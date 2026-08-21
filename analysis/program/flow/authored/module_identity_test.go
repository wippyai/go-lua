package authored

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestModuleIDIsIndependentAuthoredImportIdentity(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	request := keyspace.MakeTerm(keyspace.FamilyString, 1)
	row := Import{
		Term: keyspace.MakeTerm(keyspace.FamilyImport, 1),
		Call: call, Request: request,
	}
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyCall] = 1
	counts[keyspace.FamilyImport] = 1
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyNil] = 1
	flowDraft, err := Build(Input{
		Counts:  counts,
		Imports: []Import{row},
		Values:  ValuesInput{Rows: []Value{{Owner: body}}},
		Calls:   []Call{{Owner: body, Callee: keyspace.MakeTerm(keyspace.FamilyNil, 1), Actuals: values}},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()

	authoredView := flowFinalizer.View()
	if !authoredView.ModuleID().Available() {
		t.Fatal("authored ModuleID is unavailable")
	}
	if authoredView.ContentID() == authoredView.ModuleID() {
		t.Fatal("Flow ContentID unexpectedly collapsed into ModuleID")
	}
	noImportCounts := counts
	noImportCounts[keyspace.FamilyImport] = 0
	noImportDraft, err := Build(Input{
		Counts: noImportCounts,
		Values: ValuesInput{Rows: []Value{{Owner: body}}},
		Calls:  []Call{{Owner: body, Callee: keyspace.MakeTerm(keyspace.FamilyNil, 1), Actuals: values}},
	})
	if err != nil {
		t.Fatalf("authored.Build without imports: %v", err)
	}
	noImportFinalizer, err := noImportDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer without imports: %v", err)
	}
	defer func() { _ = noImportFinalizer.Abort() }()
	if authoredView.ContentID() != noImportFinalizer.View().ContentID() {
		t.Fatal("Flow ContentID changed when authored imports were added")
	}
	if authoredView.ModuleID() == noImportFinalizer.View().ModuleID() {
		t.Fatal("ModuleID did not change when authored imports were added")
	}
}
