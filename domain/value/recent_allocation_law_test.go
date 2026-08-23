package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestExactRecentAllocationAcceptsOnlyOneOwnedRecentAllocation(t *testing.T) {
	heaps, values := recentAllocationFixture(t)

	var allocation heapdomain.Key
	var recent, summary valuedomain.Value
	for index := 0; index < heaps.KeyCount(); index++ {
		candidate, candidateOK := heaps.KeyAt(index)
		if !candidateOK || candidate.Kind() != heapdomain.RootAllocation {
			continue
		}
		atom, atomOK := values.Allocation(candidate, materialization.Recent)
		summaryAtom, summaryOK := values.Allocation(candidate, materialization.Summary)
		if !atomOK || !summaryOK {
			continue
		}
		recent, atomOK = values.Singleton(atom)
		summary, summaryOK = values.Singleton(summaryAtom)
		if atomOK && summaryOK {
			allocation = candidate
			break
		}
	}
	if !allocation.Valid() {
		t.Fatal("fixture has no allocation with Recent and Summary Value atoms")
	}

	var scalar valuedomain.Value
	var rootedNonAllocation valuedomain.Value
	var scalarFound, rootedNonAllocationFound bool
	var atoms []valuedomain.Atom
	atomsOK := values.VisitSupport(values.Top(), func(atom valuedomain.Atom) {
		atoms = append(atoms, atom)
	})
	if !atomsOK {
		t.Fatal("enumerate Value atoms")
	}
	for _, atom := range atoms {
		fact, factOK := values.Singleton(atom)
		if !factOK {
			continue
		}
		if _, scalarOK := values.ExactScalar(fact); scalarOK && !scalarFound {
			scalar = fact
			scalarFound = true
		}
		reference, role, referenceOK := atom.Reference()
		_, allocationReference := reference.AllocationKey()
		if referenceOK && role == materialization.Exact && !allocationReference && !rootedNonAllocationFound {
			rootedNonAllocation = fact
			rootedNonAllocationFound = true
		}
	}
	if !scalarFound || !rootedNonAllocationFound {
		t.Fatalf("fixture scalar/rooted non-allocation facts: scalar=%t rooted=%t", scalarFound, rootedNonAllocationFound)
	}

	recentAtom, recentOK := values.Allocation(allocation, materialization.Recent)
	if !recentOK {
		t.Fatal("recover Recent atom")
	}
	multi, multiOK := values.Alternatives(recentAtom, mustAtom(t, values, scalar))
	if !multiOK {
		t.Fatal("construct multi-atom union")
	}

	tests := []struct {
		name    string
		fact    valuedomain.Value
		present bool
		want    bool
	}{
		{name: "Recent exact", fact: recent, present: true, want: true},
		{name: "Summary", fact: summary, present: true},
		{name: "absent", fact: recent, present: false},
		{name: "Bottom", fact: values.Bottom(), present: true},
		{name: "Top", fact: values.Top(), present: true},
		{name: "scalar", fact: scalar, present: true},
		{name: "multi-atom union", fact: multi, present: true},
		{name: "rooted non-allocation", fact: rootedNonAllocation, present: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := values.ExactRecentAllocation(test.fact, test.present)
			if ok != test.want {
				t.Fatalf("ExactRecentAllocation ok = %t, want %t", ok, test.want)
			}
			if test.want && got != allocation {
				t.Fatalf("ExactRecentAllocation key = %#v, want %#v", got, allocation)
			}
			if !test.want && got.Valid() {
				t.Fatalf("rejected fact returned valid key %#v", got)
			}
		})
	}
}

