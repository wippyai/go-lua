package value_test

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// allocationMembershipFixture seals exactly the Value and Heap halves this law
// reads. Membership is a Value relation over a Value-owned AllocationResult, so
// the fixture deliberately mounts no other domain.
type allocationMembershipFixture struct {
	values *valuedomain.Schema
	heaps  heapdomain.Schema
	first  *valuedomain.AllocationResult
	second *valuedomain.AllocationResult
}

func allocationMembershipContract(t testing.TB) *target.Contract {
	t.Helper()
	value, primitiveOK := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !primitiveOK {
		t.Fatal("allocation membership portable any type")
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{value, value}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil || contract == nil {
		t.Fatalf("seal allocation membership Target: %v", err)
	}
	return contract
}

func allocationMembershipFixtureFor(t testing.TB, label string) allocationMembershipFixture {
	t.Helper()
	// Two distinct table literals give two distinct root allocations, which is
	// what a membership classification must keep apart.
	published, err := lower.Lower(lower.Source{Name: "allocation_membership_" + label + ".lua", Text: []byte("({}):send({})\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: allocationMembershipContract(t), Modules: []linkproject.Module{{Name: "allocation_membership_" + label, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := composite.Global()
	if !grammarOK {
		t.Fatal("allocation membership program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("allocation membership mount")
	}
	artifact, failure := composite.CompileArtifactDetailed(program, grammar)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile allocation membership artifact: %s", failure.Error())
	}
	heapMount, heapMountOK := heapdomain.NewArtifactMount(artifact, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(artifact, module, programID)
	if !heapMountOK || !valueMountOK {
		t.Fatal("allocation membership artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount})
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal allocation membership schemas heap=%s value=%s", heapFailure, valueFailure)
	}
	fixture := allocationMembershipFixture{values: values, heaps: heaps}
	for index := 0; index < heaps.KeyCount(); index++ {
		key, keyOK := heaps.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		allocation, allocationOK := values.AllocationResultFor(key)
		if !allocationOK {
			continue
		}
		if fixture.first == nil {
			fixture.first = allocation
			continue
		}
		fixture.second = allocation
		break
	}
	if fixture.first == nil || fixture.second == nil {
		t.Fatal("allocation membership distinct allocation results")
	}
	return fixture
}

// TestAllocationMembershipIsExactAndCoordinateFenced records the deliberately
// narrow Phase3D Value surface. Recent/Summary membership is meaningful only
// for the canonical AllocationResult and its exact Value owner; it is not an
// alias or uniqueness proof.
func TestAllocationMembershipIsExactAndCoordinateFenced(t *testing.T) {
	fixture := allocationMembershipFixtureFor(t, "membership")
	// Re-seal the identical input independently. Scalar schema identities must
	// match while Value owners remain distinct live authorities.
	foreign := allocationMembershipFixtureFor(t, "membership")
	if fixture.heaps.ContentID() != foreign.heaps.ContentID() {
		t.Fatal("equal-content foreign Heap fixture")
	}
	recent, recentOK := fixture.first.Fresh()
	summary, summaryOK := fixture.first.Age(recent)
	other, otherOK := fixture.second.Fresh()
	mixed, mixedOK := fixture.values.Join(recent, other)
	recentAndSummary, recentAndSummaryOK := fixture.values.Join(recent, summary)
	foreignRecent, foreignRecentOK := foreign.first.Fresh()
	if !recentOK || !summaryOK || !otherOK || !mixedOK || !recentAndSummaryOK || !foreignRecentOK {
		t.Fatal("allocation membership fixture")
	}

	cases := []struct {
		name string
		fact valuedomain.Value
		want valuedomain.AllocationMembership
	}{
		{name: "recent", fact: recent, want: valuedomain.MembershipRecent},
		{name: "summary", fact: summary, want: valuedomain.MembershipSummary},
		{name: "top", fact: fixture.values.Top(), want: valuedomain.MembershipMixedOrUnknown},
		{name: "bottom", fact: fixture.values.Bottom(), want: valuedomain.MembershipMixedOrUnknown},
		{name: "mixed", fact: mixed, want: valuedomain.MembershipMixedOrUnknown},
		{name: "same-key-recent-summary", fact: recentAndSummary, want: valuedomain.MembershipMixedOrUnknown},
		{name: "different-allocation", fact: other, want: valuedomain.MembershipMixedOrUnknown},
	}
	for _, test := range cases {
		got, classified := fixture.first.ClassifyMembership(test.fact)
		if !classified || got != test.want {
			t.Fatalf("membership %s got=%d classified=%t want=%d", test.name, got, classified, test.want)
		}
	}
	if got, classified := fixture.first.ClassifyMembership(foreign.values.Top()); classified || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("foreign Value owner entered allocation membership classification")
	}
	if got, classified := fixture.first.ClassifyMembership(foreignRecent); classified || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("equal-content foreign Value/allocation result entered local classification")
	}
	if got, classified := foreign.first.ClassifyMembership(recent); classified || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("local Value fact entered equal-content foreign allocation result")
	}
}
