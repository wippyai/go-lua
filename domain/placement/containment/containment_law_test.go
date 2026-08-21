package containment

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

type containmentFixture struct {
	heap      heapdomain.Schema
	placement placementdomain.Schema
	roots     []heapdomain.Key
	slot      heapdomain.Slot
	payload   heapdomain.Payload
	selector  heapdomain.KeySelector
}

func TestContainmentVisitorExactNoneUnknownAndMetatableEdges(t *testing.T) {
	fixture := newContainmentFixture(t)
	parent, child := fixture.roots[0], fixture.roots[1]
	none := mustNone(t, fixture.heap)
	childReference := mustReference(t, fixture.heap, child, materialization.Recent)
	exact := mustExact(t, fixture.heap, childReference)
	unknown := mustUnknown(t, fixture.heap)

	exactValue := mustValue(t, fixture, parent, none, exact, none)
	exactObservations := visit(t, fixture.heap, exactValue)
	if !hasEdge(exactObservations, heapdomain.ContainmentExact, heapdomain.ContainmentSiteValue, child) {
		t.Fatal("exact Present value containment was not visited")
	}

	noneValue := mustValue(t, fixture, parent, none, none, none)
	noneObservations := visit(t, fixture.heap, noneValue)
	if countKind(noneObservations, heapdomain.ContainmentNone) != 2 {
		t.Fatalf("None Present pair count = %d, want 2", countKind(noneObservations, heapdomain.ContainmentNone))
	}

	unknownValue := mustValue(t, fixture, parent, none, unknown, none)
	unknownObservations := visit(t, fixture.heap, unknownValue)
	if countKind(unknownObservations, heapdomain.ContainmentUnknown) != 1 {
		t.Fatalf("Unknown Present edge count = %d, want 1", countKind(unknownObservations, heapdomain.ContainmentUnknown))
	}

	metaObject := mustObject(t, fixture.heap, exact)
	metaValue := mustRelation(t, fixture.heap, parent, metaObject)
	metaObservations := visit(t, fixture.heap, metaValue)
	if !hasEdge(metaObservations, heapdomain.ContainmentExact, heapdomain.ContainmentSiteMetatable, child) {
		t.Fatal("exact metatable containment was not visited")
	}
}

func TestContainmentVisitorSelfAndMutualCyclesAreFinite(t *testing.T) {
	fixture := newContainmentFixture(t)
	first, second := fixture.roots[0], fixture.roots[1]
	none := mustNone(t, fixture.heap)
	firstReference := mustReference(t, fixture.heap, first, materialization.Recent)
	secondReference := mustReference(t, fixture.heap, second, materialization.Recent)
	firstCycle := mustExact(t, fixture.heap, firstReference)
	secondCycle := mustExact(t, fixture.heap, secondReference)

	self := mustValue(t, fixture, first, none, firstCycle, none)
	selfObservations := visit(t, fixture.heap, self)
	if len(selfObservations) != 2 || countKind(selfObservations, heapdomain.ContainmentExact) != 1 {
		t.Fatalf("self-cycle observations = %d/%d, want one exact pair", len(selfObservations), countKind(selfObservations, heapdomain.ContainmentExact))
	}

	left := mustValue(t, fixture, first, none, secondCycle, none)
	right := mustValue(t, fixture, second, none, firstCycle, none)
	if len(visit(t, fixture.heap, left)) != 2 || len(visit(t, fixture.heap, right)) != 2 {
		t.Fatal("mutual cycle traversal did not terminate at one complete object")
	}
}

func TestContainmentVisitorRejectsForeignOwnerAndDirectKeyFence(t *testing.T) {
	fixture := newContainmentFixture(t)
	foreign := newContainmentFixtureNamed(t, "placement-containment-foreign")
	foreignValue := foreign.heap.Bottom()
	if fixture.heap.VisitContainments(foreignValue, func(heapdomain.ContainmentVisit) bool { return true }) {
		t.Fatal("foreign Heap Value crossed the visitor owner fence")
	}
	foreignID, foreignIDOK := foreign.roots[0].ContentID()
	if !foreignIDOK {
		t.Fatal("foreign root identity")
	}
	if _, ok := fixture.heap.KeyForID(foreignID); ok {
		t.Fatal("foreign root identity received a parent operand")
	}
}

