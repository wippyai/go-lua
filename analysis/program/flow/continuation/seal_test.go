package continuation_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func continuationFixtureCorpus(t *testing.T) *testfixture.Corpus {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("continuation test source location unavailable")
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := testfixture.LoadCorpus(repository)
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	return corpus
}

// TestContinuationSealEdgeMatrixEndpointsAreTotal checks the endpoint
// denominator at the canonical semantic edge-matrix boundary. The expected
// route total is derived independently from the sealed local Edge and
// CallBoundary planes. A same-point route is admissible only with its
// owner-issued Mu/reset witness; Mu-less self-referential candidates are
// classified as rejected rather than counted as semantic routes.
func TestContinuationSealEdgeMatrixEndpointsAreTotal(t *testing.T) {
	project, err := continuationFixtureCorpus(t).Project("semantic/type-engine-edge-matrix")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatal(err)
	}
	mounts := linked.Project().Mounts()
	if mounts.Count() != 1 {
		t.Fatalf("edge-matrix mount count = %d, want 1", mounts.Count())
	}
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("edge-matrix mount is unavailable")
	}
	program, ok := mounts.Program(shard)
	if !ok || program == nil {
		t.Fatal("edge-matrix Program is unavailable")
	}

	identity := program.Source().Identity()
	causal := program.Flow().Causal()
	successors := program.Flow().Causal().Successors()
	continuation := program.Flow().Continuation()
	endpoints := make(map[keyspace.Term]struct{})
	routeCount := successors.TotalCount()
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= uint32(identity.FamilyCount(family)); ordinal++ {
			from := keyspace.MakeTerm(family, ordinal)
			for index := 0; index < successors.Count(from); index++ {
				successor, successorOK := successors.At(from, index)
				if !successorOK {
					t.Fatalf("Causal Successors.At(%08x,%d) failed", uint32(from), index)
				}
				endpoints[successor.From] = struct{}{}
				endpoints[successor.To] = struct{}{}
			}
		}
	}
	// Recompute the admissible denominator from the two sealed owner planes,
	// rather than trusting the combined Successor index. Local self-edges are
	// admitted only when their Mu witness is present; CallBoundary has no
	// recurrence witness field, so a boundary endpoint equal to its Call is
	// rejected as an unproven self-reference.
	admitted, rejected := 0, 0
	edges := causal.Edges()
	for index := 0; index < edges.Count(); index++ {
		edge, ok := edges.At(index)
		if !ok {
			t.Fatalf("edge-matrix Causal Edge %d is unavailable", index)
		}
		if edge.From == edge.To && edge.Mu == 0 {
			rejected++
			continue
		}
		admitted++
	}
	boundaries := causal.Boundaries()
	for index := 0; index < boundaries.Count(); index++ {
		boundary, ok := boundaries.At(index)
		if !ok {
			t.Fatalf("edge-matrix CallBoundary %d is unavailable", index)
		}
		for _, endpoint := range []keyspace.Term{
			boundary.Normal, boundary.Other, boundary.TailReturn,
			boundary.Throw, boundary.Yield, boundary.Cancel,
		} {
			if endpoint == 0 {
				continue
			}
			if endpoint == boundary.Call {
				rejected++
				continue
			}
			admitted++
		}
	}
	if routeCount != admitted {
		t.Fatalf("edge-matrix Causal route total = %d, independently admissible = %d (rejected candidates = %d)", routeCount, admitted, rejected)
	}
	if rejected != 0 {
		t.Fatalf("edge-matrix published Mu-less self-referential candidates = %d", rejected)
	}
	for endpoint := range endpoints {
		count, available := continuation.GuardCount(endpoint)
		if !available || count < 0 {
			t.Fatalf("edge-matrix endpoint GuardCount(%08x) = %d/%v, want available", uint32(endpoint), count, available)
		}
		for index := 0; index < count; index++ {
			guard, guardOK := continuation.GuardAt(endpoint, index)
			guardFamily := keyspace.TermFamily(guard)
			if !guardOK || (guardFamily != keyspace.FamilySelect && guardFamily != keyspace.FamilyBranch && guardFamily != keyspace.FamilyLoop) {
				t.Fatalf("edge-matrix endpoint GuardAt(%08x,%d) = %08x/%v, want existing decision", uint32(endpoint), index, uint32(guard), guardOK)
			}
		}
		if _, guardOK := continuation.GuardAt(endpoint, count); guardOK {
			t.Fatalf("edge-matrix endpoint GuardAt(%08x,%d) crossed exact scope", uint32(endpoint), count)
		}
	}
}
