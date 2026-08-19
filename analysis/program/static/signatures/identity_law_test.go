package signatures

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sectionDomain  = "program/static/signatures-law"
	sectionVersion = 1
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyCell] = 2
	counts[keyspace.FamilyTypeInterface] = 1
	counts[keyspace.FamilyTypePrimitive] = 4
	counts[keyspace.FamilyTypeParam] = 2
	counts[keyspace.FamilyTypeFunction] = 2
	counts[keyspace.FamilyTypeAsserts] = 2
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func primitive(ordinal uint32) keyspace.Term { return term(keyspace.FamilyTypePrimitive, ordinal) }

func coordinate(t *testing.T, startLine, startColumn, endLine, endColumn uint32) source.Coordinate {
	t.Helper()
	value, ok := source.CoordinateFromParts(startLine, startColumn, endLine, endColumn)
	if !ok {
		t.Fatal("CoordinateFromParts rejected the ledger fixture")
	}
	return value
}

// ledgerInput carries one fully populated callable, one minimal callable, and
// both assertion binding forms, so every column and every scalar is exercised.
func ledgerInput(t *testing.T) Input {
	t.Helper()
	first := coordinate(t, 1, 1, 1, 2)
	second := coordinate(t, 2, 3, 2, 7)
	return Input{
		TypeFunction: []TypeFunction{
			{
				Scope:      term(keyspace.FamilyCell, 1),
				TypeParams: []keyspace.Term{term(keyspace.FamilyTypeParam, 1), term(keyspace.FamilyTypeParam, 2)},
				Parameters: []Parameter{
					{Name: 11, NameCoordinate: first, Type: primitive(1)},
					{Name: 12, NameCoordinate: second, Type: primitive(2)},
					{Type: primitive(3)},
				},
				Variadic:           primitive(4),
				VariadicCoordinate: second,
				ReturnsKnown:       true,
				Returns:            []keyspace.Term{primitive(1), primitive(2)},
			},
			{Scope: term(keyspace.FamilyCell, 2)},
		},
		TypeAsserts: []TypeAsserts{
			{Name: 11, ParamCoordinate: first, Bound: true, Param: 0, Narrow: primitive(4)},
			{Name: 13, ParamCoordinate: second},
		},
	}
}

func sectionBytes(t *testing.T, input Input) []byte {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func sectionReader(t *testing.T, data []byte) *framing.Reader {
	t.Helper()
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

// TestAuthoredDistinctionsReachTheSection proves the section byte stream, which
// is the same schema the Static ContentID digests, separates every authored
// field, arity, and order distinction this vertical retains.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*testing.T, *Input)
	}{
		{"signature.scope", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Scope = term(keyspace.FamilyCell, 2)
		}},
		{"signature.type-param", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].TypeParams[0] = term(keyspace.FamilyTypeParam, 2)
		}},
		{"signature.type-param-arity", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].TypeParams = in.TypeFunction[0].TypeParams[:1]
		}},
		{"signature.type-param-order", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].TypeParams[0], in.TypeFunction[0].TypeParams[1] =
				in.TypeFunction[0].TypeParams[1], in.TypeFunction[0].TypeParams[0]
		}},
		{"signature.parameter.name", func(t *testing.T, in *Input) {
			in.TypeFunction[0].Parameters[0].Name = 77
		}},
		{"signature.parameter.coordinate", func(t *testing.T, in *Input) {
			in.TypeFunction[0].Parameters[0].NameCoordinate = coordinate(t, 4, 4, 4, 9)
		}},
		{"signature.parameter.type", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Parameters[0].Type = primitive(3)
		}},
		// An absent parameter name and its absent coordinate move together.
		{"signature.parameter.unnamed", func(t *testing.T, in *Input) {
			in.TypeFunction[0].Parameters[2] = Parameter{Name: 14, NameCoordinate: coordinate(t, 1, 1, 1, 2), Type: primitive(3)}
		}},
		{"signature.parameter-arity", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Parameters = in.TypeFunction[0].Parameters[:2]
		}},
		{"signature.parameter-order", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Parameters[0], in.TypeFunction[0].Parameters[1] =
				in.TypeFunction[0].Parameters[1], in.TypeFunction[0].Parameters[0]
		}},
		// The variadic tail and its coordinate are a present/absent pair.
		{"signature.variadic", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Variadic = primitive(1)
		}},
		{"signature.variadic-coordinate", func(t *testing.T, in *Input) {
			in.TypeFunction[0].VariadicCoordinate = coordinate(t, 4, 4, 4, 9)
		}},
		{"signature.variadic-absent", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Variadic = 0
			in.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
		}},
		{"signature.returns-known", func(_ *testing.T, in *Input) {
			in.TypeFunction[1].ReturnsKnown = true
		}},
		{"signature.return", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Returns[0] = primitive(3)
		}},
		{"signature.return-arity", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Returns = in.TypeFunction[0].Returns[:1]
		}},
		{"signature.return-order", func(_ *testing.T, in *Input) {
			in.TypeFunction[0].Returns[0], in.TypeFunction[0].Returns[1] =
				in.TypeFunction[0].Returns[1], in.TypeFunction[0].Returns[0]
		}},
		{"signature.row-order", func(_ *testing.T, in *Input) {
			in.TypeFunction[0], in.TypeFunction[1] = in.TypeFunction[1], in.TypeFunction[0]
		}},

		{"assertion.name", func(_ *testing.T, in *Input) { in.TypeAsserts[0].Name = 77 }},
		{"assertion.coordinate", func(t *testing.T, in *Input) {
			in.TypeAsserts[0].ParamCoordinate = coordinate(t, 4, 4, 4, 9)
		}},
		{"assertion.bound", func(_ *testing.T, in *Input) {
			in.TypeAsserts[0].Bound = false
			in.TypeAsserts[0].Param = 0
		}},
		{"assertion.param", func(_ *testing.T, in *Input) { in.TypeAsserts[0].Param = 1 }},
		{"assertion.narrow", func(_ *testing.T, in *Input) { in.TypeAsserts[0].Narrow = 0 }},
		{"assertion.row-order", func(_ *testing.T, in *Input) {
			in.TypeAsserts[0], in.TypeAsserts[1] = in.TypeAsserts[1], in.TypeAsserts[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := sectionBytes(t, ledgerInput(t))
			perturbed := ledgerInput(t)
			test.perturb(t, &perturbed)
			if bytes.Equal(base, sectionBytes(t, perturbed)) {
				t.Fatal("authored distinction is absent from the section stream")
			}
		})
	}
}

