package continuation_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/internal/testfixture"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/targetprofile"
)

// continuationFixtureCorpus loads the checked-in fixture corpus for one test.
// The repository root is derived from this source file, so the census is
// independent of the working directory a test runs in.
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

// TestContinuationSealEdgeMatrixEndpointsAreTotal fixes the endpoint
// denominator at the canonical semantic edge-matrix boundary. Causal emits
// exactly 5,317 semantic routes there; every existing From/To term must have
// an owner-local unpolarized Guard scope, including an available empty scope.
func TestContinuationSealEdgeMatrixEndpointsAreTotal(t *testing.T) {
	project, err := continuationFixtureCorpus(t).Project("semantic/type-engine-edge-matrix")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
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
	successors := program.Flow().Causal().Successors()
	continuation := program.Flow().Continuation()
	endpoints := make(map[keyspace.Term]struct{})
	routeCount := 0
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= uint32(identity.FamilyCount(family)); ordinal++ {
			from := keyspace.MakeTerm(family, ordinal)
			for index := 0; index < successors.Count(from); index++ {
				successor, successorOK := successors.At(from, index)
				if !successorOK {
					t.Fatalf("Causal Successors.At(%08x,%d) failed", uint32(from), index)
				}
				routeCount++
				endpoints[successor.From] = struct{}{}
				endpoints[successor.To] = struct{}{}
			}
		}
	}
	if routeCount != 5317 {
		t.Fatalf("edge-matrix Causal route count = %d, want 5317", routeCount)
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
