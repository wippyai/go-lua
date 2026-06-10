package identity

import (
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

var (
	allocA  = ID{Kind: "alloc", Site: "chunk.lua:12:4", Index: 1}
	allocB  = ID{Kind: "alloc", Site: "chunk.lua:12:4", Index: 2}
	closure = ID{Kind: "closure", Site: "chunk.lua:20:9", Index: 1}
)

func TestIdentityLatticeLaws(t *testing.T) {
	suite := latticelaws.LawSuite[Value]{
		Name:   "axis.identity",
		Domain: Spec().Lattice(),
		Sample: []Value{
			Bottom(),
			Singleton(allocA),
			Singleton(allocB),
			Singleton(closure),
			Top(),
		},
		Format: Value.String,
	}
	suite.Run(t)
}

func TestSingletonReadback(t *testing.T) {
	v := Singleton(allocA)
	got, ok := v.ID()
	if !ok || got != allocA {
		t.Fatalf("ID() = (%#v, %v), want (%#v, true)", got, ok, allocA)
	}
	if got, ok := Bottom().ID(); ok || got != (ID{}) {
		t.Fatalf("Bottom().ID() = (%#v, %v), want zero/false", got, ok)
	}
	if got, ok := Top().ID(); ok || got != (ID{}) {
		t.Fatalf("Top().ID() = (%#v, %v), want zero/false", got, ok)
	}
}

func TestIdentityJoin(t *testing.T) {
	a := Singleton(allocA)
	same := Singleton(allocA)
	b := Singleton(allocB)

	if got := Join(Bottom(), a); !Equal(got, a) {
		t.Fatalf("Bottom join singleton = %s, want %s", got, a)
	}
	if got := Join(a, same); !Equal(got, a) {
		t.Fatalf("same singleton join = %s, want %s", got, a)
	}
	if got := Join(a, b); !Equal(got, Top()) {
		t.Fatalf("different singleton join = %s, want top", got)
	}
	if got := Widen(a, b); !Equal(got, Join(a, b)) {
		t.Fatalf("Widen = %s, want Join result", got)
	}
}

func TestIdentityOrderAndCovers(t *testing.T) {
	a := Singleton(allocA)
	b := Singleton(allocB)

	if !LessOrEq(Bottom(), a) || !LessOrEq(a, Top()) {
		t.Fatalf("expected bottom < singleton < top")
	}
	if LessOrEq(a, b) || LessOrEq(b, a) {
		t.Fatalf("distinct singleton identities must be incomparable")
	}
	if !Top().Covers(a) || !a.Covers(Bottom()) {
		t.Fatalf("Covers should be the inverse order")
	}
	if a.Covers(b) {
		t.Fatalf("singleton should not cover a distinct singleton")
	}
}

func TestIdentityHashAndString(t *testing.T) {
	a := Singleton(allocA)
	same := Singleton(allocA)
	b := Singleton(allocB)

	if a.Hash() != same.Hash() {
		t.Fatalf("equal singleton values should have equal hashes")
	}
	if a.Hash() == b.Hash() {
		t.Fatalf("distinct singleton identities should not collide in this regression case")
	}
	if Bottom().Hash() == Top().Hash() {
		t.Fatalf("bottom and top should not hash identically")
	}
	if got := allocA.String(); got != "alloc:chunk.lua:12:4#1" {
		t.Fatalf("ID.String() = %q, want alloc:chunk.lua:12:4#1", got)
	}
	if got := a.String(); got != "singleton(alloc:chunk.lua:12:4#1)" {
		t.Fatalf("Value.String() = %q, want singleton(alloc:chunk.lua:12:4#1)", got)
	}
}
