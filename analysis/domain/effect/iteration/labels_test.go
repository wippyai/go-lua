package iteration

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

func TestIterator(t *testing.T) {
	iter := Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateIndexed}
	if got := iter.String(); got != "iterator(param[0], indexed)" {
		t.Errorf("Iterator indexed.String() = %q", got)
	}

	iterKeyed := Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateKeyed}
	if got := iterKeyed.String(); got != "iterator(param[0], keyed)" {
		t.Errorf("Iterator keyed.String() = %q", got)
	}

	if !iter.Equals(Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateIndexed}) {
		t.Error("same Iterator should be equal")
	}

	if iter.Equals(Iterator{Source: effect.ParamRef{Index: 1}, Kind: IterateIndexed}) {
		t.Error("different source should not be equal")
	}

	if iter.Equals(Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateKeyed}) {
		t.Error("different kind should not be equal")
	}

	if iter.Equals(returns.Return{}) {
		t.Error("Iterator should not equal Return")
	}
}

func TestIteratorImplementsLabel(t *testing.T) {
	var _ effect.Label = Iterator{}

	indexed := Iterator{Source: effect.ParamRef{Index: 3}, Kind: IterateIndexed}
	if indexed.Kind != IterateIndexed {
		t.Errorf("indexed iterator Kind = %d, want %d", indexed.Kind, IterateIndexed)
	}
	if got := indexed.String(); got != "iterator(param[3], indexed)" {
		t.Errorf("indexed iterator.String() = %q, want %q", got, "iterator(param[3], indexed)")
	}

	keyed := Iterator{Source: effect.ParamRef{Index: 4}, Kind: IterateKeyed}
	if keyed.Kind != IterateKeyed {
		t.Errorf("keyed iterator Kind = %d, want %d", keyed.Kind, IterateKeyed)
	}
	if got := keyed.String(); got != "iterator(param[4], keyed)" {
		t.Errorf("keyed iterator.String() = %q, want %q", got, "iterator(param[4], keyed)")
	}
}

func TestIteratorEffects(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateIndexed}}}

	if !r.Has(func(l effect.Label) bool {
		_, ok := l.(Iterator)
		return ok
	}) {
		t.Error("Should have iterator")
	}

	iter, ok := effect.NormalizeLabel(r.Labels[0]).(Iterator)
	if !ok {
		t.Fatal("Should normalize iterator label")
	}
	if iter.Source.Index != 0 {
		t.Errorf("Iterator source index = %d, want 0", iter.Source.Index)
	}
	if iter.Kind != IterateIndexed {
		t.Error("Should be indexed iterator")
	}

	r2 := effect.Row{Labels: []effect.Label{Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateKeyed}}}
	iter2, ok := effect.NormalizeLabel(r2.Labels[0]).(Iterator)
	if !ok {
		t.Fatal("Should normalize keyed iterator label")
	}
	if iter2.Kind != IterateKeyed {
		t.Error("Should be keyed iterator")
	}

	if effect.Empty.Has(func(l effect.Label) bool {
		_, ok := l.(Iterator)
		return ok
	}) {
		t.Error("Empty row should not be any iterator")
	}
}

func TestActiveIterator(t *testing.T) {
	indexed := Iterator{Source: effect.ParamRef{Index: 1}, Kind: IterateIndexed}
	keyed := Iterator{Source: effect.ParamRef{Index: 2}, Kind: IterateKeyed}

	got, ok := ActiveIterator([]effect.Label{returns.Return{}, indexed})
	if !ok || !got.Equals(indexed) {
		t.Fatalf("ActiveIterator value label = %v/%v, want %v", got, ok, indexed)
	}

	got, ok = ActiveIterator([]effect.Label{&keyed})
	if !ok || !got.Equals(keyed) {
		t.Fatalf("ActiveIterator pointer label = %v/%v, want %v", got, ok, keyed)
	}

	if got, ok := ActiveIterator([]effect.Label{returns.Return{}}); ok || got.Source.Index != 0 {
		t.Fatalf("ActiveIterator missing label = %v/%v, want false", got, ok)
	}
}