func TestContainmentRuleEnumeratesEachAllocationRootOnce(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	want := 0
	for _, key := range fixture.roots {
		if key.Kind() == heapdomain.RootAllocation {
			want++
		}
	}
	if rule.Count() != want {
		t.Fatalf("Link count = %d, want %d", rule.Count(), want)
	}
	seen := make(map[identity.ContentID]struct{}, rule.Count())
	for index := 0; index < rule.Count(); index++ {
		id, idOK := rule.IDAt(index)
		key, keyOK := fixture.heap.KeyForID(id)
		if !idOK || !keyOK || key.Kind() != heapdomain.RootAllocation {
			t.Fatalf("Link row %d is not an allocation root", index)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatal("Link repeated one allocation-root identity")
		}
		seen[id] = struct{}{}
	}
}

func TestContainmentRuleCatalogueKeepsCanonicalMountOrder(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	if rule.catalogue == nil || rule.catalogue.count != rule.Count() || len(rule.catalogue.mounts) > fixture.heap.ArtifactMountCount() {
		t.Fatalf("sealed containment catalogue = %#v, want count=%d with at most %d mount prefixes", rule.catalogue, rule.Count(), fixture.heap.ArtifactMountCount())
	}
	previousEnd := 0
	for index, prefix := range rule.catalogue.mounts {
		if prefix.start != previousEnd || prefix.end <= prefix.start || !prefix.issuer.Module().Available() || !prefix.issuer.ProgramID().Available() {
			t.Fatalf("mount prefix %d = [%d,%d) module=%v program=%v after %d", index, prefix.start, prefix.end, prefix.issuer.Module(), prefix.issuer.ProgramID(), previousEnd)
		}
		previousEnd = prefix.end
	}
	if previousEnd != rule.catalogue.count {
		t.Fatalf("mount prefix coverage ends at %d, want %d", previousEnd, rule.catalogue.count)
	}

	heapSchema := fixture.heap
	want := make([]identity.ContentID, 0, rule.Count())
	for mountIndex := 0; mountIndex < heapSchema.ArtifactMountCount(); mountIndex++ {
		mount, mountOK := heapSchema.ArtifactMountAt(mountIndex)
		if !mountOK {
			t.Fatalf("mount %d", mountIndex)
		}
		issuer, issuerOK := heapSchema.OccurrenceMountForModule(mount.Module())
		if !issuerOK {
			t.Fatalf("issuer %d", mountIndex)
		}
		for allocationIndex := 0; allocationIndex < issuer.AllocationCount(); allocationIndex++ {
			_, key, keyOK := issuer.AllocationAt(allocationIndex)
			if !keyOK {
				t.Fatalf("allocation %d/%d", mountIndex, allocationIndex)
			}
			id, idOK := key.ContentID()
			if !idOK {
				t.Fatalf("allocation identity %d/%d", mountIndex, allocationIndex)
			}
			want = append(want, id)
		}
	}
	if len(want) != rule.Count() {
		t.Fatalf("canonical catalogue width = %d, want %d", rule.Count(), len(want))
	}
	for index, wantID := range want {
		gotID, gotOK := rule.IDAt(index)
		if !gotOK || gotID != wantID {
			t.Fatalf("catalogue row %d = %v/%t, want %v/true", index, gotID, gotOK, wantID)
		}
	}
	if _, ok := rule.IDAt(-1); ok {
		t.Fatal("negative catalogue index accepted")
	}
	if _, ok := rule.IDAt(rule.Count()); ok {
		t.Fatal("catalogue end index accepted")
	}
}

func TestContainmentCatalogueRejectsInvalidSchema(t *testing.T) {
	catalogue, catalogueOK := buildCatalogue(placementdomain.Schema{})
	if catalogueOK || catalogue != nil {
		t.Fatal("invalid Placement schema produced a containment catalogue")
	}
}

