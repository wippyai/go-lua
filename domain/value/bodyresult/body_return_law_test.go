package bodyresult

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/bodyresult/returnroute"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// bodyResultSubject has two executable bodies with distinct two-value return
// paths and one fixed-two-result call site. A Call fact can select both body
// targets, so the rule must join both canonical first return members.
const bodyResultSubject = "local function left()\n" +
	"\treturn 1, \"left\"\n" +
	"end\n" +
	"local function right()\n" +
	"\treturn 2, \"right\"\n" +
	"end\n" +
	"local first, second = left()\n" +
	"return first, second\n"

type bodyResultLawFixture struct {
	linked   *link.Link
	contract *contract.Contract
	snapshot *ingress.Snapshot
	values   *valuedomain.Schema
	calls    *calldomain.Algebra
	module   identity.ContentID
	call     identity.ContentID
	bodies   []calldomain.Body
	fact     calldomain.Value
	result0  valuedomain.MountedCallResultSlot
	result1  valuedomain.MountedCallResultSlot
}

// TestBodyResultPlanKeepsCanonicalBodyReturnBoundary proves that the rule's
// complete input set is Value's sealed body index. It retains the canonical
// row rather than reconstructing a Program return relation downstream.
func TestBodyResultPlanKeepsCanonicalBodyReturnBoundary(t *testing.T) {
	fixture := newBodyResultLawFixture(t, "body_result_plan")
	if len(fixture.bodies) != 2 {
		t.Fatalf("selected body count = %d, want two", len(fixture.bodies))
	}
	selectedBody := fixture.bodies[0]
	module, moduleOK := selectedBody.ModuleKey()
	body, bodyOK := selectedBody.BodyPath()
	canonical, canonicalOK := fixture.values.ReturnBoundariesForBody(module, body)
	entry, planned := returnroute.BodyBoundaries(fixture.values, selectedBody)
	if !moduleOK || !bodyOK || !canonicalOK || !planned || len(canonical) != 1 || len(entry) != len(canonical) {
		t.Fatalf("body-result return plan module=%t body=%t canonical=%d/%t planned=%t rows=%d", moduleOK, bodyOK, len(canonical), canonicalOK, planned, len(entry))
	}
	for index, boundary := range canonical {
		if entry[index] != boundary || !fixture.values.OwnsReturnBoundary(boundary) || boundary.MemberCount() < 2 {
			t.Fatalf("return boundary %d was not preserved as its canonical Value row", index)
		}
	}
}

// TestBodyResultRuleSelectsEveryReturnButOnlyResultZero proves both halves of
// the body-result contract: all authenticated return paths contribute their
// fixed first member to the one join, while a later CallResultSlot cannot
// become an output of this result-zero rule.
func TestBodyResultRuleSelectsEveryReturnButOnlyResultZero(t *testing.T) {
	fixture := newBodyResultLawFixture(t, "body_result_selection")

	selection, selected := returnroute.Select(fixture.values, fixture.calls, fixture.result0, fixture.fact)
	if !selected || !selection.HasBody() || selection.NilCase() || selection.Top() || len(selection.Tags()) != 2 {
		t.Fatalf("result-zero selection = %#v/%t, want two concrete body returns", selection, selected)
	}
	want := make([]uint64, 0, len(fixture.bodies))
	for _, selectedBody := range fixture.bodies {
		module, moduleOK := selectedBody.ModuleKey()
		body, bodyOK := selectedBody.BodyPath()
		boundaries, boundariesOK := fixture.values.ReturnBoundariesForBody(module, body)
		if !moduleOK || !bodyOK || !boundariesOK || len(boundaries) != 1 {
			t.Fatal("canonical selected body return boundary")
		}
		member, memberOK := boundaries[0].MemberAt(0)
		coordinate, coordinateOK := member.Coordinate()
		index, indexOK := fixture.values.CoordinateIndex(coordinate)
		if !memberOK || !coordinateOK || !indexOK {
			t.Fatal("canonical first return member")
		}
		want = append(want, uint64(index)+1)
	}
	sort.Slice(want, func(left, right int) bool { return want[left] < want[right] })
	if len(fixture.bodies) != 2 || !sameTags(selection.Tags(), want) {
		t.Fatalf("selected return tags=%v, want canonical body tags=%v", selection.Tags(), want)
	}

	resolved, resolvedOK := fixture.values.MountedCallResultSlotForMountedOccurrence(fixture.module, fixture.call)
	ordinal, ordinalOK := resolved.Ordinal()
	if !resolvedOK || !ordinalOK || ordinal != 0 || resolved != fixture.result0 {
		t.Fatalf("body-result candidate = ordinal %d/%t resolved=%t; want exact result slot zero", ordinal, ordinalOK, resolvedOK)
	}
	if _, contentOK := fixture.values.MountedCallResultSlotOrdinal(fixture.result0); !contentOK {
		t.Fatal("result-zero slot did not issue a body-result candidate address")
	}
	if _, selected := returnroute.Select(fixture.values, fixture.calls, fixture.result1, fixture.fact); selected {
		t.Fatal("result-one slot was admitted by the result-zero route")
	}
	if _, contentOK := fixture.values.MountedCallResultSlotOrdinal(fixture.result1); contentOK {
		t.Fatal("result-one slot issued a body-result candidate address")
	}
}