// TestSectionRoundTripPreservesEveryAuthoredRow proves the section decoder
// recovers exactly the authored input the writer emitted.
func TestSectionRoundTripPreservesEveryAuthoredRow(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput(t))
	reader := sectionReader(t, encoded)
	decoded, err := Decode(reader)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(encoded, sectionBytes(t, decoded)) {
		t.Fatal("round-tripped input did not reproduce the section stream")
	}
}

// TestScanValidatesWithoutRetainingRows proves the preflight half consumes the
// same stream shape as Decode.
func TestScanValidatesWithoutRetainingRows(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput(t))
	reader := sectionReader(t, encoded)
	if err := Scan(reader); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Scan left the stream unconsumed: %v", err)
	}
}

// TestBindsFormalIsTheBinderLastNameRule proves the published binder rule: a
// bound assertion selects a named formal at that exact index, and that formal
// must be the last one carrying the name, so the joint bound-assertion law
// never re-derives a callable's own scoping.
func TestBindsFormalIsTheBinderLastNameRule(t *testing.T) {
	input := ledgerInput(t)
	// A later formal repeats the first one's name, so index 0 no longer binds.
	input.TypeFunction[0].Parameters[1].Name = 11
	shadowed, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	function := term(keyspace.FamilyTypeFunction, 1)
	if shadowed.BindsFormal(function, 0, 11) {
		t.Fatal("a shadowed formal was admitted as the binder")
	}
	if !shadowed.BindsFormal(function, 1, 11) {
		t.Fatal("the last formal carrying the name did not bind")
	}

	table, err := Build(ledgerInput(t), ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !table.BindsFormal(function, 0, 11) {
		t.Fatal("the authored binder formal did not bind")
	}
	for _, test := range []struct {
		name     string
		function keyspace.Term
		param    uint32
		formal   keyspace.Key
	}{
		{name: "wrong name", function: function, param: 0, formal: 12},
		{name: "unnamed formal", function: function, param: 2, formal: 0},
		{name: "past arity", function: function, param: 9, formal: 11},
		{name: "foreign function", function: term(keyspace.FamilyFunction, 1), param: 0, formal: 11},
		{name: "absent function", function: term(keyspace.FamilyTypeFunction, 9), param: 0, formal: 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			if table.BindsFormal(test.function, test.param, test.formal) {
				t.Fatal("BindsFormal admitted a formal it does not name")
			}
		})
	}
}

// TestScopeIsPublishedForTheInterfaceMethodLaw proves the column the joint
// interface-method scope law consumes resolves exactly the sealed callables.
func TestScopeIsPublishedForTheInterfaceMethodLaw(t *testing.T) {
	table, err := Build(ledgerInput(t), ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	scope, ok := table.Scope(term(keyspace.FamilyTypeFunction, 1))
	if !ok || scope != term(keyspace.FamilyCell, 1) {
		t.Fatalf("Scope = %d/%v, want the authored scope", scope, ok)
	}
	if scope, ok := table.Scope(term(keyspace.FamilyTypeFunction, 9)); ok || scope != 0 {
		t.Fatalf("Scope admitted an absent callable: %d/%v", scope, ok)
	}
	if scope, ok := table.Scope(term(keyspace.FamilyFunction, 1)); ok || scope != 0 {
		t.Fatalf("Scope admitted a foreign-family term: %d/%v", scope, ok)
	}
}
