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
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture"
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
		t.Fatal("foreign root identity received a parent Operand")
	}
}

func TestContainmentRulePublishesOneCompleteSummaryOperand(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	if rule.Count() != 1 {
		t.Fatalf("mounted-point occurrence count = %d, want 1", rule.Count())
	}
	id, idOK := rule.IDAt(0)
	if !idOK || id != fixture.placement.ContentID() {
		t.Fatalf("closure Operand id=%t/%t", idOK, id == fixture.placement.ContentID())
	}
	if _, ok := rule.IDAt(-1); ok {
		t.Fatal("negative occurrence index accepted")
	}
	if _, ok := rule.IDAt(rule.Count()); ok {
		t.Fatal("occurrence end index accepted")
	}
}

func TestContainmentEmptyMountedPointDenominatorRemainsValid(t *testing.T) {
	heapSchema, placementSchema := newContainmentSchemas(t, "placement-containment-empty", "return 1")
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			t.Fatalf("scalar-only fixture unexpectedly contains allocation root at %d", index)
		}
	}
	rule := bindContainmentLaw(t, containmentFixture{heap: heapSchema, placement: placementSchema})
	if rule == nil || rule.Count() != 1 {
		t.Fatal("empty Heap lost the singleton mounted-point closure occurrence")
	}
}

func TestContainmentRuleRoutePlanExactAndUnknownIdentityBroadcast(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	none := mustNone(t, fixture.heap)
	child := mustReference(t, fixture.heap, fixture.roots[1], materialization.Recent)
	exact := mustExact(t, fixture.heap, child)
	exactValue := mustValue(t, fixture, fixture.roots[0], none, exact, none)
	if opaque, complete := containmentEvidence(rule.heap.Schema(), exactValue); opaque || !complete {
		t.Fatalf("exact containment evidence = opaque:%t complete:%t, want false/true", opaque, complete)
	}
	if !walkContainments(rule.heap.Schema(), exactValue, func(heapdomain.Key) bool { return true }) {
		t.Fatal("exact route walk was not complete")
	}
	unknown := mustUnknown(t, fixture.heap)
	unknownValue := mustValue(t, fixture, fixture.roots[0], none, unknown, none)
	if opaque, complete := containmentEvidence(rule.heap.Schema(), unknownValue); !opaque || !complete {
		t.Fatalf("opaque containment evidence = opaque:%t complete:%t, want true/true", opaque, complete)
	}
	if walkContainments(rule.heap.Schema(), unknownValue, func(heapdomain.Key) bool { return true }) {
		t.Fatal("unknown identity route walk was treated as complete")
	}
	unknownMetaObject := mustObject(t, fixture.heap, unknown)
	unknownMetaValue := mustRelation(t, fixture.heap, fixture.roots[0], unknownMetaObject)
	if walkContainments(rule.heap.Schema(), unknownMetaValue, func(heapdomain.Key) bool { return true }) {
		t.Fatal("unknown metatable route walk was treated as complete")
	}
	top := fixture.heap.Top()
	if opaque, complete := containmentEvidence(rule.heap.Schema(), top); opaque || complete {
		t.Fatalf("Top containment walk = opaque:%t complete:%t, want false/false before authenticated Top widening", opaque, complete)
	}
	if walkContainments(rule.heap.Schema(), top, func(heapdomain.Key) bool { return true }) {
		t.Fatal("Top route walk was treated as complete")
	}
	// Unknown containment is identity uncertainty only. Both unknown values
	// remain valid, non-Top Heap values, so locate's authenticated opaque edge
	// selects the conservative all-child route set while Fold retains the known
	// parent Placement for each route. Placement.Unknown is reserved for an
	// unknown parent policy, not an unknown child identity.
	if !unknownValue.Valid() || unknownValue.IsTop() || !unknownMetaValue.Valid() || unknownMetaValue.IsTop() {
		t.Fatal("unknown containment witness changed class uncertainty")
	}

	if rule == nil || rule.Count() != 1 {
		t.Fatal("containment route denominator")
	}
	seen := make(map[heapdomain.Key]struct{})
	parentIndex, parentOK := fixture.heap.KeyIndex(fixture.roots[0])
	if !parentOK {
		t.Fatal("parent dense index")
	}
	for index := 0; index < fixture.heap.KeyCount(); index++ {
		key, keyOK := fixture.heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		tag, tagOK := routeTag(parentIndex, index)
		if !tagOK {
			t.Fatalf("broadcast route tag for dense index %d", index)
		}
		gotParent, gotChild, resolvedOK := routeIndices(fixture.placement, tag)
		if !resolvedOK || gotParent != parentIndex || gotChild != index {
			t.Fatalf("broadcast route %d resolved to %d->%d/%t", index, gotParent, gotChild, resolvedOK)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Fatal("broadcast route duplicated an allocation root")
		}
		seen[key] = struct{}{}
	}
	if len(seen) == 0 {
		t.Fatal("unknown identity route plan selected no allocation roots")
	}
}

