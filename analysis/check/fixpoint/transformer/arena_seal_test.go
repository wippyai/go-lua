package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestArenaSealReusesFrozenTermsAndRejectsGrowth(t *testing.T) {
	reg := axis.NewRegistry()
	arena := NewArena(reg)
	effects := NewEffectArena(arena)
	root := arena.Root(Root{Kind: RootParam})
	path := arena.Path(Root{Kind: RootParam})
	guard := arena.Truthy(root)
	frame := arena.callFrame(CellRef{Function: 1}, cfg.Point(7), 1, Shape{Params: 1}, []ValueTerm{root}, []PathTerm{path}, 0)
	loop := arena.loopMu(cfg.Point(3), 0, []cfg.Point{3, 4}, []loopMuBackedge{{from: 4, to: 3}})
	effect, err := effects.InvalidatePath(InvalidatePathConfig{Target: PathEffectTarget(path), Scope: InvalidationScopeSubtree})
	if err != nil || root == 0 || path == 0 || guard == 0 || frame == 0 || loop == 0 || effect == 0 {
		t.Fatalf("pre-seal construction failed: root=%d path=%d guard=%d frame=%d loop=%d effect=%d err=%v", root, path, guard, frame, loop, effect, err)
	}
	values, paths, guards := len(arena.values), len(arena.paths), len(arena.guards)
	frames, loops, effectTerms := len(arena.callFrames), len(arena.loopMus), len(effects.nodes)

	arena.Seal()
	effects.Seal()
	if !arena.Sealed() || !effects.Sealed() {
		t.Fatal("seal did not close both owner arenas")
	}
	if arena.Root(Root{Kind: RootParam}) != root || arena.Path(Root{Kind: RootParam}) != path || arena.Truthy(root) != guard ||
		arena.callFrame(CellRef{Function: 1}, cfg.Point(7), 1, Shape{Params: 1}, []ValueTerm{root}, []PathTerm{path}, 0) != frame ||
		arena.loopMu(cfg.Point(3), 0, []cfg.Point{3, 4}, []loopMuBackedge{{from: 4, to: 3}}) != loop {
		t.Fatal("seal prevented structural reuse of frozen terms")
	}
	if got, err := effects.InvalidatePath(InvalidatePathConfig{Target: PathEffectTarget(path), Scope: InvalidationScopeSubtree}); err != nil || got != effect {
		t.Fatalf("seal prevented structural effect reuse: got=%d err=%v", got, err)
	}
	if arena.Root(Root{Kind: RootGlobal}) != 0 || arena.Falsy(root) != 0 ||
		arena.callFrame(CellRef{Function: 2}, cfg.Point(7), 1, Shape{Params: 1}, []ValueTerm{root}, []PathTerm{path}, 0) != 0 ||
		arena.loopMu(cfg.Point(5), 0, []cfg.Point{5, 6}, []loopMuBackedge{{from: 6, to: 5}}) != 0 {
		t.Fatal("sealed scalar arena admitted novel syntax")
	}
	if got, err := effects.InvalidatePath(InvalidatePathConfig{Target: PathEffectTarget(path), Scope: InvalidationScopeDescendants}); err != nil || got != 0 {
		t.Fatalf("sealed effect arena admitted novel syntax: got=%d err=%v", got, err)
	}
	if len(arena.values) != values || len(arena.paths) != paths || len(arena.guards) != guards ||
		len(arena.callFrames) != frames || len(arena.loopMus) != loops || len(effects.nodes) != effectTerms {
		t.Fatal("sealed arenas grew")
	}
}
