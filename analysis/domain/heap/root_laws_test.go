package heap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/analysis/domain/type/typecontract"
	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// Keep the law prose compact while making the test's dependency direction
// explicit: this root suite is an external consumer of Heap's public seam.
type ArtifactMount = heapdomain.ArtifactMount
type AllocationReceipt = heapdomain.AllocationReceipt
type CellState = heapdomain.CellState
type Containment = heapdomain.Containment
type Key = heapdomain.Key
type KeySelector = heapdomain.KeySelector
type Object = heapdomain.Object
type Payload = heapdomain.Payload
type RawPresence = heapdomain.RawPresence
type Reference = heapdomain.Reference
type Schema = heapdomain.Schema
type Slot = heapdomain.Slot
type Value = heapdomain.Value
type WidenRank = heapdomain.WidenRank
type World = heapdomain.World

type Shape = heapdomain.Shape
type Frozen = heapdomain.Frozen

const (
	RootAllocation  = heapdomain.RootAllocation
	RootBoot        = heapdomain.RootBoot
	SealFailureNone = heapdomain.SealFailureNone
	KeySelectorAtom = heapdomain.KeySelectorAtom
	RawAbsent       = heapdomain.RawAbsent
	RawInvalid      = heapdomain.RawInvalid
	RawPresent      = heapdomain.RawPresent
	ShapeEligible   = heapdomain.ShapeEligible
	ShapeIneligible = heapdomain.ShapeIneligible
	FrozenMutable   = heapdomain.FrozenMutable
	FrozenFrozen    = heapdomain.FrozenFrozen
	WorldOne        = heapdomain.WorldOne
)

var (
	NewArtifactMount  = heapdomain.NewArtifactMount
	SealWithArtifacts = heapdomain.SealWithArtifacts
	Same              = heapdomain.Same
	Equal             = heapdomain.Equal
	LessOrEq          = heapdomain.LessOrEq
	Join              = heapdomain.Join
	Widen             = heapdomain.Widen
	NewWidenRank      = heapdomain.NewWidenRank
)

// compactHeapFixture deliberately enters through the current artifact-native
// seal seam.  It is intentionally small: the root laws below exercise the
// published Heap carrier, not a legacy Link/Flow fixture helper.
func compactHeapFixture(t testing.TB, name, source string, spec *target.Spec) (*proglink.Link, Schema, []ArtifactMount) {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		spec = &target.Spec{Semantics: domaincontract.NewSemantics()}
	}
	contract, err := target.Seal(spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("program schema receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := make([]ArtifactMount, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("artifact mount source")
		}
		artifact, failure := composite.CompileArtifactDetailed(program, receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("artifact compile: %v", failure)
		}
		var mountOK bool
		mounts[index], mountOK = NewArtifactMount(artifact, module, programID)
		if !mountOK {
			t.Fatal("artifact mount")
		}
	}
	schema, failure := SealWithArtifacts(linked, mounts)
	if failure != SealFailureNone || !schema.Valid() {
		t.Fatalf("heap seal: %v", failure)
	}
	return linked, schema, mounts
}

const compactHeapSource = `
local child = { value = 1 }
local record = { child = child, name = child }
return record
`

func compactAllocationKeys(t testing.TB, schema Schema, want int) []Key {
	t.Helper()
	keys := make([]Key, 0, want)
	for index := 0; index < schema.KeyCount() && len(keys) < want; index++ {
		key, ok := schema.KeyAt(index)
		if ok && key.Kind() == RootAllocation {
			keys = append(keys, key)
		}
	}
	if len(keys) < want {
		t.Fatalf("allocation roots=%d, want at least %d", len(keys), want)
	}
	return keys
}