func TestContainmentCataloguePrefixLookupUsesHalfOpenMountRanges(t *testing.T) {
	catalogue := &containmentCatalogue{
		mounts: []catalogueMount{{start: 0, end: 2}, {start: 2, end: 5}, {start: 5, end: 6}},
		count:  6,
	}
	for index, wantLocal := range []int{0, 1, 0, 1, 2, 0} {
		mount, local, ok := catalogue.mountAt(index)
		if !ok || local != wantLocal || index < mount.start || index >= mount.end {
			t.Fatalf("prefix lookup %d = [%d,%d) local=%d/%t, want local=%d", index, mount.start, mount.end, local, ok, wantLocal)
		}
	}
	for _, index := range []int{-1, catalogue.count} {
		if mount, local, ok := catalogue.mountAt(index); ok {
			t.Fatalf("out-of-range prefix lookup %d = %#v local=%d", index, mount, local)
		}
	}
	broken := &containmentCatalogue{mounts: []catalogueMount{{start: 1, end: 2}}, count: 2}
	if mount, local, ok := broken.mountAt(0); ok {
		t.Fatalf("prefix gap acquired a mount: %#v local=%d", mount, local)
	}
}

func TestContainmentEmptyLinkDenominatorRemainsValid(t *testing.T) {
	heapSchema, placementSchema := newContainmentSchemas(t, "placement-containment-empty", "return 1")
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			t.Fatalf("scalar-only fixture unexpectedly contains allocation root at %d", index)
		}
	}
	rule := bindContainmentLaw(t, containmentFixture{heap: heapSchema, placement: placementSchema})
	if rule == nil || rule.Count() != 0 {
		t.Fatal("empty containment rule did not retain an empty Link denominator")
	}
}

func TestContainmentRuleRoutePlanExactAndUnknownBroadcast(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	none := mustNone(t, fixture.heap)
	child := mustReference(t, fixture.heap, fixture.roots[1], materialization.Recent)
	exact := mustExact(t, fixture.heap, child)
	exactValue := mustValue(t, fixture, fixture.roots[0], none, exact, none)
	if opaque, complete := rule.containmentEvidence(exactValue); opaque || !complete {
		t.Fatalf("exact containment evidence = opaque:%t complete:%t, want false/true", opaque, complete)
	}
	if !rule.walkContainments(exactValue, nil) {
		t.Fatal("exact route walk was not complete")
	}
	unknown := mustUnknown(t, fixture.heap)
	unknownValue := mustValue(t, fixture, fixture.roots[0], none, unknown, none)
	if opaque, complete := rule.containmentEvidence(unknownValue); !opaque || !complete {
		t.Fatalf("opaque containment evidence = opaque:%t complete:%t, want true/true", opaque, complete)
	}
	if rule.walkContainments(unknownValue, nil) {
		t.Fatal("unknown identity route walk was treated as complete")
	}
	unknownMetaObject := mustObject(t, fixture.heap, unknown)
	unknownMetaValue := mustRelation(t, fixture.heap, fixture.roots[0], unknownMetaObject)
	if rule.walkContainments(unknownMetaValue, nil) {
		t.Fatal("unknown metatable route walk was treated as complete")
	}
	top := fixture.heap.Top()
	if opaque, complete := rule.containmentEvidence(top); opaque || complete {
		t.Fatalf("Top containment walk = opaque:%t complete:%t, want false/false before authenticated Top widening", opaque, complete)
	}
	if rule.walkContainments(top, nil) {
		t.Fatal("Top route walk was treated as complete")
	}
	// Unknown containment is identity uncertainty only. Both unknown values
	// remain valid, non-Top Heap values, so the active Fold's class-widening
	// branch (`!Valid || IsTop`) does not force Placement.Unknown; locate's
	// authenticated opaque edge selects the conservative broadcast route set.
	if !unknownValue.Valid() || unknownValue.IsTop() || !unknownMetaValue.Valid() || unknownMetaValue.IsTop() {
		t.Fatal("unknown containment witness changed class uncertainty")
	}

	if rule == nil || rule.Count() == 0 {
		t.Fatal("containment route denominator")
	}
	seen := make(map[heapdomain.Key]struct{}, rule.Count())
	for index := 0; index < fixture.heap.KeyCount(); index++ {
		key, keyOK := fixture.heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		tag, tagOK := broadcastRouteTag(index, index)
		if !tagOK {
			t.Fatalf("broadcast route tag for dense index %d", index)
		}
		resolved, resolvedOK := routeKey(fixture.placement, tag)
		if !resolvedOK || resolved != key {
			t.Fatalf("broadcast route %d resolved to %#v/%t, want %#v", index, resolved, resolvedOK, key)
		}
		if _, duplicate := seen[resolved]; duplicate {
			t.Fatal("broadcast route duplicated an allocation root")
		}
		seen[resolved] = struct{}{}
	}
	if len(seen) != rule.Count() {
		t.Fatalf("unknown identity route plan = %d roots, Link = %d", len(seen), rule.Count())
	}
}

