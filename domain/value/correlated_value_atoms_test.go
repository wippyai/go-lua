package value

import (
	"testing"

	"github.com/wippyai/go-lua/domain/runtimekind"
)

func TestValueAtomPullAccessIsOwnerBoundAndCanonical(t *testing.T) {
	schema := &Schema{
		atomByRow: make(map[atomRow]uint32),
		potential: 32,
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	if schema.addAtom(atomRow{kind: atomOpaqueKind, runtime: runtimekind.Number}) == 0 ||
		schema.addAtom(atomRow{kind: atomOpaqueKind, runtime: runtimekind.String}) == 0 {
		t.Fatal("test atom vocabulary unavailable")
	}
	number, numberOK := schema.OpaqueKind(runtimekind.Number)
	string, stringOK := schema.OpaqueKind(runtimekind.String)
	if !numberOK || !stringOK {
		t.Fatal("test atoms unavailable")
	}
	fact, factOK := schema.Alternatives(string, number)
	if !factOK || schema.ValueAtomCount(fact) != 2 {
		t.Fatal("exact atom relation unavailable")
	}
	first, firstOK := schema.ValueAtomAt(fact, 0)
	second, secondOK := schema.ValueAtomAt(fact, 1)
	if !firstOK || !secondOK || first != number || second != string || !schema.OwnsAtom(first) || !schema.OwnsAtom(second) {
		t.Fatalf("canonical pull order/ownership = %#v/%t, %#v/%t", first, firstOK, second, secondOK)
	}
	if _, ok := schema.ValueAtomAt(fact, -1); ok {
		t.Fatal("negative atom index accepted")
	}
	if _, ok := schema.ValueAtomAt(fact, schema.ValueAtomCount(fact)); ok {
		t.Fatal("out-of-range atom index accepted")
	}
	if schema.ValueAtomCount(schema.Top()) != 0 {
		t.Fatal("Top exposed a finite exact atom count")
	}
	foreign := &Schema{atomByRow: make(map[atomRow]uint32), potential: 32}
	if foreign.ValueAtomCount(fact) != 0 {
		t.Fatal("foreign schema counted a Value relation")
	}
	if _, ok := foreign.ValueAtomAt(fact, 0); ok {
		t.Fatal("foreign schema read a Value relation")
	}
}