func compactField(t testing.TB, schema Schema, key Key) (Slot, Payload, KeySelector) {
	t.Helper()
	for index := 0; index < schema.FieldCount(key); index++ {
		field, fieldOK := schema.FieldAt(key, index)
		slot, slotOK := schema.SlotForField(field)
		payload, payloadOK := schema.PayloadForField(field)
		selector, selectorOK := schema.SelectorForSlot(slot)
		if fieldOK && slotOK && payloadOK && selectorOK && selector.Kind() == KeySelectorAtom {
			return slot, payload, selector
		}
	}
	t.Fatal("allocation field")
	return Slot{}, Payload{}, KeySelector{}
}

func compactNone(t testing.TB, schema Schema) Containment {
	t.Helper()
	containment, ok := schema.ContainmentNone()
	if !ok {
		t.Fatal("none containment")
	}
	return containment
}

func compactObject(t testing.TB, schema Schema, shape Shape, frozen Frozen, metatable Containment) Object {
	t.Helper()
	object, ok := schema.Object(shape, frozen, metatable)
	if !ok {
		t.Fatal("object")
	}
	return object
}

type compactObjectStep struct {
	selector KeySelector
	state    CellState
}

func compactBuiltObject(t testing.TB, schema Schema, shape Shape, frozen Frozen, metatable Containment, steps ...compactObjectStep) Object {
	t.Helper()
	initializer, ok := schema.BeginObject(shape, frozen, metatable)
	if !ok {
		t.Fatal("object initializer")
	}
	for _, step := range steps {
		if !initializer.Apply(step.selector, step.state) {
			t.Fatal("object initializer step")
		}
	}
	object, ok := initializer.Finish()
	if !ok {
		t.Fatal("object initializer finish")
	}
	return object
}

func compactValue(t testing.TB, schema Schema, key Key, object Object) Value {
	t.Helper()
	world, worldOK := schema.One(key, object)
	value, valueOK := schema.Relation(key, world)
	if !worldOK || !valueOK {
		t.Fatal("allocation value")
	}
	return value
}

func compactPresent(t testing.TB, schema Schema, slot Slot, payload Payload, valueChild, keyChild Containment) CellState {
	t.Helper()
	state, ok := schema.CellPresent(slot, payload, valueChild, keyChild)
	if !ok {
		t.Fatal("present cell")
	}
	return state
}

func compactObjectState(t testing.TB, schema Schema, key Key, object Object, selector KeySelector) CellState {
	t.Helper()
	if !object.Valid() || !selector.Valid() || selector.Kind() != KeySelectorAtom {
		t.Fatal("object selector")
	}
	world, worldOK := schema.One(key, object)
	value, valueOK := schema.Relation(key, world)
	if !worldOK || !valueOK {
		t.Fatal("object state relation")
	}
	var state CellState
	seen := 0
	if !schema.VisitRawAccess(key, value, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		candidate, ok := access.Cell()
		if ok {
			state, seen = candidate, seen+1
		}
		return true
	}) || seen != 1 {
		t.Fatalf("partition routes=%d", seen)
	}
	return state
}

func compactObjectRaw(t testing.TB, schema Schema, key Key, object Object, selector KeySelector) RawPresence {
	t.Helper()
	world, worldOK := schema.One(key, object)
	value, valueOK := schema.Relation(key, world)
	if !worldOK || !valueOK {
		t.Fatal("object raw relation")
	}
	raw := RawInvalid
	seen := 0
	if !schema.VisitRawAccess(key, value, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		state, ok := access.Cell()
		if !ok {
			return true
		}
		candidate, candidateOK := state.Raw()
		if candidateOK {
			raw |= candidate
			seen++
		}
		return true
	}) || seen == 0 {
		t.Fatalf("raw routes=%d", seen)
	}
	return raw
}

func compactBootSpec() *target.Spec {
	return &target.Spec{
		Semantics: domaincontract.NewSemantics(),
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
	}
}

func compactModuleID(t testing.TB, linked *proglink.Link, index int) identity.ContentID {
	t.Helper()
	shard, ok := linked.Project().Mounts().At(index)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if !ok || !moduleOK {
		t.Fatal("module ID")
	}
	return module
}

