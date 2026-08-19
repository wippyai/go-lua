package signatures

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func sealTable(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return table
}

// TestSignaturesPreserveTypedRelations proves every authored callable and
// assertion field survives the seal, including the three ordered columns.
func TestSignaturesPreserveTypedRelations(t *testing.T) {
	signatures := sealTable(t, ledgerInput(t)).View()
	function := term(keyspace.FamilyTypeFunction, 1)
	assertion := term(keyspace.FamilyTypeAsserts, 1)
	if scope, variadic, coordinate, known, ok := signatures.TypeFunctions().Get(function); !ok ||
		scope != term(keyspace.FamilyCell, 1) || variadic != primitive(4) ||
		coordinate == (source.Coordinate{}) || !known {
		t.Fatalf("function header = (%v, %v, %v, %v, %v)", scope, variadic, coordinate, known, ok)
	}
	if count, ok := signatures.TypeFunctions().TypeParamCount(function); !ok || count != 2 {
		t.Fatalf("type parameter count = (%d, %v)", count, ok)
	}
	if param, ok := signatures.TypeFunctions().TypeParamAt(function, 0); !ok || param != term(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("type parameter = (%v, %v)", param, ok)
	}
	if parameter, ok := signatures.TypeFunctions().ParameterAt(function, 0); !ok || parameter.Name != 11 ||
		parameter.NameCoordinate == (source.Coordinate{}) || parameter.Type != primitive(1) {
		t.Fatalf("fixed parameter = (%+v, %v)", parameter, ok)
	}
	// An unnamed formal carries neither a name nor a coordinate.
	if parameter, ok := signatures.TypeFunctions().ParameterAt(function, 2); !ok || parameter.Name != 0 ||
		parameter.NameCoordinate != (source.Coordinate{}) || parameter.Type != primitive(3) {
		t.Fatalf("unnamed parameter = (%+v, %v)", parameter, ok)
	}
	if result, ok := signatures.TypeFunctions().ReturnAt(function, 0); !ok || result != primitive(1) {
		t.Fatalf("return = (%v, %v)", result, ok)
	}
	if name, coordinate, bound, ordinal, narrow, ok := signatures.Assertions().Get(assertion); !ok || name != 11 ||
		coordinate == (source.Coordinate{}) || !bound || ordinal != 0 || narrow != primitive(4) {
		t.Fatalf("assertion = (%v, %v, %v, %d, %v, %v)", name, coordinate, bound, ordinal, narrow, ok)
	}
}

// TestSignaturesReturnsAndAssertionEncoding proves an omitted return clause and
// an explicit empty one are distinct authored facts, and that an unbound
// assertion cannot carry a parameter ordinal.
func TestSignaturesReturnsAndAssertionEncoding(t *testing.T) {
	omitted := ledgerInput(t)
	omitted.TypeFunction[0].Returns = nil
	omitted.TypeFunction[0].ReturnsKnown = false
	function := term(keyspace.FamilyTypeFunction, 1)
	if _, _, _, known, ok := sealTable(t, omitted).View().TypeFunctions().Get(function); !ok || known {
		t.Fatalf("omitted returns = (%v, %v)", known, ok)
	}

	empty := ledgerInput(t)
	empty.TypeFunction[0].Returns = nil
	empty.TypeFunction[0].ReturnsKnown = true
	if _, _, _, known, ok := sealTable(t, empty).View().TypeFunctions().Get(function); !ok || !known {
		t.Fatalf("explicit empty returns = (%v, %v)", known, ok)
	}

	unbound := ledgerInput(t)
	unbound.TypeAsserts[0].Bound = false
	unbound.TypeAsserts[0].Param = 1
	if _, err := Build(unbound, ledgerCounts()); err == nil {
		t.Fatal("Build() accepted an unbound assertion carrying a parameter ordinal")
	}
}

// TestSignaturesRejectXORScopeAndChildDefects proves the admissions this
// vertical owns. TypeParam ownership, the interface-method scope join, and the
// containment forest span other verticals and are the enclosing owner's laws.
func TestSignaturesRejectXORScopeAndChildDefects(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"anonymous parameter with coordinate", func(in *Input) { in.TypeFunction[0].Parameters[0].Name = 0 }},
		{"named parameter missing coordinate", func(in *Input) {
			in.TypeFunction[0].Parameters[0].NameCoordinate = source.Coordinate{}
		}},
		{"named parameter zero coordinate span", func(in *Input) {
			in.TypeFunction[0].Parameters[2].Name = 14
		}},
		{"nonstatic parameter type", func(in *Input) {
			in.TypeFunction[0].Parameters[0].Type = term(keyspace.FamilyCell, 1)
		}},
		{"variadic missing coordinate", func(in *Input) {
			in.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
		}},
		{"absent variadic with coordinate", func(in *Input) { in.TypeFunction[0].Variadic = 0 }},
		{"nonstatic variadic", func(in *Input) {
			in.TypeFunction[0].Variadic = term(keyspace.FamilyCell, 1)
		}},
		{"invalid static scope", func(in *Input) {
			in.TypeFunction[0].Scope = term(keyspace.FamilyBody, 1)
		}},
		{"foreign static scope", func(in *Input) {
			in.TypeFunction[0].Scope = term(keyspace.FamilyCell, 9)
		}},
		{"omitted returns with children", func(in *Input) { in.TypeFunction[0].ReturnsKnown = false }},
		{"nonstatic return", func(in *Input) {
			in.TypeFunction[0].Returns[0] = term(keyspace.FamilyCell, 1)
		}},
		{"assertion missing name", func(in *Input) { in.TypeAsserts[0].Name = 0 }},
		{"assertion missing coordinate", func(in *Input) {
			in.TypeAsserts[0].ParamCoordinate = source.Coordinate{}
		}},
		{"nonstatic assertion narrow", func(in *Input) {
			in.TypeAsserts[0].Narrow = term(keyspace.FamilyCell, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput(t)
			test.edit(&input)
			if _, err := Build(input, ledgerCounts()); err == nil {
				t.Fatal("Build() accepted an invalid signature relation")
			}
		})
	}
}

