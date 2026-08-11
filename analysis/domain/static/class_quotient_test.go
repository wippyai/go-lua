package static

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestClassSetExtensionalCoverageDoesNotEquateMutualEdges(t *testing.T) {
	tableTop := typ.BuiltinTableTopMarker()
	empty := typ.NewInterface("Empty", nil)
	set, runtime, classes := sealExtensionalClassFixture(t, []typ.Type{
		typ.Nil, tableTop, empty, typ.Any,
	})
	tableTopInner, emptyInner := set.rows[classes[1].index].inner, set.rows[classes[2].index].inner
	left, leftDecided := runtime.Subtype(tableTopInner, emptyInner)
	right, rightDecided := runtime.Subtype(emptyInner, tableTopInner)
	if !leftDecided || !rightDecided || !left || !right {
		t.Fatalf("fixture lost direct mutual table judgments: %t/%t %t/%t", left, leftDecided, right, rightDecided)
	}
	if set.Equal(classes[1], classes[2]) {
		t.Fatal("direct mutual subtype edges were incorrectly treated as an equivalence relation")
	}
	if !set.LessOrEq(classes[2], classes[1]) || set.LessOrEq(classes[1], classes[2]) {
		t.Fatal("the Any witness did not distinguish the two principal coverages")
	}
}

func TestClassSetUniversalCoveragePrefersUnknown(t *testing.T) {
	set, _, classes := sealExtensionalClassFixture(t, []typ.Type{
		typ.Nil, typ.Any, typ.Unknown, typ.String,
	})
	anyClass, unknownClass, textClass := classes[1], classes[2], classes[3]
	if !set.Equal(set.AnyValue(), anyClass) || !set.Equal(anyClass, unknownClass) {
		t.Fatal("Any, Unknown, and ClassAnyValue did not share universal coverage")
	}
	atoms, ok := set.classAtoms(unknownClass)
	if !ok || len(atoms) != 1 || atoms[0] != set.unknownAtom {
		t.Fatal("universal principal basis did not choose Unknown")
	}
	if !set.LessOrEq(textClass, unknownClass) || set.LessOrEq(unknownClass, textClass) {
		t.Fatal("universal coverage order is not strict over string")
	}
	if set.Rank(textClass) <= set.Rank(unknownClass) {
		t.Fatal("ideal-complement rank did not descend into universal coverage")
	}
}

func TestClassSetExtensionalOrderIgnoresConstructionPermutation(t *testing.T) {
	left, _, leftClasses := sealExtensionalClassFixture(t, []typ.Type{typ.Nil, typ.Any, typ.Unknown, typ.String})
	right, _, rightClasses := sealExtensionalClassFixture(t, []typ.Type{typ.String, typ.Unknown, typ.Any, typ.Nil})
	// Positional values differ, so compare the stable algebra itself and the
	// universal/string ranks rather than construction-local handles.
	if len(left.rows) != len(right.rows) || len(left.universeIDs) != len(right.universeIDs) {
		t.Fatal("extensional carrier cardinality depends on construction order")
	}
	for index := range left.universeIDs {
		if left.universeIDs[index] != right.universeIDs[index] {
			t.Fatalf("portable universe order changed at %d", index)
		}
	}
	if left.Rank(leftClasses[1]) != right.Rank(rightClasses[2]) ||
		left.Rank(leftClasses[3]) != right.Rank(rightClasses[0]) {
		t.Fatal("coverage rank depends on construction order")
	}
}

func sealExtensionalClassFixture(t *testing.T, values []typ.Type) (*ClassSet, *typeauthority.Runtime, []Class) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "class_coverage.lua", Text: []byte(`return nil`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "class_coverage", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("type authority")
	}
	inputs := make([]typeauthority.RuntimeInput, len(values))
	encoded := make([][]byte, len(values))
	for index, value := range values {
		encoded[index], err = typ.EncodeCanonical(context.Background(), value)
		if err != nil {
			t.Fatal(err)
		}
		var admitted bool
		inputs[index], admitted = types.RuntimeInput(encoded[index])
		if !admitted {
			t.Fatalf("Runtime input %d", index)
		}
	}
	runtime, inners, err := typeauthority.SealRuntime(types, inputs)
	if err != nil {
		t.Fatal(err)
	}
	set := &ClassSet{rows: []classRow{{kind: ClassAnyValue}}, byStatic: make(map[uint32]Class), byTarget: make(map[target.Type]Class)}
	for index := range values {
		class := Class{owner: set, index: uint32(len(set.rows))}
		set.rows = append(set.rows, classRow{kind: ClassConcrete, encoded: encoded[index], inner: inners[index]})
		set.byStatic[uint32(index+2)] = class
		if typ.TypeEquals(values[index], typ.Nil) {
			set.nil = class
		}
	}
	if set.nil.owner == nil {
		t.Fatal("fixture requires nil")
	}
	if err := set.sealClassOrder(runtime); err != nil {
		t.Fatal(err)
	}
	if err := set.sealDescriptors(runtime); err != nil {
		t.Fatal(err)
	}
	set.id = sha256.Sum256([]byte("static extensional fixture"))
	result := make([]Class, len(values))
	for index := range result {
		result[index] = set.byStatic[uint32(index+2)]
	}
	return set, runtime, result
}
