package operators

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func operatorCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyRead] = 1
	counts[keyspace.FamilyTypePrimitive] = 4
	counts[keyspace.FamilyTypeOf] = 1
	counts[keyspace.FamilyTypeKeyOf] = 1
	counts[keyspace.FamilyTypeIndexAccess] = 1
	counts[keyspace.FamilyTypeConditional] = 1
	return counts
}

func operatorInput() Input {
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	return Input{
		TypeOf: []TypeOf{{
			Scope:   keyspace.MakeTerm(keyspace.FamilyCell, 1),
			Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1),
		}},
		KeyOf:       []KeyOf{{Inner: primitive(1)}},
		IndexAccess: []IndexAccess{{Object: primitive(2), Index: primitive(3)}},
		Conditional: []Conditional{{Check: primitive(1), Extends: primitive(2), Then: primitive(3), Else: primitive(4)}},
	}
}

func TestBuildSealsRowsAndQueriesByFamily(t *testing.T) {
	input := operatorInput()
	table, err := Build(input, operatorCounts())
	if err != nil {
		t.Fatal(err)
	}
	input.TypeOf[0].Scope = 0
	view := table.View()
	if !view.Available() || view.TypeOfs().Count() != 1 || view.KeyOfs().Count() != 1 ||
		view.IndexAccesses().Count() != 1 || view.Conditionals().Count() != 1 {
		t.Fatalf("sealed operator counts = %d/%d/%d/%d", view.TypeOfs().Count(), view.KeyOfs().Count(), view.IndexAccesses().Count(), view.Conditionals().Count())
	}
	if scope, operand, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("TypeOf row = %v/%v/%v", scope, operand, ok)
	}
	var edges int
	if !table.VisitContainment(func(_, _ keyspace.Term) bool { edges++; return true }) || edges != 7 {
		t.Fatalf("operator child edges = %d", edges)
	}
}

func TestCodecRoundTripUsesTheOwnerTable(t *testing.T) {
	table, err := Build(operatorInput(), operatorCounts())
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, "operators-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	reader, err := framing.NewReader(data.Bytes(), data.Len())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("operators-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := Scan(reader); err != nil {
		t.Fatal(err)
	}
	// Recreate the reader for the consuming decode after the allocation-free
	// preflight; Scan intentionally advances its input.
	reader, err = framing.NewReader(data.Bytes(), data.Len())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("operators-test", 1); err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(reader)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Build(decoded, operatorCounts())
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Count(keyspace.FamilyTypeOf) != table.Count(keyspace.FamilyTypeOf) ||
		sealed.Count(keyspace.FamilyTypeConditional) != table.Count(keyspace.FamilyTypeConditional) {
		t.Fatalf("decoded table counts differ: %d/%d", sealed.Count(keyspace.FamilyTypeOf), sealed.Count(keyspace.FamilyTypeConditional))
	}
}

func sealCounts() [keyspace.FamilyCount]uint32 {
	counts := operatorCounts()
	counts[keyspace.FamilyTypeOf] = 2
	return counts
}

// sealInput carries two TypeOf rows so the cross-owner sharing law has a
// second occurrence of the same Flow operand to observe.
func sealInput() Input {
	input := operatorInput()
	input.TypeOf = append(input.TypeOf, input.TypeOf[0])
	input.KeyOf[0] = KeyOf{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)}
	return input
}

func sealTable(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, sealCounts())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return table
}