func compactBootKey(t testing.TB, schema Schema) Key {
	t.Helper()
	rootID, ok := schema.BootIDAt(0)
	if !ok {
		t.Fatal("boot root")
	}
	key, keyOK := schema.KeyForBootID(rootID)
	if !keyOK {
		t.Fatal("boot key")
	}
	return key
}

func compactAssertRaw(t testing.TB, state CellState, want RawPresence) {
	t.Helper()
	raw, ok := state.Raw()
	if !ok || raw != want {
		t.Fatalf("raw=%b/%v, want %b/true", raw, ok, want)
	}
}

// TestHeapRootAgeLaws keeps Age's complete structural homomorphism explicit:
// it is total only for owned allocation roots, transports nested Recent
// references, preserves its predecessor, and commutes with the finite Join.
func TestHeapRootAgeLaws(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "compact_age", compactHeapSource, nil)
	keys := compactAllocationKeys(t, schema, 2)
	selected, other := keys[0], keys[1]
	selectedRecent, selectedRecentOK := schema.Reference(selected, materialization.Recent)
	selectedSummary, selectedSummaryOK := schema.Reference(selected, materialization.Summary)
	otherRecent, otherRecentOK := schema.Reference(other, materialization.Recent)
	slot, payload, selector := compactField(t, schema, selected)
	none := compactNone(t, schema)
	state := compactPresent(t, schema, slot, payload, compactExactContainment(t, schema, selectedRecent), compactExactContainment(t, schema, otherRecent))
	object := compactBuiltObject(t, schema, ShapeEligible, FrozenMutable, compactExactContainment(t, schema, selectedRecent), compactObjectStep{selector: selector, state: state})
	objectOK := object.Valid()
	value := compactValue(t, schema, selected, object)
	if !selectedRecentOK || !selectedSummaryOK || !otherRecentOK || !objectOK || !none.Valid() {
		t.Fatal("age setup")
	}

	for name, input := range map[string]Value{
		"bottom": schema.Bottom(),
		"top":    schema.Top(),
		"unrelated": compactValue(t, schema, other,
			compactObject(t, schema, ShapeEligible, FrozenMutable, none)),
	} {
		aged, ok := schema.Age(input, selected)
		if !ok || !Same(aged, input) {
			t.Fatalf("Age %s changed an unaffected image", name)
		}
	}
	before, beforeOK := schema.Fingerprint(value)
	aged, ageOK := schema.Age(value, selected)
	if !beforeOK || !ageOK || Same(aged, value) {
		t.Fatal("Age did not produce a changed deep image")
	}
	if after, ok := schema.Fingerprint(value); !ok || after != before {
		t.Fatal("Age mutated its predecessor")
	}
	world, worldOK := aged.WorldAt(0)
	agedObject, objectOK := world.Recent()
	agedMeta, metaOK := agedObject.MetatableAt(0)
	if !worldOK || !objectOK || !metaOK || agedMeta != selectedSummary {
		t.Fatal("Age did not transport a nested Recent reference")
	}
	agedState := compactObjectState(t, schema, selected, agedObject, selector)
	present, presentOK := agedState.PresentAt(0)
	valueChild, keyChild, childrenOK := present.Containment()
	valueRef, valueRefOK := valueChild.Reference()
	keyRef, keyRefOK := keyChild.Reference()
	if !presentOK || !childrenOK || !valueRefOK || !keyRefOK || valueRef != selectedSummary || keyRef != otherRecent {
		t.Fatal("Age lost nested containment transport")
	}
	again, againOK := schema.Age(aged, selected)
	if !againOK || !Same(again, aged) {
		t.Fatal("Age is not idempotent")
	}

	// A weakly larger object supplies comparable representatives for
	// monotonicity and Join preservation without enumerating old fixtures.
	large := compactBuiltObject(t, schema, ShapeEligible, FrozenMutable, none, compactObjectStep{selector: selector, state: state})
	largeOK := large.Valid()
	if !largeOK {
		t.Fatal("age order representative")
	}
	smallValue := schema.Bottom()
	largeValue := compactValue(t, schema, selected, large)
	if !LessOrEq(smallValue, largeValue) {
		t.Fatal("age order representatives are incomparable")
	}
	agedSmall, smallOK := schema.Age(smallValue, selected)
	agedLarge, largeOK := schema.Age(largeValue, selected)
	if !smallOK || !largeOK || !LessOrEq(agedSmall, agedLarge) {
		t.Fatal("Age is not monotone")
	}
	samples := []Value{schema.Bottom(), smallValue, value, largeValue, schema.Top()}
	for leftIndex, left := range samples {
		for rightIndex, right := range samples {
			union, unionOK := Join(left, right)
			agedUnion, agedUnionOK := schema.Age(union, selected)
			leftAge, leftAgeOK := schema.Age(left, selected)
			rightAge, rightAgeOK := schema.Age(right, selected)
			joinedAge, joinedAgeOK := Join(leftAge, rightAge)
			if !unionOK || !agedUnionOK || !leftAgeOK || !rightAgeOK || !joinedAgeOK || !Equal(agedUnion, joinedAge) {
				t.Fatalf("Age does not preserve Join for %d/%d", leftIndex, rightIndex)
			}
		}
	}

	_, foreign, _ := compactHeapFixture(t, "compact_age_foreign", `return {foreign = true}`, nil)
	foreignKey := compactAllocationKeys(t, foreign, 1)[0]
	for name, input := range map[string]struct {
		value Value
		key   Key
	}{
		"foreign value": {foreign.Top(), selected},
		"foreign key":   {schema.Top(), foreignKey},
		"zero key":      {schema.Top(), Key{}},
	} {
		if _, ok := schema.Age(input.value, input.key); ok {
			t.Fatalf("Age accepted %s", name)
		}
	}
	bootSchema := compactBootSchema(t)
	if _, ok := bootSchema.Age(bootSchema.Top(), compactBootKey(t, bootSchema)); ok {
		t.Fatal("Age accepted a Boot key")
	}
}