func TestContainmentMissingOrForeignEvidenceRefusesWithoutWidening(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	if opaque, complete := rule.containmentEvidence(heapdomain.Value{}); opaque || complete {
		t.Fatalf("missing containment evidence = opaque:%t complete:%t, want false/false", opaque, complete)
	}
	foreign := newContainmentFixtureNamed(t, "placement-containment-foreign-evidence")
	if opaque, complete := rule.containmentEvidence(foreign.heap.Top()); opaque || complete {
		t.Fatalf("foreign containment evidence = opaque:%t complete:%t, want false/false", opaque, complete)
	}
}

func TestContainmentExactRouteTagsRetainDuplicateEdgeOrdinals(t *testing.T) {
	fixture := newContainmentFixture(t)
	parent, child := fixture.roots[0], fixture.roots[1]
	none := mustNone(t, fixture.heap)
	reference := mustReference(t, fixture.heap, child, materialization.Recent)
	exact := mustExact(t, fixture.heap, reference)
	value := mustValue(t, fixture, parent, none, exact, exact)
	childIndex, childIndexOK := fixture.heap.KeyIndex(child)
	if !childIndexOK {
		t.Fatal("child dense index")
	}

	ordinal := uint64(0)
	matched := 0
	if !fixture.heap.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		ordinal++
		if observation.Kind() != heapdomain.ContainmentExact {
			return true
		}
		reference, referenceOK := observation.Reference()
		key, _, keyOK := reference.Key()
		if !referenceOK || !keyOK || key != child {
			return true
		}
		tag, tagOK := exactRouteTag(childIndex, ordinal)
		if !tagOK || tag&1 != 0 {
			t.Fatalf("duplicate exact edge ordinal %d produced invalid tag", ordinal)
		}
		matched++
		return true
	}) {
		t.Fatal("complete duplicate edge walk")
	}
	if matched != 2 {
		t.Fatalf("duplicate exact route edges = %d, want 2", matched)
	}
}

func TestContainmentWideRouteWalkIsAllocationFree(t *testing.T) {
	fixture, parent, child := newWideContainmentFixture(t)
	value := mustWideValue(t, fixture, parent, child, 5)
	exactEdges := countKind(visit(t, fixture.heap, value), heapdomain.ContainmentExact)
	if exactEdges < 10 {
		t.Fatalf("wide containment exact edges = %d, want at least 10", exactEdges)
	}
	rule := bindContainmentLaw(t, fixture)
	seen := 0
	firstOK := rule.walkContainments(value, nil)
	secondOK := rule.walkContainments(value, func(_ heapdomain.Key, _ uint64) bool {
		seen++
		return true
	})
	if !firstOK || !secondOK || seen != exactEdges {
		t.Fatalf("wide route walk complete=%t/%t routes=%d, want %d", firstOK, secondOK, seen, exactEdges)
	}
	complete := true
	allocs := testing.AllocsPerRun(100, func() {
		count := 0
		if !rule.walkContainments(value, nil) || !rule.walkContainments(value, func(_ heapdomain.Key, _ uint64) bool {
			count++
			return true
		}) || count != exactEdges {
			complete = false
		}
	})
	if !complete {
		t.Fatal("wide route walk lost deterministic edges")
	}
	if allocs != 0 {
		t.Fatalf("wide route walk allocations = %f, want 0", allocs)
	}
}

func bindContainmentLaw(t testing.TB, fixture containmentFixture) *HotRule {
	t.Helper()
	builder := engine.NewSchema()
	placementFragment, placementOK := placementowner.DeclareSchema(builder, containmentLawSemantic(1), containmentLawSemantic(2))
	heapFragment, heapOK := heapowner.DeclareSchema(builder, containmentLawSemantic(3), containmentLawSemantic(7))
	fragment, fragmentOK := DeclareSchema(builder, containmentLawSemantic(4), containmentLawSemantic(5), placementFragment, heapFragment)
	cold, coldOK := builder.Seal()
	binding := engine.NewSchemaBinding(cold)
	placementHot, placementHotOK := placementowner.BindHot(binding, placementFragment, fixture.placement)
	heapHot, heapHotOK := heapowner.BindHot(binding, heapFragment, fixture.heap)
	rule, ruleOK := BindHot(binding, fragment, placementHot, heapHot, fixture.placement)
	sealed := binding != nil && binding.Seal()
	if !placementOK || !heapOK || !fragmentOK || !coldOK || cold == nil || !placementHotOK || !heapHotOK || !ruleOK || !sealed {
		t.Fatalf("containment hot bind placement=%t heap=%t fragment=%t cold=%t placementHot=%t heapHot=%t rule=%t sealed=%t", placementOK, heapOK, fragmentOK, coldOK, placementHotOK, heapHotOK, ruleOK, sealed)
	}
	return rule
}

func containmentLawSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xC7, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("containment law semantic key")
	}
	return key
}

func BenchmarkContainmentLinkCount(b *testing.B) {
	fixture := newContainmentFixture(b)
	rule := bindContainmentLaw(b, fixture)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if rule.Count() < 0 {
			b.Fatal("negative Link denominator")
		}
	}
}

func BenchmarkContainmentLinkIDAt(b *testing.B) {
	fixture := newContainmentFixture(b)
	rule := bindContainmentLaw(b, fixture)
	count := rule.Count()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for index := 0; index < count; index++ {
			if _, ok := rule.IDAt(index); !ok {
				b.Fatal("canonical allocation identity")
			}
		}
	}
}

func BenchmarkContainmentCataloguePrefixLookup4096Mounts(b *testing.B) {
	const mountCount = 4096
	mounts := make([]catalogueMount, mountCount)
	for index := range mounts {
		mounts[index].start = index * 3
		mounts[index].end = mounts[index].start + 3
	}
	catalogue := &containmentCatalogue{mounts: mounts, count: mountCount * 3}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		index := (iteration * 7919) % catalogue.count
		mount, local, ok := catalogue.mountAt(index)
		if !ok || local != index-mount.start {
			b.Fatal("canonical mount prefix lookup")
		}
	}
}

func BenchmarkContainmentWideRoutePlan(b *testing.B) {
	fixture, parent, child := newWideContainmentFixture(b)
	value := mustWideValue(b, fixture, parent, child, 5)
	exactEdges := countKind(visit(b, fixture.heap, value), heapdomain.ContainmentExact)
	rule := bindContainmentLaw(b, fixture)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		seen := 0
		if !rule.walkContainments(value, nil) || !rule.walkContainments(value, func(_ heapdomain.Key, _ uint64) bool {
			seen++
			return true
		}) || seen != exactEdges {
			b.Fatal("wide route plan")
		}
	}
}

func BenchmarkContainmentVisitorEdgeWalk(b *testing.B) {
	fixture := newContainmentFixture(b)
	none := mustNone(b, fixture.heap)
	child := mustReference(b, fixture.heap, fixture.roots[1], materialization.Recent)
	exact := mustExact(b, fixture.heap, child)
	value := mustValue(b, fixture, fixture.roots[0], none, exact, none)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		seen := 0
		if !fixture.heap.VisitContainments(value, func(heapdomain.ContainmentVisit) bool {
			seen++
			return true
		}) || seen != 2 {
			b.Fatal("complete containment edge walk")
		}
	}
}

func newContainmentFixture(t testing.TB) containmentFixture {
	return newContainmentFixtureNamed(t, "placement-containment")
}

func newContainmentFixtureNamed(t testing.TB, name string) containmentFixture {
	t.Helper()
	heapSchema, placementSchema := newContainmentSchemas(t, name, `
local first = { value = 1 }
local second = { child = first }
local third = { child = second }
return third
	`)
	roots := make([]heapdomain.Key, 0)
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			roots = append(roots, key)
		}
	}
	if len(roots) < 3 {
		t.Fatalf("containment fixture roots = %d, want at least 3", len(roots))
	}
	for _, key := range roots {
		for index := 0; index < heapSchema.FieldCount(key); index++ {
			field, fieldOK := heapSchema.FieldAt(key, index)
			slot, slotOK := heapSchema.SlotForField(field)
			payload, payloadOK := heapSchema.PayloadForField(field)
			selector, selectorOK := heapSchema.SelectorForSlot(slot)
			if fieldOK && slotOK && payloadOK && selectorOK && selector.Kind() == heapdomain.KeySelectorAtom {
				return containmentFixture{heap: heapSchema, placement: placementSchema, roots: roots, slot: slot, payload: payload, selector: selector}
			}
		}
	}
	t.Fatal("containment fixture did not expose an exact field")
	return containmentFixture{}
}

