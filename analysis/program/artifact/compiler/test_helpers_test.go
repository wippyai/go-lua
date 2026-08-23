package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/seal"
)

type compilerTestEmptySurface struct{ kind schema.SurfaceKind }

func (surface compilerTestEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (compilerTestEmptySurface) Entries() []schema.Entry          { return nil }
func (compilerTestEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func testCompileKey(t testing.TB, input *program.Program) programartifact.CompileKey {
	t.Helper()
	schema, schemaOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	key, keyOK := programartifact.NewCompileKey(input, schema)
	if !schemaOK || !keyOK {
		t.Fatal("canonical test CompileKey unavailable")
	}
	return key
}

func testIssuancePlan(t testing.TB) schemaissuance.Plan {
	t.Helper()
	entries, entriesOK := programissuance.Entries()
	if !entriesOK {
		t.Fatal("Program issuance declarations unavailable")
	}
	builder := seal.NewBuilder()
	builder.Register(compilerTestEmptySurface{schema.SurfaceKindStructure})
	builder.Register(compilerTestEmptySurface{schema.SurfaceKindAxis})
	builder.Register(schemaissuance.NewSurface(entries))
	for kind := schema.SurfaceKindRule; kind <= schema.SurfaceKindObservation; kind++ {
		builder.Register(compilerTestEmptySurface{kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("Program issuance schema refused: %+v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := schemaissuance.NewTable(view)
	plan, planOK := schemaissuance.NewPlan(table, nil)
	if !viewOK || !tableOK || !planOK {
		t.Fatal("Program issuance plan unavailable")
	}
	return plan
}

func valuesLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}