// TestBodyResultHotBindRefusesForeignCallAuthority proves that no equality of
// source text, Program IDs, or result geometry can bridge the Value/Call
// owner boundary. The foreign Call Algebra is intentionally sealed from the
// same immutable source and compiled snapshot but a distinct Link owner.
func TestBodyResultHotBindRefusesForeignCallAuthority(t *testing.T) {
	fixture := newBodyResultLawFixture(t, "body_result_foreign")
	foreignCalls := foreignCallsForBodyResultFixture(t, fixture)
	if judgment, accepted := Derive(fixture.values, foreignCalls); accepted || judgment.Valid() {
		t.Fatal("body-result judgment accepted a Call authority from a distinct Link owner")
	}
	if judgment, accepted := Derive(fixture.values, fixture.calls); !accepted || !judgment.Valid() {
		t.Fatal("body-result judgment refused the Call authority of its own Link")
	}
}

func newBodyResultLawFixture(t testing.TB, name string) *bodyResultLawFixture {
	t.Helper()
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(target, name+".lua", []byte(bodyResultSubject))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	issuance := testfixture.EmptyProgramIssuancePlan(t)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("source mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if !grammarOK || failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	structural := bodyResultStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("ingress snapshot")
	}
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	mountedProgram := programmount.Program{ModuleKey: module, Program: artifact.Program()}
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{{Program: mountedProgram, Snapshot: snapshot}})
	if !callsOK || calls == nil {
		t.Fatal("Call algebra")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calls, []programmount.MountedArtifact{valueMount}, structural)
	if !heapMountOK || !valueMountOK || heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal Value fixture heapMount=%t valueMount=%t heap=%s value=%s", heapMountOK, valueMountOK, heapFailure, valueFailure)
	}

	count, countOK := artifact.Program().CallCount()
	if !countOK {
		t.Fatal("Call family")
	}
	var resultZeroRows, resultOneRows, mountedRows, bodyTargets, selectedBodies, completeFacts int
	for index := 0; index < count; index++ {
		row, rowOK := artifact.Program().CallAt(index)
		if !rowOK {
			t.Fatalf("Call row %d", index)
		}
		result0, result0OK := values.MountedCallResultSlotFor(module, row.ID(), 0)
		result1, result1OK := values.MountedCallResultSlotFor(module, row.ID(), 1)
		mounted, mountedOK := calls.MountedCallForOccurrence(module, row.ID())
		key, keyOK := calls.KeyForMountedCall(mounted)
		if result0OK {
			resultZeroRows++
		}
		if result1OK {
			resultOneRows++
		}
		if mountedOK {
			mountedRows++
		}
		if !result0OK || !result1OK || !mountedOK || !keyOK {
			continue
		}
		candidates := make([]calldomain.Target, 0, 2)
		bodies := make([]calldomain.Body, 0, 2)
		for support := 0; support < calls.SupportCount(key); support++ {
			candidate, candidateOK := calls.SupportTargetAt(key, support)
			body, bodyOK := candidate.Body()
			bodyModule, bodyModuleOK := body.ModuleKey()
			bodyPath, bodyPathOK := body.BodyPath()
			boundaries, boundariesOK := values.ReturnBoundariesForBody(bodyModule, bodyPath)
			if candidateOK && bodyOK {
				bodyTargets++
			}
			if !candidateOK || !bodyOK || !bodyModuleOK || !bodyPathOK || !boundariesOK || len(boundaries) != 1 {
				continue
			}
			if !values.OwnsReturnBoundary(boundaries[0]) || boundaries[0].MemberCount() < 2 {
				continue
			}
			candidates = append(candidates, candidate)
			bodies = append(bodies, body)
		}
		selectedBodies += len(bodies)
		if len(candidates) != 2 {
			continue
		}
		fact, factOK := calls.DispatchValue(key, candidates, false)
		if !factOK || fact.KnownTargetCount() != len(candidates) {
			continue
		}
		completeFacts++
		return &bodyResultLawFixture{linked: linked, contract: target, snapshot: snapshot, values: values, calls: calls, module: module, call: row.ID(), bodies: bodies, fact: fact, result0: result0, result1: result1}
	}
	t.Fatalf("two selected body returns with fixed result slots: calls=%d result0=%d result1=%d mounted=%d bodyTargets=%d selectedBodies=%d facts=%d", count, resultZeroRows, resultOneRows, mountedRows, bodyTargets, selectedBodies, completeFacts)
	return nil
}