func newWideContainmentFixture(t testing.TB) (containmentFixture, heapdomain.Key, heapdomain.Key) {
	heapSchema, placementSchema := newContainmentSchemas(t, "placement-containment-wide", `
local first = {}
local second = { a = first, b = first, c = first, d = first, e = first }
return second
	`)
	var roots []heapdomain.Key
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			roots = append(roots, key)
		}
	}
	var parent, child heapdomain.Key
	for _, key := range roots {
		if heapSchema.FieldCount(key) >= 5 {
			parent = key
			break
		}
	}
	for _, key := range roots {
		if key != parent {
			child = key
			break
		}
	}
	if !parent.Valid() || !child.Valid() {
		t.Fatalf("wide fixture roots parent=%t child=%t", parent.Valid(), child.Valid())
	}
	return containmentFixture{heap: heapSchema, placement: placementSchema}, parent, child
}

func mustWideValue(t testing.TB, fixture containmentFixture, parent, child heapdomain.Key, minimumFields int) heapdomain.Value {
	t.Helper()
	none := mustNone(t, fixture.heap)
	reference := mustReference(t, fixture.heap, child, materialization.Recent)
	exact := mustExact(t, fixture.heap, reference)
	initializer, initializerOK := fixture.heap.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !initializerOK {
		t.Fatal("wide Heap object initializer")
	}
	applied := 0
	for index := 0; index < fixture.heap.FieldCount(parent); index++ {
		field, fieldOK := fixture.heap.FieldAt(parent, index)
		slot, slotOK := fixture.heap.SlotForField(field)
		payload, payloadOK := fixture.heap.PayloadForField(field)
		selector, selectorOK := fixture.heap.SelectorForSlot(slot)
		if !fieldOK || !slotOK || !payloadOK || !selectorOK || selector.Kind() != heapdomain.KeySelectorAtom {
			continue
		}
		cell, cellOK := fixture.heap.CellPresent(slot, payload, exact, exact)
		if !cellOK || !initializer.Apply(selector, cell) {
			t.Fatal("wide Heap exact field")
		}
		applied++
	}
	if applied < minimumFields {
		t.Fatalf("wide Heap exact fields = %d, want at least %d", applied, minimumFields)
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		t.Fatal("wide Heap object finish")
	}
	return mustRelation(t, fixture.heap, parent, object)
}

func newContainmentSchemas(t testing.TB, name, source string) (heapdomain.Schema, placementdomain.Schema) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	issuanceDirectory := issuance.Directory{}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuanceDirectory)
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural := syntheticStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := heapdomain.NewArtifactMount(snapshot, module, programID)
	heapSchema, sealFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{mount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	if !grammarOK || failure.Available() || !lowered || !shardOK || !moduleOK || !programIDOK || !mountOK || sealFailure != heapdomain.SealFailureNone || !placementOK {
		t.Fatalf("containment fixture grammar=%t artifact=%v ingress=%t shard=%t module=%t program=%t mount=%t seal=%v placement=%t", grammarOK, failure, lowered, shardOK, moduleOK, programIDOK, mountOK, sealFailure, placementOK)
	}
	return heapSchema, placementSchema
}

