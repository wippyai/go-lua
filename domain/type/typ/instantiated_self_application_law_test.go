package typ

import (
	"testing"
)

// selfApplicationList builds a List<T> = {head: T, tail: List<T>} declaration
// through the same open-then-SetBody sequence canonical_decode.go and
// canonical_formals_decode.go use for a genuine self-referential generic: the
// tail field's Instantiated is built while the Generic's own Body is still
// nil. It returns the self-application used as the tail field (built before
// SetBody) and a second, independently constructed application of the same
// closed declaration (built after SetBody).
func selfApplicationList(t *testing.T) (self, fresh *Instantiated) {
	t.Helper()
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, nil)
	self = Instantiate(g, tp)
	body := newRecord().
		Field("head", tp).
		Field("tail", self).
		Build()
	g.SetBody(body)
	fresh = Instantiate(g, tp)
	return self, fresh
}

// TestInstantiatedHashClosesIndependentlyOfConstructionOrder is the red law
// for the eager-seal defect: Instantiate captures Generic.Hash() at
// construction, which is lawfully still provisional during a self
// application. Before Hash was made to read a close-gated cache, self (built
// while the generic was open) and fresh (built after SetBody) hashed
// differently even though they are the same declaration applied to the same
// argument.
func TestInstantiatedHashClosesIndependentlyOfConstructionOrder(t *testing.T) {
	self, fresh := selfApplicationList(t)

	if got, want := self.Hash(), fresh.Hash(); got != want {
		t.Fatalf("Instantiated.Hash() depends on construction order: self-referential build = %d, post-close build = %d", got, want)
	}
	if !self.Equals(fresh) {
		t.Fatal("the same closed self application must compare equal regardless of construction order")
	}
	// Hash must remain stable on repeat reads once published.
	if got, want := self.Hash(), self.Hash(); got != want {
		t.Fatalf("published Instantiated.Hash() is not stable across reads: %d, %d", got, want)
	}
}

// TestInstantiatedUnionOrderAndCanonicalBytesIgnoreConstructionOrder is the
// union-order/wire half of the red law: a stale hash on self would sort it
// into a different union slot, and therefore a different byte position, than
// the semantically identical fresh application.
func TestInstantiatedUnionOrderAndCanonicalBytesIgnoreConstructionOrder(t *testing.T) {
	self, fresh := selfApplicationList(t)

	unionSelf := MaterializeUnion([]Type{String, self})
	unionFresh := MaterializeUnion([]Type{String, fresh})

	selfUnion, ok := unionSelf.(*Union)
	if !ok {
		t.Fatalf("expected *Union from self-referential build, got %T", unionSelf)
	}
	freshUnion, ok := unionFresh.(*Union)
	if !ok {
		t.Fatalf("expected *Union from post-close build, got %T", unionFresh)
	}
	if len(selfUnion.Members) != len(freshUnion.Members) {
		t.Fatalf("union member count differs by construction order: %d vs %d", len(selfUnion.Members), len(freshUnion.Members))
	}
	for i := range selfUnion.Members {
		if selfUnion.Members[i].Kind() != freshUnion.Members[i].Kind() {
			t.Fatalf("union member order at position %d differs by construction order: %v vs %v", i, selfUnion.Members[i].Kind(), freshUnion.Members[i].Kind())
		}
	}

	if !TypeEquals(unionSelf, unionFresh) {
		t.Fatal("union semantics differ by construction order")
	}
}

// TestInstantiatedHashDoesNotPublishWhileGenericIsOpen proves the memo
// discipline rule: a hash read while the reachable graph is still open must
// never be cached, because SetBody can still change it.
func TestInstantiatedHashDoesNotPublishWhileGenericIsOpen(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, nil)
	self := Instantiate(g, tp)

	_ = self.Hash()
	if _, ok := self.equalityHashCache.load(); ok {
		t.Fatal("Instantiated must not publish its hash while the generic body is still open")
	}

	body := newRecord().Field("head", tp).Field("tail", self).Build()
	g.SetBody(body)

	closed := self.Hash()
	if _, ok := self.equalityHashCache.load(); !ok {
		t.Fatal("Instantiated must publish its hash once the generic body closes")
	}
	if got := self.Hash(); got != closed {
		t.Fatalf("published hash changed on a later read: %d, then %d", closed, got)
	}
}

// TestInstantiatedPublishedHashReadAllocatesNothing pins the hot read path:
// once a self application's hash is published, reading it again must be a
// pure cache hit with no allocation.
func TestInstantiatedPublishedHashReadAllocatesNothing(t *testing.T) {
	self, _ := selfApplicationList(t)
	_ = self.Hash()
	if _, ok := self.equalityHashCache.load(); !ok {
		t.Fatal("setup: expected the hash to already be published")
	}

	allocs := testing.AllocsPerRun(100, func() {
		_ = self.Hash()
	})
	if allocs != 0 {
		t.Fatalf("published Instantiated.Hash() read allocated %.1f times/run, want 0", allocs)
	}
}
