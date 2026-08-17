package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/analysis/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func portableAnyType() schematype.Type {
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		panic("portable any type")
	}
	return value
}

type directAllocationSubjectFixture struct {
	values      *valuedomain.Schema
	packs       *packdomain.Schema
	heaps       heapdomain.Schema
	allocation  *valuedomain.AllocationResult
	other       *valuedomain.AllocationResult
	source      packdomain.SemanticSource
	otherSource packdomain.SemanticSource
}

func directAllocationSubjectContract(t testing.TB) (*target.Contract, target.Operation) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"send"}}},
		Input:    target.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil || contract == nil {
		t.Fatalf("seal direct allocation Target: %v", err)
	}
	operation, operationOK := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"send"}})
	if !operationOK {
		t.Fatal("direct allocation operation")
	}
	return contract, operation
}

func directAllocationSubjectFixtureFor(t testing.TB, label string) directAllocationSubjectFixture {
	t.Helper()
	contract, operation := directAllocationSubjectContract(t)
	// Both literal allocations reach the method call directly. The receiver is
	// formal 0 and therefore names its allocation root itself; the argument is
	// a distinct literal at formal 1 for the negative allocation/source cases.
	// A named local would be a storage-alias coordinate and must not satisfy the
	// direct-identity receipt.
	published, err := lower.Lower(lower.Source{Name: "direct_allocation_subject_" + label + ".lua", Text: []byte("({}):send({})\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "direct_allocation_subject_" + label, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := composite.Global()
	if !grammarOK {
		t.Fatal("direct allocation program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("direct allocation mount")
	}
	artifact, failure := composite.CompileArtifactDetailed(program, grammar)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile direct allocation artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), []*programartifact.Artifact{artifact})
	if err != nil || types == nil {
		t.Fatalf("seal direct allocation types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedArtifact{{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal direct allocation static: %v", err)
	}
	packMount, packMountOK := packdomain.NewArtifactMount(artifact, module, programID)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(artifact, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(artifact, module, programID)
	if !packMountOK || !heapMountOK || !valueMountOK {
		t.Fatal("direct allocation artifact mounts")
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []packdomain.ArtifactMount{packMount})
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount})
	if !packsOK || packs == nil || heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal direct allocation schemas pack=%t heap=%s value=%s", packsOK, heapFailure, valueFailure)
	}
	selector, selectorOK := packs.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0})
	otherSelector, otherSelectorOK := packs.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	if !selectorOK || !otherSelectorOK {
		t.Fatal("direct allocation fixed selectors")
	}
	var callID identity.ContentID
	for index := 0; index < artifact.CallCount(); index++ {
		call, callOK := artifact.CallAt(index)
		if callOK && call.Form() == flow.CallFormMethod {
			callID = call.ID()
			break
		}
	}
	if !callID.Available() {
		t.Fatal("direct allocation call")
	}
	source, sourceOK := packs.MountedInputSemanticSource(module, callID, selector)
	otherSource, otherSourceOK := packs.MountedInputSemanticSource(module, callID, otherSelector)
	if !sourceOK || !otherSourceOK || !packs.OwnsSemanticSource(source) || !packs.OwnsSemanticSource(otherSource) || source.Same(otherSource) {
		t.Fatal("direct allocation source")
	}
	fixture := directAllocationSubjectFixture{values: values, packs: packs, heaps: heaps, source: source, otherSource: otherSource}
	rootCount, resultCount, directCount := 0, 0, 0
	for index := 0; index < heaps.KeyCount(); index++ {
		key, keyOK := heaps.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		rootCount++
		allocation, allocationOK := values.AllocationResultFor(key)
		if !allocationOK {
			continue
		}
		resultCount++
		if _, directOK := values.DirectAllocationSubjectFor(packs, source, allocation); directOK {
			fixture.allocation = allocation
			directCount++
		} else if fixture.other == nil {
			fixture.other = allocation
		}
	}
	if fixture.allocation == nil || fixture.other == nil {
		t.Fatalf("direct and distinct allocation results roots=%d results=%d direct=%d", rootCount, resultCount, directCount)
	}
	return fixture
}

func TestDirectAllocationSubjectFencesValuePackAndAllocation(t *testing.T) {
	left := directAllocationSubjectFixtureFor(t, "equal")
	right := directAllocationSubjectFixtureFor(t, "equal")
	foreign := directAllocationSubjectFixtureFor(t, "foreign")
	if left.heaps.ContentID() != right.heaps.ContentID() {
		t.Fatal("equal-content Heap schemas")
	}
	direct, directOK := left.values.DirectAllocationSubjectFor(left.packs, left.source, left.allocation)
	rightDirect, rightDirectOK := right.values.DirectAllocationSubjectFor(right.packs, right.source, right.allocation)
	leftID, leftIDOK := direct.ContentID()
	rightID, rightIDOK := rightDirect.ContentID()
	if !directOK || !rightDirectOK || !leftIDOK || !rightIDOK || !direct.Valid() || !rightDirect.Valid() || leftID != rightID {
		t.Fatal("equal-content direct identity receipt")
	}
	if _, accepted := left.values.DirectAllocationSubjectFor(right.packs, right.source, left.allocation); accepted {
		t.Fatal("foreign equal-content Pack/Value entered direct identity")
	}
	if _, accepted := left.values.DirectAllocationSubjectFor(left.packs, foreign.source, left.allocation); accepted {
		t.Fatal("nonmember Pack source entered direct identity")
	}
	if _, accepted := left.values.DirectAllocationSubjectFor(left.packs, left.otherSource, left.allocation); accepted {
		t.Fatal("different valid local Pack source entered direct identity")
	}
	if _, accepted := left.values.DirectAllocationSubjectFor(left.packs, left.source, right.allocation); accepted {
		t.Fatal("foreign Value AllocationResult entered direct identity")
	}
	if _, accepted := left.values.DirectAllocationSubjectFor(left.packs, left.source, left.other); accepted {
		t.Fatal("different allocation entered direct identity")
	}
}
