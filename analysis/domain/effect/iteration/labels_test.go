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

	if !HasIterator(r) {
		t.Error("Should have iterator")
	}

	iter := GetIterator(r)
	if iter == nil {
		t.Fatal("Should find iterator")
	}
	if iter.Source.Index != 0 {
		t.Errorf("Iterator source index = %d, want 0", iter.Source.Index)
	}

	if !IsIndexedIterator(r) {
		t.Error("Should be indexed iterator")
	}

	if IsKeyedIterator(r) {
		t.Error("Should not be keyed iterator")
	}

	r2 := effect.Row{Labels: []effect.Label{Iterator{Source: effect.ParamRef{Index: 0}, Kind: IterateKeyed}}}
	if !IsKeyedIterator(r2) {
		t.Error("Should be keyed iterator")
	}

	if IsIndexedIterator(r2) {
		t.Error("Should not be indexed iterator")
	}

	if IsIndexedIterator(effect.Empty) || IsKeyedIterator(effect.Empty) {
		t.Error("Empty row should not be any iterator")
	}
}
