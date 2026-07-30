package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func TestTypestateQueryCapabilityMatchesConcretePathEqualityQuotient(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	primary := pathdom.PathKey("sym810@1.resource")
	alias := pathdom.PathKey("sym811@1.alias")
	primaryKey := mustStateKey(t, keys, primary)
	aliasKey := mustStateKey(t, keys, alias)
	protocol := typestate.Protocol("test.protocol")
	resource := TypestateResourceFromCanonicalKey(testStateKey(t, primary), protocol)
	input := Reachable(State{}).
		AcquireTypestate(resource, typestate.State("open"), typestate.Obligation{Final: typestate.State("closed")}).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: primaryKey, Other: aliasKey})

	capability, err := domain.SealTypestateQueryCapability(keys)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(input, []ProductLane{capability.TypestateLane(), capability.PathEqualityLane()})
	if err != nil {
		t.Fatal(err)
	}
	gotResource, gotSlot, found, err := domain.CanonicalTypestateResourceFactor(
		capability, factors[0], factors[1], testStateKey(t, alias), protocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantResource := input.CanonicalTypestateResource(keys, testStateKey(t, alias), protocol)
	wantSlot, wantFound := input.TypestateSlot(wantResource)
	if !found || !wantFound || gotResource != wantResource || gotSlot != wantSlot {
		t.Fatalf("factor query = (%#v,%#v,%t), concrete = (%#v,%#v,%t)", gotResource, gotSlot, found, wantResource, wantSlot, wantFound)
	}
	obligations, err := domain.OpenTypestateObligationsFactor(capability, factors[0], factors[1])
	if err != nil || len(obligations) != 1 || obligations[0].Resource != wantResource {
		t.Fatalf("factor obligations = %#v, err=%v", obligations, err)
	}
}

func TestTypestateResourceQueryIgnoresUnrelatedLifecycleAndEqualityFacts(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	receiver := pathdom.PathKey("sym820@1.receiver")
	alias := pathdom.PathKey("sym821@1.alias")
	unrelated := pathdom.PathKey("sym822@1.unrelated")
	unrelatedAlias := pathdom.PathKey("sym823@1.unrelated_alias")
	receiverKey := mustStateKey(t, keys, receiver)
	aliasKey := mustStateKey(t, keys, alias)
	unrelatedKey := mustStateKey(t, keys, unrelated)
	unrelatedAliasKey := mustStateKey(t, keys, unrelatedAlias)
	protocol := typestate.Protocol("channel.lifecycle")
	capability, err := domain.SealTypestateQueryCapability(keys)
	if err != nil {
		t.Fatal(err)
	}
	query, err := SealTypestateResourceQuery(domain, capability, testStateKey(t, alias), protocol)
	if err != nil {
		t.Fatal(err)
	}
	want := typestate.Slot{Current: typestate.State("closed"), Locality: typestate.LocalityOpen}
	base := Reachable(State{}).
		AcquireTypestate(TypestateResourceFromCanonicalKey(testStateKey(t, receiver), protocol), typestate.State("closed"), typestate.Obligation{}).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: receiverKey, Other: aliasKey})
	withUnrelated := base.
		AcquireTypestate(TypestateResourceFromCanonicalKey(testStateKey(t, unrelated), protocol), typestate.State("open"), typestate.Obligation{Final: typestate.State("closed")}).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: unrelatedKey, Other: unrelatedAliasKey})

	left, err := query.ObserveState(domain, base)
	if err != nil {
		t.Fatal(err)
	}
	right, err := query.ObserveState(domain, withUnrelated)
	if err != nil {
		t.Fatal(err)
	}
	if !left.Equal(right) {
		t.Fatalf("unrelated lifecycle/path decisions changed keyed observation: left=%#v right=%#v", left, right)
	}
	if got, found := left.Slot(); !found || got != want {
		t.Fatalf("keyed observation = (%#v,%t), want (%#v,true)", got, found, want)
	}
}