// TestOperatorsPreserveTypedRelationsAndCrossOwnerLeaves proves each operator
// keeps its exact authored shape, and that a Flow value occurrence may appear
// in several TypeOf rows: it is a cross-owner leaf, not a concrete type child,
// so sharing it is not a defect.
func TestOperatorsPreserveTypedRelationsAndCrossOwnerLeaves(t *testing.T) {
	table := sealTable(t, sealInput())
	view := table.View()
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	if scope, operand, ok := view.TypeOfs().Get(typeOf); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("typeof relation = (%v, %v, %v)", scope, operand, ok)
	}
	if _, operand, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 2)); !ok ||
		operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("second typeof cross-owner operand = (%v, %v)", operand, ok)
	}
	if inner, ok := view.KeyOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)); !ok || inner != typeOf {
		t.Fatalf("keyof typed child = (%v, %v)", inner, ok)
	}
	if object, index, ok := view.IndexAccesses().Get(keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)); !ok ||
		object != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) ||
		index != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) {
		t.Fatalf("index-access relation = (%v, %v, %v)", object, index, ok)
	}
	if check, extends, then, otherwise, ok := view.Conditionals().Get(keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)); !ok ||
		check != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) || extends != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) ||
		then != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) || otherwise != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4) {
		t.Fatalf("conditional relation = (%v, %v, %v, %v, %v)", check, extends, then, otherwise, ok)
	}
}

// TestOperatorsRejectInvalidRelations proves the admissions this vertical
// owns. Sharing a concrete child and every cycle are combined-forest defects
// and belong to the enclosing owner's containment seal.
func TestOperatorsRejectInvalidRelations(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"invalid typeof scope family", func(in *Input) {
			in.TypeOf[0].Scope = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"foreign typeof scope", func(in *Input) {
			in.TypeOf[0].Scope = keyspace.MakeTerm(keyspace.FamilyCell, 9)
		}},
		{"zero typeof operand", func(in *Input) { in.TypeOf[0].Operand = 0 }},
		{"foreign typeof operand", func(in *Input) {
			in.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyRead, 9)
		}},
		{"non-flow typeof operand", func(in *Input) {
			in.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"nonstatic keyof child", func(in *Input) {
			in.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyRead, 1)
		}},
		{"nonstatic indexed object", func(in *Input) {
			in.IndexAccess[0].Object = keyspace.MakeTerm(keyspace.FamilyCell, 1)
		}},
		{"nonstatic conditional branch", func(in *Input) {
			in.Conditional[0].Else = keyspace.MakeTerm(keyspace.FamilyCell, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := sealInput()
			test.edit(&input)
			if _, err := Build(input, sealCounts()); err == nil {
				t.Fatal("Build() accepted an invalid operator relation")
			}
		})
	}
}

// TestOperatorsCopyFencesBoundsAndQueriesDoNotAllocate proves the seal takes a
// copy, every read is total, and the hot queries allocate nothing.
func TestOperatorsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := sealInput()
	table := sealTable(t, input)
	input.TypeOf[0].Operand = 0
	input.KeyOf[0].Inner = 0
	input.IndexAccess[0].Object = 0
	input.Conditional[0].Check = 0

	view := table.View()
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	keyOf := keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)
	indexAccess := keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)
	conditional := keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)
	if _, operand, ok := view.TypeOfs().Get(typeOf); !ok || operand == 0 {
		t.Fatalf("typeof copy fence = (%v, %v)", operand, ok)
	}
	if inner, ok := view.KeyOfs().Get(keyOf); !ok || inner == 0 {
		t.Fatalf("keyof copy fence = (%v, %v)", inner, ok)
	}
	if object, _, ok := view.IndexAccesses().Get(indexAccess); !ok || object == 0 {
		t.Fatalf("indexed access copy fence = (%v, %v)", object, ok)
	}
	if check, _, _, _, ok := view.Conditionals().Get(conditional); !ok || check == 0 {
		t.Fatalf("conditional copy fence = (%v, %v)", check, ok)
	}
	if _, ok := view.KeyOfs().At(-1); ok {
		t.Fatal("KeyOfs.At accepted negative index")
	}
	if _, _, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 3)); ok {
		t.Fatal("TypeOfs.Get accepted out-of-range term")
	}
	if _, ok := view.KeyOfs().Get(typeOf); ok {
		t.Fatal("KeyOfs.Get accepted foreign family")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		view.TypeOfs().Get(typeOf)
		view.KeyOfs().Get(keyOf)
		view.IndexAccesses().Get(indexAccess)
		view.Conditionals().Get(conditional)
	}); allocations != 0 {
		t.Fatalf("operator queries allocated %.2f times", allocations)
	}
}