// syntheticStructuralVocabulary is deliberately local to this law package.
// It keeps the fixture on the same direct artifact -> ingress path as the
// return-escape laws, so containment tests do not depend on the composite
// registration graph that owns the rule itself.
func syntheticStructuralVocabulary(t testing.TB) structure.Table {
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
		case structure.CategoryIssuanceForm:
			return 5
		case structure.CategoryIssuanceInput:
			return 4
		case structure.CategoryIssuanceStage:
			return 5
		case structure.CategoryIssuanceRequirement:
			return 2
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("placement-containment/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("synthetic structural declarations")
	}
	builder := schema.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("synthetic structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(emptySurface{kind: kind}) {
			t.Fatalf("synthetic surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("synthetic schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("synthetic structure view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("synthetic structure table")
	}
	return table
}

type emptySurface struct{ kind schema.SurfaceKind }

func (surface emptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface emptySurface) Entries() []schema.Entry  { return nil }
func (surface emptySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func mustNone(t testing.TB, schema heapdomain.Schema) heapdomain.Containment {
	t.Helper()
	value, ok := schema.ContainmentNone()
	if !ok {
		t.Fatal("Heap None containment")
	}
	return value
}

func mustUnknown(t testing.TB, schema heapdomain.Schema) heapdomain.Containment {
	t.Helper()
	value, ok := schema.ContainmentUnknown()
	if !ok {
		t.Fatal("Heap Unknown containment")
	}
	return value
}

func mustReference(t testing.TB, schema heapdomain.Schema, key heapdomain.Key, role materialization.Role) heapdomain.Reference {
	t.Helper()
	value, ok := schema.Reference(key, role)
	if !ok {
		t.Fatal("Heap reference")
	}
	return value
}

func mustExact(t testing.TB, schema heapdomain.Schema, reference heapdomain.Reference) heapdomain.Containment {
	t.Helper()
	value, ok := schema.ContainmentExact(reference)
	if !ok {
		t.Fatal("Heap exact containment")
	}
	return value
}

func mustObject(t testing.TB, schema heapdomain.Schema, metatable heapdomain.Containment) heapdomain.Object {
	t.Helper()
	value, ok := schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, metatable)
	if !ok {
		t.Fatal("Heap object")
	}
	return value
}

func mustValue(t testing.TB, fixture containmentFixture, key heapdomain.Key, metatable, valueChild, keyChild heapdomain.Containment) heapdomain.Value {
	t.Helper()
	slot, payload, selector := fieldForRoot(t, fixture.heap, key)
	object := mustObject(t, fixture.heap, metatable)
	cell, ok := fixture.heap.CellPresent(slot, payload, valueChild, keyChild)
	if !ok {
		t.Fatal("Heap present cell")
	}
	initializer, ok := fixture.heap.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, metatable)
	if !ok || !initializer.Apply(selector, cell) {
		t.Fatal("Heap object initializer")
	}
	object, ok = initializer.Finish()
	if !ok {
		t.Fatal("Heap object finish")
	}
	return mustRelation(t, fixture.heap, key, object)
}

func fieldForRoot(t testing.TB, schema heapdomain.Schema, key heapdomain.Key) (heapdomain.Slot, heapdomain.Payload, heapdomain.KeySelector) {
	t.Helper()
	for index := 0; index < schema.FieldCount(key); index++ {
		field, fieldOK := schema.FieldAt(key, index)
		slot, slotOK := schema.SlotForField(field)
		payload, payloadOK := schema.PayloadForField(field)
		selector, selectorOK := schema.SelectorForSlot(slot)
		if fieldOK && slotOK && payloadOK && selectorOK && selector.Kind() == heapdomain.KeySelectorAtom {
			return slot, payload, selector
		}
	}
	t.Fatal("Heap root field")
	return heapdomain.Slot{}, heapdomain.Payload{}, heapdomain.KeySelector{}
}

func mustRelation(t testing.TB, schema heapdomain.Schema, key heapdomain.Key, object heapdomain.Object) heapdomain.Value {
	t.Helper()
	world, worldOK := schema.One(key, object)
	value, valueOK := schema.Relation(key, world)
	if !worldOK || !valueOK {
		t.Fatal("Heap relation")
	}
	return value
}

func visit(t testing.TB, schema heapdomain.Schema, value heapdomain.Value) []heapdomain.ContainmentVisit {
	t.Helper()
	observations := make([]heapdomain.ContainmentVisit, 0)
	if !schema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		observations = append(observations, observation)
		return true
	}) {
		t.Fatal("complete Heap containment walk failed")
	}
	return observations
}

func countKind(observations []heapdomain.ContainmentVisit, kind heapdomain.ContainmentKind) int {
	count := 0
	for _, observation := range observations {
		if observation.Kind() == kind {
			count++
		}
	}
	return count
}

func hasEdge(observations []heapdomain.ContainmentVisit, kind heapdomain.ContainmentKind, site heapdomain.ContainmentSite, key heapdomain.Key) bool {
	for _, observation := range observations {
		if observation.Kind() != kind || observation.Site != site {
			continue
		}
		reference, referenceOK := observation.Reference()
		child, _, childOK := reference.Key()
		if referenceOK && childOK && child == key {
			return true
		}
	}
	return false
}
