package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestTransientPathEqualityClosesProofsWithoutPublishingSyntheticIdentity(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	anchor := mustStateKey(t, keys, pathdom.PathKey("sym910@1"))
	left := mustStateKey(t, keys, pathdom.PathKey("sym911@1"))
	right := mustStateKey(t, keys, pathdom.PathKey("sym912@1"))
	existing := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: anchor, Other: left}
	synthetic := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: left, Other: right}
	input := Reachable(State{}).AddBranchProof(existing)
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("path-evidence coordinate family missing")
	}
	factors, err := domain.DecomposeLanes(input, []ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	union := make([]CoordinateSlot, len(scalars))
	for index := range scalars {
		union[index] = scalars[index].Slot()
	}
	plan, err := domain.SealPathEqualityFactorPlan(keys, left, right, union)
	if err != nil {
		t.Fatal(err)
	}
	authority := sealTestPathEvidenceAuthority(
		t, domain, keys, nil, nil, plan.CoordinateReads(), plan.CoordinateWrites(), false, true,
	)
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, ValueLaneFactor{}, true, authority, PathDescendantMutationFactors{},
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.PrepareCoordinateTransientPathEqualityTransaction(carrier, synthetic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ApplyCoordinatePathEqualityTransaction(transaction, carrier); err != nil {
		t.Fatal(err)
	}
	if carrier.HasProof(synthetic) {
		t.Fatal("transient value equality escaped as persistent path identity")
	}
	if !carrier.HasProof(existing) {
		t.Fatal("transient equality removed an existing proof")
	}
}
