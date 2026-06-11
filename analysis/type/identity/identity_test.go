package identity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestTypeEqualsAliasAndNilNormalization(t *testing.T) {
	alias := typ.NewAlias("MyNum", typ.Number)
	if !TypeEquals(alias, typ.Number) || !TypeEquals(typ.Number, alias) {
		t.Fatal("alias equality should be transparent")
	}

	var nilFunction *typ.Function
	var nilType typ.Type = nilFunction
	if !TypeEquals(nilType, nil) {
		t.Fatal("typed nil should normalize to nil")
	}
}

func TestNormalizeNilType(t *testing.T) {
	var nilFunction *typ.Function
	var nilType typ.Type = nilFunction
	if NormalizeNilType(nilType) != nil {
		t.Fatal("typed nil should normalize to nil")
	}
	if NormalizeNilType(typ.String) != typ.String {
		t.Fatal("non-nil types should be returned unchanged")
	}
}

func TestTypeEqualsRecursiveEquivalent(t *testing.T) {
	left := typ.NewRecursivePlaceholder("Node")
	left.SetBody(&typ.Record{Fields: []typ.Field{{Name: "next", Type: left}}})

	right := typ.NewRecursivePlaceholder("Node")
	right.SetBody(&typ.Record{Fields: []typ.Field{{Name: "next", Type: right}}})

	if !TypeEquals(left, right) {
		t.Fatal("equivalent recursive products should compare equal")
	}
}

func TestSameNodeOrAcyclicEqual(t *testing.T) {
	left := &typ.Record{Fields: []typ.Field{{Name: "name", Type: typ.String}}}
	right := &typ.Record{Fields: []typ.Field{{Name: "name", Type: typ.String}}}

	if !SameNodeOrAcyclicEqual(left, right) {
		t.Fatal("acyclic structural equality should succeed")
	}

	recA := typ.NewRecursivePlaceholder("Node")
	recA.SetBody(&typ.Record{Fields: []typ.Field{{Name: "next", Type: recA}}})
	recB := typ.NewRecursivePlaceholder("Node")
	recB.SetBody(&typ.Record{Fields: []typ.Field{{Name: "next", Type: recB}}})

	if SameNodeOrAcyclicEqual(recA, recB) {
		t.Fatal("recursive equivalence should not be proven by the acyclic fallback")
	}
	if !SameNodeOrAcyclicEqual(recA, recA) {
		t.Fatal("same node identity should still succeed for recursive nodes")
	}
}

func TestSameNode(t *testing.T) {
	left := &typ.Record{Fields: []typ.Field{{Name: "name", Type: typ.String}}}
	right := &typ.Record{Fields: []typ.Field{{Name: "name", Type: typ.String}}}

	if !SameNode(left, left) {
		t.Fatal("SameNode should accept the same type node")
	}
	if SameNode(left, right) {
		t.Fatal("SameNode must not collapse structurally equal but distinct nodes")
	}
}

func TestEqualityHashStableForRecursiveWrappers(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	staleWrapper := &typ.Record{Fields: []typ.Field{{Name: "next", Type: node, Optional: true}}}

	node.SetBody(&typ.Record{Fields: []typ.Field{
		{Name: "next", Type: node, Optional: true},
		{Name: "value", Type: typ.Number},
	}})
	freshWrapper := &typ.Record{Fields: []typ.Field{{Name: "next", Type: node, Optional: true}}}

	if !TypeEquals(staleWrapper, freshWrapper) {
		t.Fatal("wrapper built before SetBody should remain structurally equal to a fresh wrapper")
	}
	if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
		t.Fatalf("equality hash should refresh open recursive wrapper: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
	}
}
