package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLoopContinuationIdentityIncludesLexicalOwner(t *testing.T) {
	arena := NewArena(standard.Registry())
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("loop-continuation-owner"))
	first := lexicalidentity.FunctionBody(namespace, 1)
	second := lexicalidentity.FunctionBody(namespace, 2)
	if !arena.bindLexicalOwner(first) {
		t.Fatal("failed to bind first lexical owner")
	}
	local := arena.loopContinuationValue(5)
	foreign := arena.loopContinuationValueOwned(second, 5)
	if local == 0 || foreign == 0 || local == foreign {
		t.Fatalf("same-point loop continuations aliased across lexical owners: local=%d foreign=%d", local, foreign)
	}
	if repeated := arena.loopContinuationValueOwned(first, 5); repeated != local {
		t.Fatalf("same-owner loop continuation was not interned: got %d want %d", repeated, local)
	}
	if arena.canonicalValue(local) == arena.canonicalValue(foreign) {
		t.Fatal("same-point loop continuations have colliding canonical identities")
	}
}

func TestLoopMuBinderHasNoNestingDepthLimit(t *testing.T) {
	arena := NewArena(standard.Registry())
	var parent loopMuTerm
	const nesting = 256
	for depth := 0; depth < nesting; depth++ {
		head := cfg.Point(depth + 1)
		members := []cfg.Point{head}
		backedges := []loopMuBackedge{{from: head, to: head}}
		binder := arena.loopMu(head, parent, members, backedges)
		if binder == 0 {
			t.Fatalf("LoopMu rejected exact structural nesting at depth %d", depth)
		}
		if repeated := arena.loopMu(head, parent, members, backedges); repeated != binder {
			t.Fatalf("LoopMu at depth %d was not structurally interned: %d != %d", depth, repeated, binder)
		}
		parent = binder
	}
	if got := len(arena.loopMus) - 1; got != nesting {
		t.Fatalf("LoopMu binder count = %d, want %d", got, nesting)
	}
}