func TestContainmentUnknownChildIdentityRetainsKnownParentPlacement(t *testing.T) {
	fixture := newContainmentFixture(t)
	unknown := mustUnknown(t, fixture.heap)
	for _, parent := range []placementdomain.Fact{
		placementdomain.DefaultFact(),
		{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven},
		{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven},
		placementdomain.UnknownFact(),
	} {
		for name, child := range map[string]heapdomain.Value{
			"opaque-edge": mustValue(t, fixture, fixture.roots[0], mustNone(t, fixture.heap), unknown, mustNone(t, fixture.heap)),
			"top":         fixture.heap.Top(),
		} {
			got, ok := containmentValue(placementdomain.DefaultFact(), parent, child)
			if !ok || got != parent {
				t.Fatalf("%s child identity parent=%v route=%v/%t, want parent placement", name, parent, got, ok)
			}
		}
	}
	shared := placementdomain.Fact{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven}
	if got, ok := containmentValue(placementdomain.DefaultFact(), shared, fixture.heap.Bottom()); ok || got != placementdomain.BottomFact() {
		t.Fatalf("Bottom child evidence route=%v/%t, want refusal", got, ok)
	}
}

func TestContainmentMissingOrForeignEvidenceRefusesWithoutWidening(t *testing.T) {
	fixture := newContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	if opaque, complete := containmentEvidence(rule.heap.Schema(), heapdomain.Value{}); opaque || complete {
		t.Fatalf("missing containment evidence = opaque:%t complete:%t, want false/false", opaque, complete)
	}
	foreign := newContainmentFixtureNamed(t, "placement-containment-foreign-evidence")
	if opaque, complete := containmentEvidence(rule.heap.Schema(), foreign.heap.Top()); opaque || complete {
		t.Fatalf("foreign containment evidence = opaque:%t complete:%t, want false/false", opaque, complete)
	}
}