func compactExactContainment(t testing.TB, schema Schema, reference Reference) Containment {
	t.Helper()
	containment, ok := schema.ContainmentExact(reference)
	if !ok {
		t.Fatal("exact containment")
	}
	return containment
}

// compactBootSchema is separate so Age's allocation-only key rejection is
// checked against an actual detached bootstrap root.
func compactBootSchema(t testing.TB) Schema {
	t.Helper()
	_, schema, _ := compactHeapFixture(t, "compact_boot_age", `return {boot = true}`, compactBootSpec())
	return schema
}

// TestHeapRootLatticeLaws distinguishes exact Join from Mu/Widen and keeps a
// Many pair intact until coalescence. The fixed rank must descend at the
// changed same-family component.
func TestHeapRootLatticeLaws(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "compact_lattice", compactHeapSource, nil)
	key := compactAllocationKeys(t, schema, 1)[0]
	none := compactNone(t, schema)
	unknown, unknownOK := schema.ContainmentUnknown()
	if !unknownOK {
		t.Fatal("unknown containment")
	}
	mutable := compactObject(t, schema, ShapeEligible, FrozenMutable, none)
	frozen := compactObject(t, schema, ShapeIneligible, FrozenFrozen, unknown)
	left := compactValue(t, schema, key, mutable)
	right := compactValue(t, schema, key, frozen)
	joined, joinOK := Join(left, right)
	if !joinOK || joined.WorldCount() != 2 || !LessOrEq(left, joined) || !LessOrEq(right, joined) {
		t.Fatal("Join erased complete worlds")
	}
	widened, widenOK := Widen(left, right)
	if !widenOK || widened.WorldCount() != 1 || Equal(joined, widened) || !LessOrEq(joined, widened) {
		t.Fatal("Widen did not name its Mu loss")
	}
	world, worldOK := widened.WorldAt(0)
	recent, recentOK := world.Recent()
	shape, frozenState, headerOK := recent.Header()
	if !worldOK || !recentOK || !headerOK || shape != ShapeEligible|ShapeIneligible || frozenState != FrozenMutable|FrozenFrozen || !recent.MayHaveUnknownMetatable() {
		t.Fatal("Mu lost complete object alternatives")
	}
	if same, ok := Widen(left, left); !ok || !Same(same, left) {
		t.Fatal("Widen(v,v) rewrote its operand")
	}

	pairedLeftWorld, pairedLeftOK := schema.Many(key, mutable, frozen)
	pairedRightWorld, pairedRightOK := schema.Many(key, frozen, mutable)
	pairedLeft := compactRelation(t, schema, key, pairedLeftWorld)
	pairedRight := compactRelation(t, schema, key, pairedRightWorld)
	pairedJoin, pairedJoinOK := Join(pairedLeft, pairedRight)
	pairedWiden, pairedWidenOK := Widen(pairedLeft, pairedRight)
	if !pairedLeftOK || !pairedRightOK || !pairedJoinOK || !pairedWidenOK || pairedJoin.WorldCount() != 2 || pairedWiden.WorldCount() != 1 {
		t.Fatal("Many pairing was split or not coalesced")
	}
	pairedWorld, pairedWorldOK := pairedWiden.WorldAt(0)
	pairedRecent, pairedRecentOK := pairedWorld.Recent()
	pairedSummary, pairedSummaryOK := pairedWorld.Summary()
	if !pairedWorldOK || !pairedRecentOK || !pairedSummaryOK {
		t.Fatal("Mu dropped one Many role")
	}
	pairedRecentShape, pairedRecentFrozen, recentHeaderOK := pairedRecent.Header()
	pairedSummaryShape, pairedSummaryFrozen, summaryHeaderOK := pairedSummary.Header()
	if !recentHeaderOK || !summaryHeaderOK || pairedRecentShape != ShapeEligible|ShapeIneligible || pairedSummaryShape != ShapeEligible|ShapeIneligible || pairedRecentFrozen != FrozenMutable|FrozenFrozen || pairedSummaryFrozen != FrozenMutable|FrozenFrozen {
		t.Fatal("Mu corrupted paired role headers")
	}
	rank, rankOK := NewWidenRank(schema)
	if !rankOK || rank.Width() != 3 {
		t.Fatal("widen rank")
	}
	assertRankDescent(t, rank, key, left, widened)
	assertRankDescent(t, rank, key, pairedLeft, pairedWiden)
}

