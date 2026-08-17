package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestLocalEnumeratesEmptyIssuedHeadInCanonicalOrder(t *testing.T) {
	r := syntheticResult()
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	issueSyntheticComponents(r, loop)
	if !r.buildLocal() {
		t.Fatal("failed to build empty issued region")
	}
	local := r.Local()
	if local.Count() != 1 {
		t.Fatalf("empty issued region count = %d, want 1", local.Count())
	}
	region, ok := local.At(0)
	if !ok || region.SuccessorCount() != 0 || region.SiteCount() != 0 {
		t.Fatal("empty issued region was not enumerable")
	}
	if replay, ok := local.Resolve(region.ID()); !ok || replay.ID() != region.ID() {
		t.Fatal("canonical empty region identity failed to resolve")
	}
	if _, ok := local.At(1); ok {
		t.Fatal("region enumeration exposed a physical overflow row")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _ = local.At(0); _, _ = local.Resolve(region.ID()) }); allocs != 0 {
		t.Fatalf("local enumeration allocates %v times", allocs)
	}
}
