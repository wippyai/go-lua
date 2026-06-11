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

func TestEqualityFacadeMatchesTypHelpers(t *testing.T) {
	var nilFunction *typ.Function
	var nilType typ.Type = nilFunction

	recA := typ.NewRecursivePlaceholder("Node")
	recA.SetBody(&typ.Record{Fields: []typ.Field{{Name: "next", Type: recA}}})
	recB := typ.NewRecursivePlaceholder("Node")
	recB.SetBody(&typ.Record{Fields: []typ.Field{{Name: "next", Type: recB}}})

	acyclicA := &typ.Record{Fields: []typ.Field{{Name: "name", Type: typ.String}}}
	acyclicB := &typ.Record{Fields: []typ.Field{{Name: "name", Type: typ.String}}}

	for _, tc := range []struct {
		name string
		a    typ.Type
		b    typ.Type
	}{
		{name: "recursive equivalent", a: recA, b: recB},
		{name: "recursive same node", a: recA, b: recA},
		{name: "acyclic structural pair", a: acyclicA, b: acyclicB},
		{name: "typed nil and nil", a: nilType, b: nil},
	} {
		if got, want := TypeEquals(tc.a, tc.b), typ.TypeEquals(tc.a, tc.b); got != want {
			t.Fatalf("%s: identity.TypeEquals = %v, typ.TypeEquals = %v", tc.name, got, want)
		}
		if got, want := SameNode(tc.a, tc.b), typ.SameNode(tc.a, tc.b); got != want {
			t.Fatalf("%s: identity.SameNode = %v, typ.SameNode = %v", tc.name, got, want)
		}
		if got, want := SameNodeOrAcyclicEqual(tc.a, tc.b), typ.SameNodeOrAcyclicEqual(tc.a, tc.b); got != want {
			t.Fatalf("%s: identity.SameNodeOrAcyclicEqual = %v, typ.SameNodeOrAcyclicEqual = %v", tc.name, got, want)
		}
	}

	for _, tc := range []struct {
		name string
		t    typ.Type
	}{
		{name: "nil", t: nil},
		{name: "typed nil", t: nilType},
		{name: "non-nil", t: typ.String},
	} {
		if got, want := NormalizeNilType(tc.t), typ.NormalizeNilType(tc.t); got != want {
			t.Fatalf("%s: identity.NormalizeNilType = %v, typ.NormalizeNilType = %v", tc.name, got, want)
		}
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

func TestEqualityHashParityWithTypForRecursiveWrappers(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	staleWrapper := &typ.Record{Fields: []typ.Field{{Name: "next", Type: node, Optional: true}}}

	node.SetBody(&typ.Record{Fields: []typ.Field{
		{Name: "next", Type: node, Optional: true},
		{Name: "value", Type: typ.Number},
	}, StaticMembers: []typ.StaticMember{
		{Kind: typ.StaticMemberStringIndex, Name: "meta", Type: typ.String, Optional: true},
	}})
	freshWrapper := &typ.Record{Fields: []typ.Field{{Name: "next", Type: node, Optional: true}}}
	memberWrapper := &typ.Record{
		Fields: []typ.Field{{Name: "next", Type: node, Optional: true}},
		StaticMembers: []typ.StaticMember{{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "meta",
			Type:     typ.String,
			Optional: true,
		}},
	}

	if !TypeEquals(staleWrapper, freshWrapper) {
		t.Fatal("wrapper built before SetBody should remain structurally equal to a fresh wrapper")
	}
	if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
		t.Fatalf("equality hash should refresh open recursive wrapper: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
	}

	for _, tc := range []struct {
		name string
		t    typ.Type
	}{
		{name: "stale wrapper", t: staleWrapper},
		{name: "fresh wrapper", t: freshWrapper},
		{name: "member wrapper", t: memberWrapper},
	} {
		if got, want := EqualityHash(tc.t), typ.EqualityHash(tc.t); got != want {
			t.Fatalf("%s: identity.EqualityHash = %d, typ.EqualityHash = %d", tc.name, got, want)
		}
	}
}
