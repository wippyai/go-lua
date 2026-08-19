package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestRuntimeKindNamesUsesTheSealedVocabulary verifies the semantic result
// of the Value projection through a real mounted schema. The expected names
// are read from the same sealed structural table that fed Value sealing; no
// runtime-kind strings are restated by this test or by the Value domain.
func TestRuntimeKindNamesUsesTheSealedVocabulary(t *testing.T) {
	fixture := allocationMembershipFixtureFor(t, "runtime_kind_names")
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}

	all, allOK := fixture.values.RuntimeKindNames(fixture.values.Top())
	if !allOK {
		t.Fatal("runtime-kind projection of Top")
	}
	atoms, atomsOK := fixture.values.Atoms(all)
	if !atomsOK || len(atoms) != structural.Count(structure.CategoryRuntimeKind) {
		t.Fatalf("runtime-kind names atoms = %d, want %d", len(atoms), structural.Count(structure.CategoryRuntimeKind))
	}

	seen := make(map[string]bool, len(atoms))
	for _, atom := range atoms {
		scalar, scalarOK := fixture.values.ExactScalar(mustSingleton(t, fixture.values, atom))
		if !scalarOK {
			t.Fatal("runtime-kind result atom is not an exact scalar")
		}
		literal, literalOK := scalar.Literal()
		if !literalOK || literal.Kind != keyspace.LiteralString {
			t.Fatalf("runtime-kind result literal = %#v, want string", literal)
		}
		if _, keyed := atom.ExactKey(); keyed {
			t.Fatal("computed runtime-kind result acquired authored key identity")
		}
		seen[literal.String] = true
	}
	for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
		entry, entryOK := structural.At(structure.CategoryRuntimeKind, uint16(kind))
		if !entryOK || !seen[entry.Spelling()] {
			t.Fatalf("sealed runtime-kind spelling %q missing from Value result", entry.Spelling())
		}
	}

	bottom, bottomOK := fixture.values.RuntimeKindNames(fixture.values.Bottom())
	if !bottomOK || !bottom.IsBottom() {
		t.Fatal("runtime-kind projection of Bottom did not remain Bottom")
	}
	knownTable, knownOK := fixture.first.Fresh()
	if !knownOK {
		t.Fatal("known table Value")
	}
	tableNames, tableNamesOK := fixture.values.RuntimeKindNames(knownTable)
	if !tableNamesOK {
		t.Fatal("runtime-kind projection of known table")
	}
	if fixture.values.RuntimeKinds(tableNames) != runtimekind.Bit(runtimekind.String) {
		t.Fatalf("runtime-kind result Value has kinds %#x, want String", fixture.values.RuntimeKinds(tableNames))
	}
}

func mustSingleton(t *testing.T, schema *valuedomain.Schema, atom valuedomain.Atom) valuedomain.Value {
	t.Helper()
	value, valueOK := schema.Singleton(atom)
	if !valueOK {
		t.Fatal("runtime-kind atom does not belong to fixture schema")
	}
	return value
}
