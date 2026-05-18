package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
)

type parentScopeStoreStub struct {
	parents map[uint64]*scope.State
	hashes  map[uint64]uint64
}

func (s *parentScopeStoreStub) Parents() map[uint64]*scope.State {
	return s.parents
}

func (s *parentScopeStoreStub) GraphParentHashOf(graphID uint64) uint64 {
	return s.hashes[graphID]
}

func (s *parentScopeStoreStub) GraphKeyFor(graph *cfg.Graph, parent *scope.State) (GraphKey, bool) {
	return GraphKey{}, false
}

func TestParentScopeForGraph_PrefersStoredParent(t *testing.T) {
	defaultScope := scope.New()
	stored := scope.New()
	store := &parentScopeStoreStub{
		parents: map[uint64]*scope.State{11: stored},
		hashes:  map[uint64]uint64{7: 11},
	}

	got := ParentScopeForGraph(store, 7, defaultScope)
	if got != stored {
		t.Fatalf("expected stored parent, got %p want %p", got, stored)
	}
}

func TestParentScopeForGraph_UsesDefaultWhenStoredMissing(t *testing.T) {
	defaultScope := scope.New()
	store := &parentScopeStoreStub{
		parents: map[uint64]*scope.State{},
		hashes:  map[uint64]uint64{7: 11},
	}

	got := ParentScopeForGraph(store, 7, defaultScope)
	if got != defaultScope {
		t.Fatalf("expected default parent, got %p want %p", got, defaultScope)
	}
}

func TestParentHashForGraph_PrefersStoredHash(t *testing.T) {
	defaultScope := scope.New()
	store := &parentScopeStoreStub{
		parents: map[uint64]*scope.State{},
		hashes:  map[uint64]uint64{7: 11},
	}

	got := ParentHashForGraph(store, 7, defaultScope)
	if got != 11 {
		t.Fatalf("expected stored hash 11, got %d", got)
	}
}

func TestParentHashForGraph_UsesDefaultScopeHash(t *testing.T) {
	defaultScope := scope.New()
	store := &parentScopeStoreStub{
		parents: map[uint64]*scope.State{},
		hashes:  map[uint64]uint64{},
	}

	got := ParentHashForGraph(store, 7, defaultScope)
	if got != defaultScope.Hash() {
		t.Fatalf("expected default hash %d, got %d", defaultScope.Hash(), got)
	}
}