func compactRelation(t testing.TB, schema Schema, key Key, world World) Value {
	t.Helper()
	value, ok := schema.Relation(key, world)
	if !ok {
		t.Fatal("relation")
	}
	return value
}

func assertRankDescent(t testing.TB, rank WidenRank, key Key, before, after Value) {
	t.Helper()
	for component := 0; component < rank.Width(); component++ {
		beforeRank, afterRank := rank.At(key, before, component), rank.At(key, after, component)
		if afterRank < beforeRank {
			return
		}
		if afterRank > beforeRank {
			t.Fatalf("rank ascended at component %d: %d -> %d", component, beforeRank, afterRank)
		}
	}
	t.Fatal("changed Widen did not descend")
}

func TestHeapThreeObjectRankAdmissionAndOverflow(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "compact_rank", compactHeapSource, nil)
	key := compactAllocationKeys(t, schema, 1)[0]
	object := compactObject(t, schema, ShapeEligible, FrozenMutable, compactNone(t, schema))
	one, oneOK := schema.One(key, object)
	many, manyOK := schema.Many(key, object, object)
	value, valueOK := schema.Relation(key, one, many)
	rank, rankOK := NewWidenRank(schema)
	if !oneOK || !manyOK || !valueOK || !rankOK || rank.At(key, value, 2) == 0 {
		t.Fatal("three-object rank was not admitted")
	}
}

