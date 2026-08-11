package semantic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestReindexJoinsReachableFibersAndExcludesOffSupport(t *testing.T) {
	fixture, domain := newUnderDomain(t)
	manager := fixture.diagram.Guards()
	target, ok := manager.SealScope(nil)
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := manager.NewReindex(manager.AllScope(), target)
	if !ok || !builder.Forget(guard.Atom(1)) || !builder.Forget(guard.Atom(2)) {
		t.Fatal("forget builder")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("forget plan")
	}
	input := underPlane(t, domain, fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.notAtom, value: 10},
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.atom, value: 20},
	))
	joined, ok := domain.Reindex(input, fixture.all, fixture.all, plan)
	if !ok {
		t.Fatal("joined reindex")
	}
	if got, present := fixture.at(t, joined, false); !present || got != 20 {
		t.Fatalf("joined fiber = %d/%t, want 20/true", got, present)
	}

	hidden := underPlane(t, domain, fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.atom, value: 20},
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.notAtom, value: 60},
	))
	filtered, ok := domain.Reindex(hidden, fixture.atom, fixture.all, plan)
	if !ok {
		t.Fatal("filtered reindex")
	}
	if got, present := fixture.at(t, filtered, false); !present || got != 20 {
		t.Fatalf("off-support value polluted target fiber = %d/%t, want 20/true", got, present)
	}
}
