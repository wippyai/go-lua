package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestGenericForIdentityPublicationFreezesCanonicalCarrySource(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	target := symbol.ID(901)
	fallback := arena.bindEnvironmentSymbol(target)
	iterator := arena.Constant(product.Top())
	// The concrete iterator value is immaterial to this ownership test: the
	// canonical projection constructor and its exact fallback are the source of
	// the descriptor.
	nilTerm := arena.Constant(typevalue.Nil(reg))
	projection := arena.genericForResultValue(0, iterator, nilTerm, nilTerm, fallback)
	publication, err := sealGenericForIdentityPublication(arena, statekey.SymbolValue(target), projection)
	if err != nil {
		t.Fatal(err)
	}
	if !publication.valid(arena, Shape{}) || publication.projection != projection ||
		publication.target != statekey.SymbolValue(target) || publication.projectionIdentity != genericForProjectionIdentityNoFinite ||
		len(publication.finiteSources) != 1 || publication.finiteSources[0] != fallback {
		t.Fatalf("frozen GenericFor identity publication = %#v", publication)
	}
	clone := publication.clone()
	publication.finiteSources[0] = nilTerm
	if clone.finiteSources[0] != fallback {
		t.Fatal("GenericFor identity publication retained caller-owned source storage")
	}
}

func TestGenericForIdentityPublicationMakesEmptyFiniteSupportExplicit(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	target := symbol.ID(902)
	arena.bindEnvironmentSymbol(target)
	source := arena.Constant(product.Top())
	projection := arena.IteratorProjectionValue(iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}, 0, source)
	publication, err := sealGenericForIdentityPublication(arena, statekey.SymbolValue(target), projection)
	if err != nil || !publication.valid(arena, Shape{}) {
		t.Fatalf("explicit no-finite GenericFor publication = %#v, %v", publication, err)
	}
	if publication.projectionIdentity != genericForProjectionIdentityNoFinite || len(publication.finiteSources) != 0 || !publication.sealed {
		t.Fatalf("empty finite support was not explicit: %#v", publication)
	}
}