// TestHeapRootPayloadAuthorityAndRoleMatrix exercises the owner fence and the
// complete RootKind×materialization.Role matrix without exposing Link roots.
func TestHeapRootPayloadAuthorityAndRoleMatrix(t *testing.T) {
	linked, schema, _ := compactHeapFixture(t, "compact_authority", compactHeapSource, nil)
	keys := compactAllocationKeys(t, schema, 1)
	allocation := keys[0]
	if !schema.ContentID().Available() || schema.LinkContentID() != linked.ContentID() || !schema.OwnsKey(allocation) || schema.OwnsKey(Key{}) {
		t.Fatal("root authority fence")
	}
	allocationReceipt, receiptOK := allocation.AllocationReceipt()
	if !receiptOK || !allocationReceipt.Available() {
		t.Fatal("allocation receipt")
	}
	resolved, resolvedOK := schema.KeyForAllocationReceipt(allocationReceipt)
	if !resolvedOK || resolved != allocation {
		t.Fatal("allocation receipt inverse")
	}
	_, foreignSchema, _ := compactHeapFixture(t, "compact_authority_foreign", compactHeapSource, nil)
	if _, ok := schema.KeyForAllocationReceipt(mustAllocationReceipt(t, foreignSchema)); ok {
		t.Fatal("foreign allocation receipt crossed owner fence")
	}

	for _, role := range []materialization.Role{materialization.Recent, materialization.Summary} {
		if _, ok := schema.Reference(allocation, role); !ok {
			t.Fatalf("allocation role %v rejected", role)
		}
	}
	if _, ok := schema.Reference(allocation, materialization.Exact); ok {
		t.Fatal("allocation admitted Exact role")
	}
	bootSchema := compactBootSchema(t)
	boot := compactBootKey(t, bootSchema)
	if _, ok := bootSchema.Reference(boot, materialization.Exact); !ok {
		t.Fatal("boot rejected Exact role")
	}
	for _, role := range []materialization.Role{materialization.Recent, materialization.Summary} {
		if _, ok := bootSchema.Reference(boot, role); ok {
			t.Fatalf("boot admitted role %v", role)
		}
	}
	allocationObject := compactObject(t, schema, ShapeEligible, FrozenMutable, compactNone(t, schema))
	if _, ok := schema.Exact(allocation, allocationObject); ok {
		t.Fatal("allocation admitted Exact world")
	}
	if _, ok := schema.One(boot, allocationObject); ok {
		t.Fatal("boot admitted One world")
	}
	bootObject := compactObject(t, bootSchema, ShapeEligible, FrozenMutable, compactNone(t, bootSchema))
	if _, ok := bootSchema.Exact(boot, bootObject); !ok {
		t.Fatal("boot rejected Exact world")
	}
	if _, ok := bootSchema.Zero(boot); ok {
		t.Fatal("boot admitted Zero world")
	}

	slot, payload, selector := compactField(t, schema, allocation)
	foreignSlot, foreignPayload, _ := compactField(t, foreignSchema, compactAllocationKeys(t, foreignSchema, 1)[0])
	none := compactNone(t, schema)
	foreignNone := compactNone(t, foreignSchema)
	if _, ok := schema.CellPresent(foreignSlot, payload, none, none); ok {
		t.Fatal("foreign slot admitted")
	}
	if _, ok := schema.CellPresent(slot, foreignPayload, none, none); ok {
		t.Fatal("foreign payload admitted")
	}
	if _, ok := schema.CellPresent(slot, payload, foreignNone, none); ok {
		t.Fatal("foreign containment admitted")
	}
	state := compactPresent(t, schema, slot, payload, none, none)
	object := compactBuiltObject(t, schema, ShapeEligible, FrozenMutable, none, compactObjectStep{selector: selector, state: state})
	value := compactValue(t, schema, allocation, object)
	if !schema.Admits(allocation, value) {
		t.Fatal("owner-issued payload value rejected")
	}
}