func foreignCallsForBodyResultFixture(t testing.TB, fixture *bodyResultLawFixture) *calldomain.Algebra {
	t.Helper()
	mounts := fixture.linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	if !shardOK || !programOK || program == nil {
		t.Fatal("foreign source program")
	}
	// Re-sealing the same source gives an equal-content Link with a distinct
	// owner capability. The shared artifact is safe because Program identity is
	// source-derived; only ownership differs.
	foreignLink, err := link.Seal(&link.Spec{Target: fixture.contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatalf("foreign link: %v", err)
	}
	foreignMounts := foreignLink.Project().Mounts()
	foreignShard, foreignShardOK := foreignMounts.At(0)
	foreignModule, foreignModuleOK := foreignLink.Project().ModuleKey(foreignShard)
	if !foreignShardOK || !foreignModuleOK {
		t.Fatal("foreign mount")
	}
	mounted := programmount.Program{ModuleKey: foreignModule, Program: fixture.snapshot.Program()}
	calls, callsOK := calldomain.NewWithMountedArtifacts(foreignLink, []calldomain.MountedArtifact{{Program: mounted, Snapshot: fixture.snapshot}})
	if !callsOK || calls == nil || fixture.values.LinkOwner().Matches(calls.LinkOwner()) {
		t.Fatal("foreign Call owner")
	}
	return calls
}

func sameTags(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// bodyResultStructuralVocabulary is a direct ingress fixture vocabulary. It
// deliberately does not import the composite catalog because that catalog is
// the production registrar of this package's rule. The Value seal needs only
// the fixed neutral category widths; it does not receive a shadow rule table.
func bodyResultStructuralVocabulary(t testing.TB) structure.Table {
	t.Helper()
	counts := func(category structure.Category) int {
		switch category {
		case structure.CategoryArm:
			return 8
		case structure.CategoryEvent:
			return 3
		case structure.CategoryOutcome:
			return 7
		case structure.CategoryRuntimeKind:
			return int(runtimekind.Count) - 1
		case structure.CategoryOccurrenceKind:
			return 32
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("body-result/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("body-result structural declarations")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("body-result structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(bodyResultEmptySurface{kind: kind}) {
			t.Fatalf("body-result structural surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("body-result structural schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("body-result structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("body-result structural table")
	}
	return table
}

type bodyResultEmptySurface struct{ kind schema.SurfaceKind }

func (surface bodyResultEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface bodyResultEmptySurface) Entries() []schema.Entry  { return nil }
func (surface bodyResultEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
