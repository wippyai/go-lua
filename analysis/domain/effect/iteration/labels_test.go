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
	var label effect.Label = Iterator{}
	_ = label.String()
	_ = label.Equals(label)
	Iterator{}.EffectLabel()
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