// TestAllocationCarryAgeUsesTheIssuedCandidateSemantics exercises the direct
// candidate transform itself. The assertion names the absolute owner result
// (the selected Recent reference becomes Summary); it does not compare two
// independently implemented Age authorities.
func TestAllocationCarryAgeUsesTheIssuedCandidateSemantics(t *testing.T) {
	heaps, values := recentAllocationFixture(t)
	var key heapdomain.Key
	var result *valuedomain.AllocationResult
	for index := 0; index < heaps.KeyCount(); index++ {
		candidate, candidateOK := heaps.KeyAt(index)
		if !candidateOK || candidate.Kind() != heapdomain.RootAllocation {
			continue
		}
		row, rowOK := values.AllocationResultFor(candidate)
		if rowOK {
			key, result = candidate, row
			break
		}
	}
	if !key.Valid() || result == nil {
		t.Fatal("fixture has no issued allocation candidate")
	}
	recentAtom, recentOK := values.Allocation(key, materialization.Recent)
	prior, priorOK := values.Singleton(recentAtom)
	if !recentOK || !priorOK {
		t.Fatal("allocation Recent prior")
	}
	aged, agedOK := result.Age(prior)
	if !agedOK {
		t.Fatal("issued allocation candidate did not age its prior")
	}
	if !hasAllocationRole(values, aged, key, materialization.Summary) || hasAllocationRole(values, aged, key, materialization.Recent) {
		t.Fatal("allocation carry did not replace the selected Recent reference with Summary")
	}
	var stale valuedomain.AllocationResult
	if _, staleOK := stale.Age(prior); staleOK {
		t.Fatal("stale allocation candidate crossed the owner fence")
	}
	foreignHeaps, foreignValues := recentAllocationFixture(t)
	var foreign *valuedomain.AllocationResult
	for index := 0; index < foreignHeaps.KeyCount(); index++ {
		candidate, candidateOK := foreignHeaps.KeyAt(index)
		if !candidateOK || candidate.Kind() != heapdomain.RootAllocation {
			continue
		}
		row, rowOK := foreignValues.AllocationResultFor(candidate)
		if rowOK {
			foreign = row
			break
		}
	}
	if foreign == nil {
		t.Fatal("foreign allocation candidate")
	}
	if _, foreignPriorOK := result.Age(foreignValues.Top()); foreignPriorOK {
		t.Fatal("allocation candidate accepted a foreign prior fact")
	}
	if _, foreignCandidateOK := foreign.Age(prior); foreignCandidateOK {
		t.Fatal("foreign allocation candidate accepted a local prior fact")
	}
}

func hasAllocationRole(schema *valuedomain.Schema, fact valuedomain.Value, want heapdomain.Key, role materialization.Role) bool {
	atoms, ok := schema.Atoms(fact)
	if !ok {
		return false
	}
	for _, atom := range atoms {
		reference, gotRole, referenceOK := atom.Reference()
		key, keyOK := reference.AllocationKey()
		if referenceOK && keyOK && key == want && gotRole == role {
			return true
		}
	}
	return false
}

func mustAtom(t testing.TB, schema *valuedomain.Schema, fact valuedomain.Value) valuedomain.Atom {
	t.Helper()
	atoms, ok := schema.Atoms(fact)
	if !ok || len(atoms) != 1 {
		t.Fatal("fact is not a singleton atom")
	}
	return atoms[0]
}

func recentAllocationFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema) {
	return sealValueSource(t, "exact_recent_allocation.lua", "return {}\n")
}

// sealValueSource seals one source to the altitude Value's directories are
// published at. It is the same construction recentAllocationFixture always
// did, with the source named by the caller so a law can ask for a program that
// actually seals the rows it is about.
func sealValueSource(t testing.TB, name, source string) (heapdomain.Schema, *valuedomain.Schema) {
	t.Helper()
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	structural, structuralOK := composite.StructureVocabulary(compilation)
	contract, contractErr := testfixture.StandardLibraryTarget()
	if contractErr != nil {
		t.Fatal(contractErr)
	}
	linked, linkErr := testfixture.SealSource(contract, name, []byte(source))
	if linkErr != nil {
		t.Fatal(linkErr)
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := mounts.ProgramID(shard)
	if !compilationOK || !grammar.Available() || !issuanceOK || !structuralOK || mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("exact Recent fixture mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile exact Recent fixture: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !heapMountOK || !valueMountOK {
		t.Fatal("exact Recent artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("exact Recent heap seal: %s", heapFailure)
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []programmount.MountedArtifact{valueMount}, structural)
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("exact Recent Value seal: %s", valueFailure)
	}
	return heaps, values
}