func mustAllocationReceipt(t testing.TB, schema Schema) AllocationReceipt {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if ok && key.Kind() == RootAllocation {
			receipt, receiptOK := key.AllocationReceipt()
			if receiptOK {
				return receipt
			}
		}
	}
	t.Fatal("allocation receipt")
	return AllocationReceipt{}
}

func TestHeapOccurrenceInverseLaws(t *testing.T) {
	linked, schema, mounts := compactHeapFixture(t, "compact_occurrences", compactHeapSource, nil)
	module := compactModuleID(t, linked, 0)
	issuer, issuerOK := schema.OccurrenceMountForModule(module)
	if !issuerOK || issuer.Module() != module || issuer.AllocationCount() == 0 {
		t.Fatal("occurrence mount")
	}
	for index := 0; index < issuer.AllocationCount(); index++ {
		id, key, ok := issuer.AllocationAt(index)
		if !ok || !id.Available() || !key.Valid() || key.Kind() != RootAllocation {
			t.Fatalf("allocation occurrence %d", index)
		}
		inverse, inverseOK := issuer.AllocationRootForOccurrence(id)
		ordinal, ordinalOK := issuer.AllocationOrdinal(id)
		if !inverseOK || inverse != key || !ordinalOK || ordinal != index {
			t.Fatalf("allocation inverse %d: %#v/%v ordinal=%d/%v", index, inverse, inverseOK, ordinal, ordinalOK)
		}
	}
	for index := 0; index < schema.IndexAccessCount(); index++ {
		access, accessOK := schema.IndexAccessAt(index)
		moduleID, occurrence, read, occurrenceOK := schema.IndexAccessOccurrence(access)
		if !accessOK || !occurrenceOK {
			t.Fatal("index occurrence")
		}
		if moduleID != module {
			t.Fatal("index module")
		}
		inverse, inverseOK := issuer.IndexAccessForOccurrence(occurrence, read)
		if !inverseOK || inverse != access {
			t.Fatal("index occurrence inverse")
		}
	}
	zeroRoot, zeroRootOK := issuer.AllocationRootForOccurrence(identity.ContentID{})
	zeroOrdinal, zeroOrdinalOK := issuer.AllocationOrdinal(identity.ContentID{})
	if zeroRootOK || zeroRoot.Valid() || zeroOrdinalOK || zeroOrdinal != 0 {
		t.Fatal("zero occurrence admitted")
	}
	foreignLinked, foreignSchema, _ := compactHeapFixture(t, "compact_occurrences_foreign", `return {foreign = {}}`, nil)
	foreignModule := compactModuleID(t, foreignLinked, 0)
	foreignIssuer, foreignIssuerOK := foreignSchema.OccurrenceMountForModule(foreignModule)
	if !foreignIssuerOK {
		t.Fatal("foreign occurrence mount")
	}
	foreignID, _, foreignIDOK := foreignIssuer.AllocationAt(0)
	if !foreignIDOK {
		t.Fatal("foreign allocation occurrence")
	}
	if _, ok := issuer.AllocationRootForOccurrence(foreignID); ok {
		t.Fatal("foreign occurrence crossed issuer fence")
	}
	if _, ok := schema.OccurrenceMountForModule(foreignModule); ok {
		t.Fatal("foreign module admitted by local schema")
	}
	if schema.IndexAccessCount() > 0 {
		access, accessOK := schema.IndexAccessAt(0)
		_, occurrence, read, occurrenceOK := schema.IndexAccessOccurrence(access)
		if accessOK && occurrenceOK {
			if _, ok := foreignIssuer.IndexAccessForOccurrence(occurrence, read); ok {
				t.Fatal("foreign index occurrence crossed issuer fence")
			}
		}
	}
	if len(mounts) == 0 {
		t.Fatal("occurrence mounts")
	}

	zeroLinked, zeroSchema, zeroMounts := compactHeapFixture(t, "compact_occurrences_zero", `return 1`, nil)
	zeroModule := compactModuleID(t, zeroLinked, 0)
	zeroIssuer, zeroIssuerOK := zeroSchema.OccurrenceMountForModule(zeroModule)
	if !zeroIssuerOK || zeroIssuer.AllocationCount() != 0 || len(zeroMounts) == 0 {
		t.Fatal("zero-allocation occurrence mount")
	}
	if _, _, ok := zeroIssuer.AllocationAt(0); ok {
		t.Fatal("zero-allocation mount issued a root")
	}
}

