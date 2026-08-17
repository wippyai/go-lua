package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func TestSourceRowsKeepBodyFillAndReservedImportState(t *testing.T) {
	rows := New(1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	stringTerm := keyspace.MakeTerm(keyspace.FamilyString, 1)
	rows.AddBody(body)
	rows.AddString(body, "module")
	if !rows.SetBody(body, []keyspace.Term{stringTerm}) || !rows.SetEntry(body) {
		t.Fatal("Source rows rejected a one-shot Body/Entry fill")
	}
	if !rows.FillImport(1, programsource.Span{File: "source.lua"}) || !rows.ImportComplete() {
		t.Fatal("Source rows did not complete its reserved Import marker")
	}
	if got, ok := rows.BodyAt(0); !ok || len(got.Terms) != 1 || got.Terms[0] != stringTerm {
		t.Fatalf("BodyAt = %#v/%t", got, ok)
	}
}
