package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func TestStaticMemberFactorMatchesCanonicalConcretePublication(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	target, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentIndexString, Name: "name"}})
	if !ok {
		t.Fatal("target key")
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	input := Reachable(State{}).WriteLocalPathStaticMember(target, product.Top())
	domain := RegisteredProductDomain(reg)
	plan, err := domain.PrepareStaticMemberFactorPlan(keys, target, value)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyStaticMember(plan, input)
	if err != nil {
		t.Fatal(err)
	}
	want := input.WriteLocalPathStaticMember(target, value)
	if canonical, mirrored := keys.FieldCanonical(target); mirrored {
		want = want.WriteLocalPathStaticMember(canonical, value)
	}
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native static publication diverged from canonical State writes")
	}
}
