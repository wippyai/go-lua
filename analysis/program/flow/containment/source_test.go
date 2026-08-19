package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestProveRejectsWrongModuleImportCallForeignKey(t *testing.T) {
	input := imports.Input{Imports: []imports.Import{{
		Term: keyspace.MakeTerm(keyspace.FamilyImport, 1),
		Call: keyspace.MakeTerm(keyspace.FamilyFunction, 1),
	}}}
	if _, err := imports.Build(input); err == nil {
		t.Fatal("imports.Build accepted an Import whose Call foreign key is not a Call")
	}
}
