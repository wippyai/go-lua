package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAllocationPathsUseTheSealedFlowCertificate(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "allocation-paths.lua",
		Text: []byte("local function make() return {} end\nreturn make()"),
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := program.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	flow := program.Flow()
	path, ok := flow.BodyPath(entry)
	if !ok || !path.Available() {
		t.Fatalf("BodyPath(%v) = %v/%v, want the sealed entry path", entry, path, ok)
	}
	if got, ok := flow.SemanticTermPath(entry); !ok || got != path {
		t.Fatalf("SemanticTermPath(%v) = %v/%v, want BodyPath %v", entry, got, ok, path)
	}
	if _, ok := flow.BodyPath(keyspace.MakeTerm(keyspace.FamilyCall, 1)); ok {
		t.Fatal("BodyPath accepted a non-Body term")
	}
	if _, ok := flow.StorageAssignmentPath(keyspace.MakeTerm(keyspace.FamilyCall, 1)); ok {
		t.Fatal("StorageAssignmentPath accepted a non-assignment term")
	}
}
