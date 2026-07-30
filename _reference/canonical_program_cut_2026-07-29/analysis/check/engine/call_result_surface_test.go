package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// unitBoxTable is the sealed shape a returned constructor publishes: one member
// present with its own value, inside a closed literal.
func unitBoxTable(closed bool) shapefact.Table {
	return shapefact.Table{
		Closed:  closed,
		Members: []shapefact.Member{{Suffix: ".v", Present: true, Value: "scalar/number/1"}},
	}
}

func callResultPartition(t *testing.T, facts ...equation.Fact) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

// TestSealedShapeAnswersItsOwnMemberRead states the authority a sealed literal
// holds over the members it names, independent of any heap cell.
func TestSealedShapeAnswersItsOwnMemberRead(t *testing.T) {
	value, found := sealedShapeMemberValue([]byte("temp/3"), unitBoxTable(true), ".v", callResultPartition(t))
	if !found || string(value) != "scalar/number/1" {
		t.Fatalf("member read = %q (found %v), want the literal's own value", value, found)
	}
}

// TestClosedSealedShapeProvesAnUnnamedSlotAbsent keeps the closed literal
// falsifiable: a slot it does not name is nil, not an unknown.
func TestClosedSealedShapeProvesAnUnnamedSlotAbsent(t *testing.T) {
	value, found := sealedShapeMemberValue([]byte("temp/3"), unitBoxTable(true), ".absent", callResultPartition(t))
	if !found || string(value) != "scalar/nil" {
		t.Fatalf("absent member = %q (found %v), want scalar/nil", value, found)
	}
}

// TestOpenSealedShapeStatesNothingAboutAnUnnamedSlot is the fail-closed half:
// an open literal has not accounted for every slot it may hold.
func TestOpenSealedShapeStatesNothingAboutAnUnnamedSlot(t *testing.T) {
	if value, found := sealedShapeMemberValue([]byte("temp/3"), unitBoxTable(false), ".absent", callResultPartition(t)); found {
		t.Fatalf("an open literal answered for a slot it does not name: %q", value)
	}
}

// TestOpaqueStoreRevokesTheSealedMemberAnswer keeps a store the partition could
// not name from being contradicted by the literal it may have overwritten.
func TestOpaqueStoreRevokesTheSealedMemberAnswer(t *testing.T) {
	identity := []byte("sealed-table/test/op-00000002")
	opaque, err := heapOpaqueMemberWriteFact(identity, "op-00000004", nil, keyedStoreTypes{})
	if err != nil {
		t.Fatalf("encode unresolved-key store: %v", err)
	}
	partition := callResultPartition(t,
		heapIdentityFact("temp/3", "op-00000002", identity),
		equation.Fact{Key: factkey.Epoch.Key().String() + "temp/3/op-00000002", Value: []byte("op-00000002")},
		opaque,
	)
	if value, found := sealedShapeMemberValue([]byte("temp/3"), unitBoxTable(true), ".v", partition); found {
		t.Fatalf("a store at an unresolved key left the literal answering: %q", value)
	}
}

// TestSealedContainerReadYieldsToATrackedHeapCell keeps the two lanes ordered:
// a container with a cell of its own is read through that cell, so the
// allocation-time literal never answers past a write the cell recorded.
func TestSealedContainerReadYieldsToATrackedHeapCell(t *testing.T) {
	encoded, ok := shapefact.EncodeTable(unitBoxTable(true))
	if !ok {
		t.Fatal("encode sealed literal")
	}
	partition := callResultPartition(t,
		equation.Fact{Key: "value/temp/3/op-00000002", Value: encoded},
		equation.Fact{Key: "value/temp/4/op-00000002", Value: []byte(`scalar/string/"v"`)},
		heapIdentityFact("temp/3", "op-00000002", []byte("sealed-table/test/op-00000002")),
		equation.Fact{Key: factkey.Epoch.Key().String() + "temp/3/op-00000002", Value: []byte("op-00000002")},
	)
	if value, found := sealedContainerMemberValue([]byte("temp/3"), []byte("temp/4"), partition); found {
		t.Fatalf("the literal answered for a tracked cell: %q", value)
	}
}

// TestSealedContainerReadAnswersATermWithNoCell is the case a call result
// reaches: the result term names no cell, so its published shape is the only
// authority the read has.
func TestSealedContainerReadAnswersATermWithNoCell(t *testing.T) {
	encoded, ok := shapefact.EncodeTable(unitBoxTable(true))
	if !ok {
		t.Fatal("encode sealed literal")
	}
	partition := callResultPartition(t,
		equation.Fact{Key: "value/temp/3/op-00000002", Value: encoded},
		equation.Fact{Key: "value/temp/4/op-00000002", Value: []byte(`scalar/string/"v"`)},
	)
	value, found := sealedContainerMemberValue([]byte("temp/3"), []byte("temp/4"), partition)
	if !found || string(value) != "scalar/number/1" {
		t.Fatalf("call-result member read = %q (found %v), want the literal's own value", value, found)
	}
}

