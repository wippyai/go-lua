package typ_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// TestMayRuntimeKindsProjectsEveryDeclaredFormOntoTheRuntimeVocabulary states
// the projection this domain publishes for a type graph it has not sealed into
// a Class. A consumer holding a declared type asks this one question - which
// Lua runtime families may a value of this type carry - and every structural
// form answers it exactly rather than falling back to the whole vocabulary.
func TestMayRuntimeKindsProjectsEveryDeclaredFormOntoTheRuntimeVocabulary(t *testing.T) {
	function, functionOK := typ.BuiltinPrimitiveType("function")
	if !functionOK || function == nil {
		t.Fatal("builtin function type unavailable")
	}
	record := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "x", Type: typ.Number}}})
	if record == nil {
		t.Fatal("record construction unavailable")
	}
	table := runtimekind.Bit(runtimekind.Table)
	for _, testCase := range []struct {
		name    string
		subject typ.Type
		want    runtimekind.Set
	}{
		{"record", record, table},
		{"array", typ.NewArray(typ.String), table},
		{"map", typ.NewMap(typ.String, typ.Number), table},
		{"readonly map", typ.NewReadonlyMap(typ.String, typ.Number), table},
		{"function", function, runtimekind.Bit(runtimekind.Function)},
		{"optional record", typeexpr.Optional(record), table | runtimekind.Bit(runtimekind.Nil)},
		{"optional string", typeexpr.Optional(typ.String), runtimekind.Bit(runtimekind.String) | runtimekind.Bit(runtimekind.Nil)},
		{"union of scalars", typeexpr.Union(typ.String, typ.Number), runtimekind.Bit(runtimekind.String) | runtimekind.Bit(runtimekind.Number)},
		{"union with a record", typeexpr.Union(typ.String, record), runtimekind.Bit(runtimekind.String) | table},
		{"string", typ.String, runtimekind.Bit(runtimekind.String)},
		{"integer", typ.Integer, runtimekind.Bit(runtimekind.Number)},
		{"boolean", typ.Boolean, runtimekind.Bit(runtimekind.Boolean)},
		{"never", typ.Never, 0},
		{"any", typ.Any, runtimekind.All},
		{"unknown", typ.Unknown, runtimekind.All},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.subject == nil {
				t.Fatal("subject type unavailable")
			}
			kinds := typ.MayRuntimeKinds(testCase.subject)
			if !kinds.Valid() {
				t.Fatalf("projection produced a set outside the closed vocabulary: %d", kinds)
			}
			if kinds != testCase.want {
				t.Fatalf("MayRuntimeKinds = %d, want %d", kinds, testCase.want)
			}
		})
	}
}

// TestMayRuntimeKindsAnswersTheWholeVocabularyForAnAbsentType states the
// abstention: a caller with no type graph is answered with every family rather
// than with an empty set, which would read as a declaration admitting nothing.
func TestMayRuntimeKindsAnswersTheWholeVocabularyForAnAbsentType(t *testing.T) {
	if kinds := typ.MayRuntimeKinds(nil); kinds != runtimekind.All {
		t.Fatalf("MayRuntimeKinds(nil) = %d, want the whole vocabulary %d", kinds, runtimekind.All)
	}
}
