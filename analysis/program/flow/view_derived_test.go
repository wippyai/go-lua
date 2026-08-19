package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestDerivedOutcomeQueriesResolveAuthoredReturn(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "derived-outcome.lua",
		Text: []byte("return 1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	view := program.Flow()
	bodies := view.Body()
	entry, ok := bodies.Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	start, end, ok := view.Outcomes().BodyRange(entry)
	if !ok || end <= start {
		t.Fatalf("BodyRange(%v) = %d/%d/%v, want the entry outcome range", entry, start, end, ok)
	}
	for index := start; index < end; index++ {
		term, ok := view.Outcomes().At(index)
		if !ok {
			t.Fatalf("Outcomes.At(%d) failed inside the body range", index)
		}
		outcome, ok := view.Outcomes().Get(term)
		if !ok || outcome.Body != entry {
			t.Fatalf("Outcomes.Get(%v) = %#v/%v, want entry-owned outcome", term, outcome, ok)
		}
	}
}

func TestExecutableRootRowsAreIssuedByFlowInSourceOrder(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "executable-root-rows.lua",
		Text: []byte("return 1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	bodies := program.Flow().Body()
	entry, ok := bodies.Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	authored, ok := bodies.RootAt(entry, 0)
	if !ok || !program.Flow().Executable().Contains(authored) {
		t.Fatal("fixture did not publish an executable Source root")
	}
	roots := program.Flow().Executable()
	rootCount, rootCountOK := roots.RootCount(entry)
	if !rootCountOK || rootCount != 1 {
		t.Fatalf("RootCount(%v) = %d/%v, want one dense executable root", entry, rootCount, rootCountOK)
	}
	id, family, ok := roots.RootAt(entry, 0)
	wantID, wantIDOK := program.Flow().SemanticTermPath(authored)
	if !ok || !wantIDOK || id != wantID || family != keyspace.TermFamily(authored) {
		t.Fatalf("RootAt(%v,0) = %v/%d/%v, want %v/%d/true", entry, id, family, ok, wantID, keyspace.TermFamily(authored))
	}
	if _, _, ok := roots.RootAt(entry, 1); ok {
		t.Fatal("RootAt accepted an out-of-range dense root")
	}
}
