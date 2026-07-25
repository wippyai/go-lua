package engine

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// resultPayloadArm and resultErrorArm are the two arms of the discriminated
// Result surface a generic wrapper declares and its call site supplies.
func resultPayloadArm(payload typ.Type) typ.Type {
	return &typ.Record{Fields: []typ.Field{
		{Name: "ok", Type: typ.LiteralBool(true)},
		{Name: "value", Type: payload},
	}}
}

func resultErrorArm() typ.Type {
	return &typ.Record{Fields: []typ.Field{
		{Name: "ok", Type: typ.LiteralBool(false)},
		{Name: "error", Type: typ.String},
	}}
}

func resultUnion(payload typ.Type) *typ.Union {
	return &typ.Union{Members: []typ.Type{resultPayloadArm(payload), resultErrorArm()}}
}

// TestGenericUnionParameterBindsFromArgumentArms covers the contract a local
// generic wrapper such as map_result(result: Result<T>, fn: (T) -> U) needs:
// its declared parameter is a union whose payload arm carries the binder, and
// the call site supplies the same union with that arm already concrete.
func TestGenericUnionParameterBindsFromArgumentArms(t *testing.T) {
	params := map[string]bool{"T": true}
	bindings := map[string]typ.Type{}
	expected := resultUnion(&typ.TypeParam{Name: "T"})
	actual := resultUnion(typ.String)
	if !inferImportedTypeArgs(expected, actual, params, bindings) {
		t.Fatal("Result<T> must unify with Result<string>")
	}
	if bound := bindings["T"]; bound == nil || !typ.TypeEquals(bound, typ.String) {
		t.Fatalf("T must bind to string, got %v", bindings["T"])
	}
}

// TestGenericUnionParameterBindsIndependentOfArmOrder proves the pairing is
// decided by structure rather than declaration order: the concrete arm is
// listed first here and the payload arm second.
func TestGenericUnionParameterBindsIndependentOfArmOrder(t *testing.T) {
	params := map[string]bool{"T": true}
	bindings := map[string]typ.Type{}
	expected := resultUnion(&typ.TypeParam{Name: "T"})
	actual := &typ.Union{Members: []typ.Type{resultErrorArm(), resultPayloadArm(typ.Number)}}
	if !inferImportedTypeArgs(expected, actual, params, bindings) {
		t.Fatal("Result<T> must unify with Result<number> regardless of arm order")
	}
	if bound := bindings["T"]; bound == nil || !typ.TypeEquals(bound, typ.Number) {
		t.Fatalf("T must bind to number, got %v", bindings["T"])
	}
}

// TestGenericUnionParameterResolvesForcedPairing proves a binder arm that
// accepts several candidates still binds once the arms that accept exactly
// one candidate have claimed theirs.
func TestGenericUnionParameterResolvesForcedPairing(t *testing.T) {
	params := map[string]bool{"T": true}
	bindings := map[string]typ.Type{}
	expected := &typ.Union{Members: []typ.Type{&typ.TypeParam{Name: "T"}, typ.String}}
	actual := &typ.Union{Members: []typ.Type{typ.String, typ.Number}}
	if !inferImportedTypeArgs(expected, actual, params, bindings) {
		t.Fatal("T | string must unify with string | number")
	}
	if bound := bindings["T"]; bound == nil || !typ.TypeEquals(bound, typ.Number) {
		t.Fatalf("T must bind to the remaining arm number, got %v", bindings["T"])
	}
}

// TestGenericUnionParameterFailsClosedWhenAmbiguous keeps the instantiation
// unproven when no pairing is forced: two binders over two unrelated arms
// admit both assignments, so neither may be published.
func TestGenericUnionParameterFailsClosedWhenAmbiguous(t *testing.T) {
	params := map[string]bool{"T": true, "U": true}
	bindings := map[string]typ.Type{}
	expected := &typ.Union{Members: []typ.Type{&typ.TypeParam{Name: "T"}, &typ.TypeParam{Name: "U"}}}
	actual := &typ.Union{Members: []typ.Type{typ.String, typ.Number}}
	if inferImportedTypeArgs(expected, actual, params, bindings) {
		t.Fatal("an ambiguous arm pairing must not be instantiated")
	}
	if len(bindings) != 0 {
		t.Fatalf("a refused unification must publish no binding, got %v", bindings)
	}
}

// TestGenericUnionParameterRejectsUnequalArity keeps a union whose arms cannot
// be paired one to one out of the proof.
func TestGenericUnionParameterRejectsUnequalArity(t *testing.T) {
	params := map[string]bool{"T": true}
	bindings := map[string]typ.Type{}
	expected := &typ.Union{Members: []typ.Type{&typ.TypeParam{Name: "T"}, typ.String}}
	actual := &typ.Union{Members: []typ.Type{typ.String, typ.Number, typ.Boolean}}
	if inferImportedTypeArgs(expected, actual, params, bindings) {
		t.Fatal("unions of different arity must not unify")
	}
}

// TestGenericInferenceTerminatesOnCyclicType walks a recursive declaration
// whose member graph closes on itself. The unifier must decide the pair
// coinductively instead of descending forever.
func TestGenericInferenceTerminatesOnCyclicType(t *testing.T) {
	expectedNode := &typ.Record{}
	expectedNode.Fields = []typ.Field{
		{Name: "kind", Type: typ.String},
		{Name: "payload", Type: &typ.TypeParam{Name: "T"}},
		{Name: "next", Type: &typ.Union{Members: []typ.Type{expectedNode, typ.Nil}}},
	}
	actualNode := &typ.Record{}
	actualNode.Fields = []typ.Field{
		{Name: "kind", Type: typ.String},
		{Name: "payload", Type: typ.Number},
		{Name: "next", Type: &typ.Union{Members: []typ.Type{actualNode, typ.Nil}}},
	}
	params := map[string]bool{"T": true}
	bindings := map[string]typ.Type{}
	done := make(chan bool, 1)
	go func() { done <- inferImportedTypeArgs(expectedNode, actualNode, params, bindings) }()
	select {
	case unified := <-done:
		if !unified {
			t.Fatal("a recursive node must unify with the same recursive node")
		}
		if bound := bindings["T"]; bound == nil || !typ.TypeEquals(bound, typ.Number) {
			t.Fatalf("T must bind to number, got %v", bindings["T"])
		}
	case <-time.After(10 * time.Second):
		t.Fatal("generic inference must terminate on a cyclic type graph")
	}
}