// TestSignatureCopyFencesBoundsAndQueriesDoNotAllocate proves the seal takes a
// copy of every column, each read is total, and hot queries allocate nothing.
func TestSignatureCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := ledgerInput(t)
	table := sealTable(t, input)
	input.TypeFunction[0].TypeParams[0] = 0
	input.TypeFunction[0].Parameters[0].Type = 0
	input.TypeFunction[0].Returns[0] = 0

	signatures := table.View()
	function := term(keyspace.FamilyTypeFunction, 1)
	assertion := term(keyspace.FamilyTypeAsserts, 1)
	if got, ok := signatures.TypeFunctions().TypeParamAt(function, 0); !ok || got == 0 {
		t.Fatalf("type parameter copy fence = (%v, %v)", got, ok)
	}
	if got, ok := signatures.TypeFunctions().ParameterAt(function, 0); !ok || got.Type == 0 {
		t.Fatalf("parameter copy fence = (%+v, %v)", got, ok)
	}
	if got, ok := signatures.TypeFunctions().ReturnAt(function, 0); !ok || got == 0 {
		t.Fatalf("return copy fence = (%v, %v)", got, ok)
	}
	if _, ok := signatures.TypeFunctions().ParameterAt(function, -1); ok {
		t.Fatal("ParameterAt accepted negative index")
	}
	if _, ok := signatures.TypeFunctions().ReturnAt(function, 2); ok {
		t.Fatal("ReturnAt accepted out-of-range index")
	}
	if _, _, _, _, ok := signatures.TypeFunctions().Get(term(keyspace.FamilyTypeFunction, 9)); ok {
		t.Fatal("TypeFunctions.Get accepted unknown term")
	}
	if _, _, _, _, ok := signatures.TypeFunctions().Get(assertion); ok {
		t.Fatal("TypeFunctions.Get accepted foreign family")
	}
	if _, ok := signatures.Assertions().At(2); ok {
		t.Fatal("Assertions.At accepted out-of-range index")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		signatures.TypeFunctions().Get(function)
		signatures.TypeFunctions().TypeParamAt(function, 0)
		signatures.TypeFunctions().ParameterAt(function, 0)
		signatures.TypeFunctions().ReturnAt(function, 0)
		signatures.Assertions().Get(assertion)
	}); allocations != 0 {
		t.Fatalf("signature queries allocated %.2f times", allocations)
	}
}

// TestDecoderRetainsFunctionAndAssertionRows proves the decoded rows map each
// wire field back to the relation it names.
func TestDecoderRetainsFunctionAndAssertionRows(t *testing.T) {
	decoded, err := Decode(sectionReader(t, sectionBytes(t, ledgerInput(t))))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.TypeFunction) != 2 || len(decoded.TypeAsserts) != 2 {
		t.Fatalf("decoded counts = (%d, %d), want (2, 2)", len(decoded.TypeFunction), len(decoded.TypeAsserts))
	}
	function := decoded.TypeFunction[0]
	if function.Scope != term(keyspace.FamilyCell, 1) || len(function.TypeParams) != 2 ||
		len(function.Parameters) != 3 || function.Parameters[0].Name != 11 ||
		function.Parameters[2].Name != 0 || function.Parameters[2].NameCoordinate != (source.Coordinate{}) ||
		function.Variadic != primitive(4) || !function.ReturnsKnown || len(function.Returns) != 2 {
		t.Fatalf("decoded type function = %+v", function)
	}
	assertion := decoded.TypeAsserts[0]
	if assertion.Name != 11 || !assertion.Bound || assertion.Param != 0 || assertion.Narrow != primitive(4) {
		t.Fatalf("decoded assertion = %+v", assertion)
	}
	if decoded.TypeAsserts[1].Bound || decoded.TypeAsserts[1].Narrow != 0 {
		t.Fatalf("decoded unbound assertion = %+v", decoded.TypeAsserts[1])
	}
}

// TestZeroViewsFailClosed proves an unavailable view answers nothing.
func TestZeroViewsFailClosed(t *testing.T) {
	var view View
	function := term(keyspace.FamilyTypeFunction, 1)
	if view.Available() || view.TypeFunctions().Count() != 0 || view.Assertions().Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, _, _, _, ok := view.TypeFunctions().Get(function); ok {
		t.Fatal("zero View returned a callable")
	}
	if _, ok := view.TypeFunctions().ParameterCount(function); ok {
		t.Fatal("zero View counted parameters")
	}
	if _, ok := view.TypeFunctions().ParameterAt(function, 0); ok {
		t.Fatal("zero View read a parameter")
	}
	if _, _, _, _, _, ok := view.Assertions().Get(term(keyspace.FamilyTypeAsserts, 1)); ok {
		t.Fatal("zero View returned an assertion")
	}
}