func TestContainmentExactRouteTagsCanonicalizeDuplicateSemanticEdges(t *testing.T) {
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

	parentIndex, parentIndexOK := fixture.heap.KeyIndex(parent)
	if !parentIndexOK {
		t.Fatal("parent dense index")
	}
	wantTag, wantTagOK := routeTag(parentIndex, childIndex)
	if !wantTagOK {
		t.Fatal("canonical route tag")
	}
	matched := 0
	if !fixture.heap.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		if observation.Kind() != heapdomain.ContainmentExact {
			return true
		}
		reference, referenceOK := observation.Reference()
		key, _, keyOK := reference.Key()
		if !referenceOK || !keyOK || key != child {
			return true
		}
		tag, tagOK := routeTag(parentIndex, childIndex)
		if !tagOK || tag != wantTag {
			t.Fatal("duplicate exact edge changed canonical parent-child route")
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
	firstOK := walkContainments(rule.heap.Schema(), value, func(heapdomain.Key) bool { return true })
	secondOK := walkContainments(rule.heap.Schema(), value, func(_ heapdomain.Key) bool {
		seen++
		return true
	})
	if !firstOK || !secondOK || seen != exactEdges {
		t.Fatalf("wide route walk complete=%t/%t routes=%d, want %d", firstOK, secondOK, seen, exactEdges)
	}
	complete := true
	allocs := testing.AllocsPerRun(100, func() {
		count := 0
		if !walkContainments(rule.heap.Schema(), value, func(heapdomain.Key) bool { return true }) || !walkContainments(rule.heap.Schema(), value, func(_ heapdomain.Key) bool {
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

func TestContainmentWideBroadcastIsAllocationFreeAndDeterministic(t *testing.T) {
	fixture, _, _ := newWideContainmentFixture(t)
	rule := bindContainmentLaw(t, fixture)
	expected := make([]heapdomain.Key, 0)
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		key, keyOK := fixture.placement.KeyAt(dense)
		if !keyOK {
			t.Fatalf("wide broadcast dense key %d", dense)
		}
		if key.Kind() == heapdomain.RootAllocation {
			expected = append(expected, key)
		}
	}
	if len(expected) == 0 {
		t.Fatal("wide broadcast fixture has no allocation roots")
	}
	checkOrder := func() bool {
		index := 0
		complete := walkAllRoots(rule.owner.Schema(), rule.heap.Schema(), func(key heapdomain.Key) bool {
			if index >= len(expected) || expected[index] != key {
				return false
			}
			index++
			return true
		})
		return complete && index == len(expected)
	}
	if !checkOrder() || !checkOrder() {
		t.Fatal("wide broadcast route order was not deterministic")
	}
	complete := true
	allocs := testing.AllocsPerRun(100, func() {
		count := 0
		if !walkAllRoots(rule.owner.Schema(), rule.heap.Schema(), func(heapdomain.Key) bool {
			count++
			return true
		}) || count != len(expected) {
			complete = false
		}
	})
	if !complete {
		t.Fatal("wide broadcast route walk lost roots")
	}
	if allocs != 0 {
		t.Fatalf("wide broadcast route walk allocations = %f, want 0", allocs)
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

func BenchmarkContainmentClosureCount(b *testing.B) {
	fixture := newContainmentFixture(b)
	rule := bindContainmentLaw(b, fixture)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if rule.Count() != 1 {
			b.Fatal("singleton closure denominator")
		}
	}
}

func BenchmarkContainmentClosureIDAt(b *testing.B) {
	fixture := newContainmentFixture(b)
	rule := bindContainmentLaw(b, fixture)
	count := rule.Count()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for index := 0; index < count; index++ {
			if _, ok := rule.IDAt(index); !ok {
				b.Fatal("canonical closure identity")
			}
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
		if !walkContainments(rule.heap.Schema(), value, func(heapdomain.Key) bool { return true }) || !walkContainments(rule.heap.Schema(), value, func(_ heapdomain.Key) bool {
			seen++
			return true
		}) || seen != exactEdges {
			b.Fatal("wide route plan")
		}
	}
}

func BenchmarkContainmentWideBroadcast(b *testing.B) {
	fixture, _, _ := newWideContainmentFixture(b)
	rule := bindContainmentLaw(b, fixture)
	rootCount := 0
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		key, keyOK := fixture.placement.KeyAt(dense)
		if !keyOK {
			b.Fatalf("wide broadcast dense key %d", dense)
		}
		if key.Kind() == heapdomain.RootAllocation {
			rootCount++
		}
	}
	if rootCount == 0 {
		b.Fatal("wide broadcast fixture has no allocation roots")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		seen := 0
		if !walkAllRoots(rule.owner.Schema(), rule.heap.Schema(), func(heapdomain.Key) bool {
			seen++
			return true
		}) || seen != rootCount {
			b.Fatal("wide broadcast route plan")
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

// The selected read and routed write both address the child. The parent is a
// separate authenticated attribute carried by the route row; using it as the
// route key would read the parent into Current and silently fold the wrong
// cell whenever parent and child differ.
func TestContainmentRouteAddressesChildAndCarriesParentSeparately(t *testing.T) {
	fixture := newContainmentFixture(t)
	parent, child := fixture.roots[0], fixture.roots[1]
	parentFact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	route := Route{childKey: child, tag: 1, parent: parentFact}
	key, destination := route.Coordinates()
	if key != child || destination != child || key == parent {
		t.Fatalf("containment route coordinates=%v/%v, want child %v distinct from parent %v", key, destination, child, parent)
	}
	if got := route.Parent(); !placementdomain.EqualFact(got, parentFact) {
		t.Fatalf("containment route parent=%#v, want %#v", got, parentFact)
	}
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
	issuanceDirectory := testfixture.EmptyProgramIssuancePlan(t)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuanceDirectory)
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural := syntheticStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, sealFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
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
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		if category == structure.CategoryRelationGeometryScalar {
			continue
		}
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("placement-containment/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	specs = append(specs, structure.RelationGeometrySpecs()...)
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("synthetic structural declarations")
	}
	builder := seal.NewBuilder()
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
func (surface emptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
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