func eventResultType() *typ.Record {
	return typetable.NewRecord().
		Field("kind", typ.String).
		Field("payload", typ.MaterializeOptional(typ.String)).
		Build()
}

// sealedCallableValue is the value a closure literal carries across the value
// lattice: the front's callable payload holding its canonical function type.
func sealedCallableValue(t *testing.T, function typ.Type) []byte {
	t.Helper()
	canonical, err := typ.EncodeCanonical(context.Background(), function)
	if err != nil {
		t.Fatalf("encode callable surface: %v", err)
	}
	payload, marshalErr := json.Marshal(callableShape{Canonical: base64.RawURLEncoding.EncodeToString(canonical)})
	if marshalErr != nil {
		t.Fatalf("encode callable payload: %v", marshalErr)
	}
	return []byte("scalar/function/" + base64.RawURLEncoding.EncodeToString(payload))
}

// bareCallableValue is the surface a returned closure literal carries: its
// parameters, and no result of its own.
func bareCallableValue(t *testing.T, params ...typ.Type) []byte {
	t.Helper()
	builder := typ.Func()
	for index, parameter := range params {
		builder.Param(string(rune('a'+index)), parameter)
	}
	return sealedCallableValue(t, builder.Build())
}

func contractFact(t *testing.T, application, key string, result typ.Type) equation.Fact {
	t.Helper()
	encoded, err := typ.EncodeCanonical(context.Background(), result)
	if err != nil {
		t.Fatalf("encode derived result: %v", err)
	}
	return equation.Fact{Key: factkey.CallInferredReturn.Key().String() + application + "/" + key, Value: encoded}
}

// TestReturnedCallableIsCompletedWithItsDerivedResult states the fusion: the
// surface crossed on the value and the result crossed on the application
// coordinate describe one callable.
func TestReturnedCallableIsCompletedWithItsDerivedResult(t *testing.T) {
	partition := callResultPartition(t, contractFact(t, "op-00000004", "00000000", eventResultType()))
	completed, fused := completedCallableResultValue(bareCallableValue(t), []byte("call/op-00000004"), "00000000", partition)
	if !fused {
		t.Fatal("a returned callable was left without the result its body derived")
	}
	surface, decoded := shapefact.DecodeTarget(completed)
	if !decoded {
		t.Fatalf("completed callable %q is not a type target", completed)
	}
	function, callable := surface.(*typ.Function)
	if !callable || len(function.Returns) != 1 || !typ.TypeEquals(function.Returns[0], eventResultType()) {
		t.Fatalf("completed callable = %v, want fun(): Event", surface)
	}
}

// TestReturnedCallableKeepsItsOwnParameterSurface keeps completion from
// rewriting anything the callable itself declared.
func TestReturnedCallableKeepsItsOwnParameterSurface(t *testing.T) {
	partition := callResultPartition(t, contractFact(t, "op-00000004", "00000000", eventResultType()))
	completed, fused := completedCallableResultValue(bareCallableValue(t, typ.Number), []byte("call/op-00000004"), "00000000", partition)
	if !fused {
		t.Fatal("a parameterized callable was left without its derived result")
	}
	surface, _ := shapefact.DecodeTarget(completed)
	function, callable := surface.(*typ.Function)
	if !callable || len(function.Params) != 1 || !typ.TypeEquals(function.Params[0].Type, typ.Number) {
		t.Fatalf("completed callable = %v, want the callable's own parameter surface", surface)
	}
}

// TestCallableWithoutADerivedResultKeepsItsBareSurface is the fail-closed half:
// no contract row means the callee derived nothing, and nothing is invented.
func TestCallableWithoutADerivedResultKeepsItsBareSurface(t *testing.T) {
	if completed, fused := completedCallableResultValue(bareCallableValue(t), []byte("call/op-00000004"), "00000000", callResultPartition(t)); fused {
		t.Fatalf("a callable with no derived result was completed with %q", completed)
	}
}

// TestDeclaredCallableResultIsNeverRewritten keeps a callable that states its
// own result as its sole authority.
func TestDeclaredCallableResultIsNeverRewritten(t *testing.T) {
	builder := typ.Func()
	builder.Returns(typ.String)
	declared := sealedCallableValue(t, builder.Build())

	partition := callResultPartition(t, contractFact(t, "op-00000004", "00000000", eventResultType()))
	if completed, fused := completedCallableResultValue(declared, []byte("call/op-00000004"), "00000000", partition); fused {
		t.Fatalf("a declared result was displaced by a derived one: %q", completed)
	}
}

// TestNonCallableResultIsNotCompleted keeps the lane to callables: a table
// result carries no surface to fuse a contract into.
func TestNonCallableResultIsNotCompleted(t *testing.T) {
	encoded, ok := shapefact.EncodeTable(unitBoxTable(true))
	if !ok {
		t.Fatal("encode sealed literal")
	}
	partition := callResultPartition(t, contractFact(t, "op-00000004", "00000000", eventResultType()))
	if completed, fused := completedCallableResultValue(encoded, []byte("call/op-00000004"), "00000000", partition); fused {
		t.Fatalf("a table result was completed as a callable: %q", completed)
	}
}
