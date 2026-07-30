package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// declaredCalleeValue is the fact set a call reads for a callee term holding a
// sealed function literal.
func declaredCalleeValue(t *testing.T, callee string, function typ.Type) []equation.Fact {
	t.Helper()
	return []equation.Fact{{Key: "value/" + callee + "/op-00000001", Value: sealedCallableValue(t, function)}}
}

// TestSealedAnyReturnMaterializesNoValue states the slot the boundary replaces:
// the value lattice carries no witness for a declared any, so a result owner
// reading only the callee's sealed contract has nothing to publish but top.
func TestSealedAnyReturnMaterializesNoValue(t *testing.T) {
	if value, materialized := providerReturnTypeValue(typ.Any); materialized {
		t.Fatalf("a declared any return materialized the value %q", value)
	}
	if value, materialized := finiteReturnWitnessValue(typ.Any); materialized {
		t.Fatalf("a declared any return produced the finite witness %q", value)
	}
}

// TestDeclaredAnyResultSlotReadsASealedCalleeContract pins the direct callee:
// the sealed function value the term holds states the slot's contract, and only
// an explicit any answers the boundary.
func TestDeclaredAnyResultSlotReadsASealedCalleeContract(t *testing.T) {
	callee := "path/decode"
	cases := []struct {
		name     string
		function typ.Type
		want     bool
	}{
		{"any", typ.Func().Returns(typ.Any).Build(), true},
		{"string", typ.Func().Returns(typ.String).Build(), false},
		{"unknown", typ.Func().Returns(typ.Unknown).Build(), false},
		{"optional any", typ.Func().Returns(typ.MaterializeOptional(typ.Any)).Build(), false},
		{"undeclared", typ.Func().Build(), false},
	}
	for _, test := range cases {
		partition := callResultPartition(t, declaredCalleeValue(t, callee, test.function)...)
		if got := declaredAnyResultSlot(nil, []byte(callee), nil, nil, 0, partition); got != test.want {
			t.Fatalf("%s return boundary = %v, want %v", test.name, got, test.want)
		}
	}
}

// TestDeclaredAnyResultSlotIsPositional keeps a mixed tuple slot by slot: the
// declared string keeps its concrete contract, the declared any publishes the
// boundary, and a slot past the declared arity states nothing at all.
func TestDeclaredAnyResultSlotIsPositional(t *testing.T) {
	callee := "path/pair"
	partition := callResultPartition(t, declaredCalleeValue(t, callee, typ.Func().Returns(typ.String, typ.Any).Build())...)
	for index, want := range []bool{false, true, false} {
		if got := declaredAnyResultSlot(nil, []byte(callee), nil, nil, index, partition); got != want {
			t.Fatalf("slot %d boundary = %v, want %v", index, got, want)
		}
	}
}

// TestDeclaredBindingOutranksTheCallableItHolds pins the declaration order: a
// literal bound to a declared callable is checked against that declaration, so
// the binding's any return decides the slot even where the literal it holds
// states a concrete one.
func TestDeclaredBindingOutranksTheCallableItHolds(t *testing.T) {
	callee := "path/decode"
	declared, encoded := shapefact.EncodeTarget(typ.Func().Returns(typ.Any).Build())
	if !encoded {
		t.Fatal("encode the declared callable")
	}
	facts := append(declaredCalleeValue(t, callee, typ.Func().Returns(typ.String).Build()),
		equation.Fact{Key: "declared-type/" + callee + "/op-00000000", Value: declared})
	if !declaredAnyResultSlot(nil, []byte(callee), nil, nil, 0, callResultPartition(t, facts...)) {
		t.Fatalf("a declared any binding was read through to the literal bound to it")
	}
}

// TestDeclaredAnyResultSlotReadsAMethodContract is the receiver counterpart: a
// published receiver surface states its method's return, and a declared any
// there is the same boundary a direct callee publishes.
func TestDeclaredAnyResultSlotReadsAMethodContract(t *testing.T) {
	receiver := "path/carrier"
	method := []byte(`method/"read"`)
	for _, test := range []struct {
		name   string
		result typ.Type
		want   bool
	}{
		{"any", typ.Any, true},
		{"string", typ.String, false},
	} {
		surface, encoded := shapefact.EncodeTarget(typetable.NewRecord().
			Field("read", typ.Func().Param("self", typ.Any).Returns(test.result).Build()).
			Build())
		if !encoded {
			t.Fatal("encode the receiver surface")
		}
		partition := callResultPartition(t, equation.Fact{Key: "value/" + receiver + "/op-00000001", Value: surface})
		if got := declaredAnyResultSlot(nil, nil, []byte(receiver), method, 0, partition); got != test.want {
			t.Fatalf("%s method return boundary = %v, want %v", test.name, got, test.want)
		}
	}
}

// TestUnresolvedCalleeStatesNoBoundary keeps the lane fail-closed: a callee with
// no published contract has declared nothing, so its result stays top rather
// than acquiring a claim it never made.
func TestUnresolvedCalleeStatesNoBoundary(t *testing.T) {
	if declaredAnyResultSlot(nil, []byte("path/decode"), nil, nil, 0, callResultPartition(t)) {
		t.Fatalf("a callee with no published contract published an any boundary")
	}
}