func TestHeapInitializerPartitionAndMetatableLaws(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "compact_initializer", compactHeapSource, nil)
	key := compactAllocationKeys(t, schema, 1)[0]
	slot, payload, selector := compactField(t, schema, key)
	none := compactNone(t, schema)
	present := compactPresent(t, schema, slot, payload, none, none)
	absent, absentOK := schema.CellAbsent()
	initializer, initializerOK := schema.BeginObject(ShapeEligible, FrozenMutable, none)
	if !absentOK || !initializerOK || !initializer.Apply(selector, present) || !initializer.Apply(selector, absent) {
		t.Fatal("ordered initializer")
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		t.Fatal("initializer finish")
	}
	compactAssertRaw(t, compactObjectState(t, schema, key, object, selector), RawAbsent)
	if initializer.Apply(selector, present) {
		t.Fatal("initializer reused after finish")
	}
	if _, ok := initializer.Finish(); ok {
		t.Fatal("initializer finished twice")
	}

	withPresent := compactBuiltObject(t, schema, ShapeEligible, FrozenMutable, none, compactObjectStep{selector: selector, state: present})
	kindsSelector, kindsOK := schema.KindSelector()
	withAbsent := compactBuiltObject(t, schema, ShapeEligible, FrozenMutable, none, compactObjectStep{selector: selector, state: present}, compactObjectStep{selector: kindsSelector, state: absent})
	withPresentOK, withAbsentOK := withPresent.Valid(), kindsOK && withAbsent.Valid()
	if !withPresentOK || !withAbsentOK {
		t.Fatal("partition updates")
	}
	compactAssertRaw(t, compactObjectState(t, schema, key, withPresent, selector), RawPresent)
	if got := compactObjectRaw(t, schema, key, withAbsent, selector); got != RawAbsent|RawPresent {
		t.Fatalf("weak partition raw=%b, want %b", got, RawAbsent|RawPresent)
	}

	unknown, unknownOK := schema.ContainmentUnknown()
	reference, referenceOK := schema.Reference(key, materialization.Recent)
	exact := compactExactContainment(t, schema, reference)
	noMeta := compactObject(t, schema, ShapeEligible, FrozenMutable, none)
	unknownMeta := compactObject(t, schema, ShapeEligible, FrozenMutable, unknown)
	exactMeta := compactObject(t, schema, ShapeEligible, FrozenMutable, exact)
	if !unknownOK || !referenceOK || !noMeta.MayHaveNoMetatable() || !unknownMeta.MayHaveUnknownMetatable() || exactMeta.MetatableCount() != 1 {
		t.Fatal("metatable carrier states")
	}
	noMetaValue := compactValue(t, schema, key, noMeta)
	exactValue := compactValue(t, schema, key, exactMeta)
	unknownValue := compactValue(t, schema, key, unknownMeta)
	merged, mergedOK := Widen(noMetaValue, exactValue)
	merged, unknownMergeOK := Widen(merged, unknownValue)
	mergedWorld, mergedWorldOK := merged.WorldAt(0)
	mergedObject, mergedObjectOK := mergedWorld.Recent()
	if !mergedOK || !unknownMergeOK || !mergedWorldOK || !mergedObjectOK || !mergedObject.MayHaveNoMetatable() || !mergedObject.MayHaveUnknownMetatable() || mergedObject.MetatableCount() != 1 {
		t.Fatal("metatable join lost a three-state alternative")
	}
}
