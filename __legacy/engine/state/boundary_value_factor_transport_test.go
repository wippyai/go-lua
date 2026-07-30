package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type boundaryFormalLikeSlot struct {
	owner lexicalidentity.StableLexicalBodyID
	index uint32
}

func TestGenericBoundaryValuesRelationPreservesFullSlotIdentityAndLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	plan, template, target := boundaryGenericValueTestPlan(t)
	ownerA := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-slot-a")), 1)
	ownerB := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-slot-b")), 1)
	left := boundaryFormalLikeSlot{owner: ownerA, index: 7}
	right := boundaryFormalLikeSlot{owner: ownerB, index: 7}
	missing := boundaryFormalLikeSlot{owner: ownerA, index: 99}
	joined := boundaryFormalLikeSlot{owner: ownerA, index: 8}
	leftTarget := boundaryFormalLikeSlot{owner: ownerB, index: 1}
	rightTarget := boundaryFormalLikeSlot{owner: ownerB, index: 2}
	joinTarget := boundaryFormalLikeSlot{owner: ownerB, index: 3}

	bindings := []BoundaryValueSlotBinding[boundaryFormalLikeSlot, boundaryFormalLikeSlot]{
		{Source: right, Target: rightTarget},
		{Source: joined, Target: joinTarget},
		{Source: left, Target: leftTarget},
		{Source: right, Target: joinTarget},
	}
	targetOrder := []boundaryFormalLikeSlot{leftTarget, rightTarget, joinTarget}
	relation, err := SealBoundaryValueSlotRelation([]boundaryFormalLikeSlot{left, right, joined}, targetOrder, bindings)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := PrepareBoundaryValueFactorTransport(plan, relation)
	if err != nil {
		t.Fatal(err)
	}
	allocation := product.Set(reg, product.Top(), identity.Key, identity.SingletonTerm(identity.AllocationTerm(template)))
	integer := typevalue.LiteralInt(reg, 7)
	stringValue := typevalue.LiteralString(reg, "seven")
	source := ValueFactor[boundaryFormalLikeSlot]{Values: map[boundaryFormalLikeSlot]product.Value{
		left: allocation, right: integer, joined: stringValue, missing: product.Top(),
	}}
	got, err := transport.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Values) != 3 {
		t.Fatalf("generic Values width = %d, want three exact targets: %#v", len(got.Values), got.Values)
	}
	if _, leaked := got.Values[missing]; leaked {
		t.Fatal("unrelated same-ordinal formal slot leaked through structural relation")
	}
	if mappedID, exact := product.Get(reg, got.Values[leftTarget], identity.Key).ID(); !exact || mappedID != target {
		t.Fatalf("generic Values identity rebase = %v/%t, want %v", mappedID, exact, target)
	}
	if !product.Equal(reg, got.Values[rightTarget], product.ProjectBoundary(reg, integer)) {
		t.Fatal("same-ordinal slots from distinct lexical owners aliased")
	}
	wantJoin := product.Join(reg, product.ProjectBoundary(reg, integer), product.ProjectBoundary(reg, stringValue))
	if !product.Equal(reg, got.Values[joinTarget], wantJoin) {
		t.Fatalf("many-to-one Values collision = %#v, want registered Join %#v", got.Values[joinTarget], wantJoin)
	}

	top, err := transport.Apply(ValueFactor[boundaryFormalLikeSlot]{Top: true, Values: source.Values})
	if err != nil || !top.Top || len(top.Values) != 0 {
		t.Fatalf("Values Top transport = %#v, %v", top, err)
	}
	bottom, err := transport.Apply(ValueFactor[boundaryFormalLikeSlot]{})
	if err != nil || bottom.Top || len(bottom.Values) != 0 {
		t.Fatalf("Values Bottom transport = %#v, %v", bottom, err)
	}

	reversed := append([]BoundaryValueSlotBinding[boundaryFormalLikeSlot, boundaryFormalLikeSlot](nil), bindings...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	relation2, err := SealBoundaryValueSlotRelation([]boundaryFormalLikeSlot{left, right, joined}, targetOrder, reversed)
	if err != nil {
		t.Fatal(err)
	}
	transport2, err := PrepareBoundaryValueFactorTransport(plan, relation2)
	if err != nil {
		t.Fatal(err)
	}
	permuted, err := transport2.Apply(source)
	valueLattice := ValueFactorLattice[boundaryFormalLikeSlot](reg)
	if err != nil || !valueLattice.Equal(got, permuted) {
		t.Fatalf("binding permutation changed generic Values result: equal=%t err=%v", valueLattice.Equal(got, permuted), err)
	}
	for slot, value := range got.Values {
		if !product.Domain(reg).Same(value, permuted.Values[slot]) {
			t.Fatalf("binding permutation changed retained product spelling at %#v", slot)
		}
	}
}

func boundaryGenericValueTestPlan(t *testing.T) (BoundaryFactorTransportPlan, identity.AllocationTemplate, identity.ID) {
	t.Helper()
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callee := lexicalidentity.FunctionBody(namespace, 1)
	caller := lexicalidentity.RootBody(namespace)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 7, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	target, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation authority omitted target")
	}
	transport, err := authority.BindTransport(to, nil, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(from, nil, []identity.Term{identity.ConcreteTerm(target)}, false)
	if err != nil {
		t.Fatal(err)
	}
	selection.closure.identities[identity.AllocationTerm(template)] = struct{}{}
	companionLane, ok := domain.BoundaryClosureCompanion()
	if !ok {
		t.Fatal("registered product omitted boundary closure companion")
	}
	companionFactor, err := domain.LaneBottom(companionLane)
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, &companionFactor)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}
	return plan, template, target
}
