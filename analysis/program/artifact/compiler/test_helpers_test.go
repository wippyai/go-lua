package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func testCompileKey(t testing.TB, input *program.Program) programartifact.CompileKey {
	t.Helper()
	schema, schemaOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	key, keyOK := programartifact.NewCompileKey(input, schema)
	if !schemaOK || !keyOK {
		t.Fatal("canonical test CompileKey unavailable")
	}
	return key
}

func valuesLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}
