package semantic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

func TestAscentOrderedContributionAdmitsDefinedWiden(t *testing.T) {
	fixture := newSemanticFixture(t)
	join := func(left, right uint8) (uint8, bool) { return left | right, true }
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:     0,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        join,
		Widen:       join,
		LessOrEq:    func(left, right uint8) bool { return left&right == left },
	})
	if !ok {
		t.Fatal("bit-inclusion domain")
	}
	left := underPlane(t, domain, fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{when: fixture.all, value: 1}))
	right := underPlane(t, domain, fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{when: fixture.all, value: 2}))
	regions := func(key semanticKey) (support.Mask, bool) {
		if key != 7 {
			return support.Mask{}, false
		}
		return fixture.all, true
	}
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	if domain.LessOrEqContribution(left, right, regions, scratch) || domain.LessOrEqContribution(right, left, regions, scratch) {
		t.Fatal("incomparable bit cells were inclusion-ordered")
	}
	if !domain.AscentOrderedContribution(left, right, regions, scratch) || !domain.AscentOrderedContribution(right, left, regions, scratch) {
		t.Fatal("defined Widen did not admit the incomparable replacement")
	}
	if !domain.AscentOrderedContribution(left, left, regions, scratch) {
		t.Fatal("identical cells were not ascent-ordered")
	}
}
