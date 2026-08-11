package heap

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestRawAccessKeepsWorldAndManyRoleCorrelation(t *testing.T) {
	_, schema := heapFixture(t, "heap_raw_access_worlds")
	key, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	recent, recentOK := overwriteObjectCell(mutableObject(t, schema), selector, present)
	if !recentOK {
		t.Fatal("recent object")
	}
	summary := mutableObject(t, schema)
	many, manyOK := schema.Many(key, recent, summary)
	fact, factOK := schema.Relation(key, many)
	if !manyOK || !factOK {
		t.Fatal("many fact")
	}
	var selected RawAccess
	seen := 0
	if !schema.VisitRawAccess(key, fact, materialization.Recent, selector, func(access RawAccess) bool {
		cell, ok := access.Cell()
		raw, rawOK := cell.Raw()
		if !ok || !rawOK || raw != RawPresent {
			return true
		}
		selected, seen = access, seen+1
		return true
	}) || seen != 1 {
		t.Fatalf("recent raw routes=%d, want one", seen)
	}
	branches, writeOK := schema.RawDelete(selected, MutationLicence{})
	normal, normalOK := branches.Normal()
	if !writeOK || !normalOK || branches.FrozenError() {
		t.Fatalf("weak delete branches = normal:%v error:%v ok:%v", normalOK, branches.FrozenError(), writeOK)
	}
	if normal.WorldCount() != 1 {
		t.Fatalf("normal worlds=%d, want one complete Many world", normal.WorldCount())
	}
	world, worldOK := normal.WorldAt(0)
	updatedRecent, recentOK := world.Recent()
	unchangedSummary, summaryOK := world.Summary()
	if !worldOK || !recentOK || !summaryOK {
		t.Fatal("normal successor lost Many roles")
	}
	recentState := initializerPartitionState(t, updatedRecent, selector)
	summaryState := initializerPartitionState(t, unchangedSummary, selector)
	recentRaw, recentRawOK := recentState.Raw()
	summaryRaw, summaryRawOK := summaryState.Raw()
	if !recentRawOK || recentRaw != RawAbsent|RawPresent {
		t.Fatalf("selected Recent weak delete=%b/%v, want absent|present", recentRaw, recentRawOK)
	}
	if !summaryRawOK || summaryRaw != RawAbsent {
		t.Fatalf("unselected Summary changed=%b/%v, want absent", summaryRaw, summaryRawOK)
	}

	// The same role may retain incomparable complete worlds.  Raw access must
	// expose each one rather than merging headers/cells before transfer.
	other, otherOK := schema.Object(ShapeIneligible, FrozenMutable, noneContainment(t, schema))
	other, otherOK = overwriteObjectCell(other, selector, present)
	if !otherOK {
		t.Fatal("second world object")
	}
	one, oneOK := schema.One(key, recent)
	otherWorld, otherWorldOK := schema.One(key, other)
	multi, multiOK := schema.Relation(key, one, otherWorld)
	if !oneOK || !otherWorldOK || !multiOK || multi.WorldCount() != 2 {
		t.Fatal("incomparable one worlds")
	}
	worlds := 0
	if !schema.VisitRawAccess(key, multi, materialization.Recent, selector, func(access RawAccess) bool {
		cell, ok := access.Cell()
		raw, rawOK := cell.Raw()
		if ok && rawOK && raw == RawPresent {
			worlds++
		}
		return true
	}) || worlds != 2 {
		t.Fatalf("complete world routes=%d, want two", worlds)
	}
}

func TestRawAccessBootSlotPolicyAndKindResidual(t *testing.T) {
	schema, key, fact, slots := rawBootFixture(t, FrozenMutable)
	frozenSelector := slots["frozen"]
	mutableSelector := slots["mutable"]
	var frozen RawAccess
	if !schema.VisitRawAccess(key, fact, materialization.Exact, frozenSelector, func(access RawAccess) bool {
		if access.InitialFrozen() {
			frozen = access
		}
		return true
	}) || !frozen.Valid() {
		t.Fatal("missing frozen boot access")
	}
	frozenBranches, frozenOK := schema.RawDelete(frozen, MutationLicence{})
	if !frozenOK || frozenBranches.FrozenError() != true {
		t.Fatal("InitialFrozen boot slot did not take error branch")
	}
	if _, normal := frozenBranches.Normal(); normal {
		t.Fatal("InitialFrozen boot slot produced a normal mutation")
	}
	var absent RawAccess
	if !schema.VisitRawAccess(key, fact, materialization.Exact, slots["absent"], func(access RawAccess) bool {
		if access.InitialFrozen() {
			absent = access
		}
		return true
	}) || !absent.Valid() {
		t.Fatal("missing frozen absent boot access")
	}
	if branches, ok := schema.RawDelete(absent, MutationLicence{}); !ok || !branches.FrozenError() {
		t.Fatal("InitialFrozen absent row lost its mutation policy")
	}

	var mutable RawAccess
	if !schema.VisitRawAccess(key, fact, materialization.Exact, mutableSelector, func(access RawAccess) bool {
		if !access.InitialFrozen() {
			mutable = access
		}
		return true
	}) || !mutable.Valid() {
		t.Fatal("missing mutable boot access")
	}
	mutableBranches, mutableOK := schema.RawDelete(mutable, MutationLicence{})
	if !mutableOK || mutableBranches.FrozenError() {
		t.Fatal("mutable boot slot was confused with table.freeze")
	}
	if _, normal := mutableBranches.Normal(); !normal {
		t.Fatal("mutable boot slot lost normal mutation")
	}

	strings, stringsOK := schema.KindsSelector(runtimekind.Bit(runtimekind.String))
	if !stringsOK {
		t.Fatal("string kind selector")
	}
	var residual RawAccess
	residualFound := false
	frozenErrors, normalRoutes := 0, 0
	if !schema.VisitRawAccess(key, fact, materialization.Exact, strings, func(access RawAccess) bool {
		if access.InitialFrozen() {
			branches, ok := schema.RawDelete(access, MutationLicence{})
			if !ok || !branches.FrozenError() {
				t.Fatal("kind selector lost frozen exact branch")
			}
			frozenErrors++
			return true
		}
		branches, ok := schema.RawDelete(access, MutationLicence{})
		if !ok {
			t.Fatal("kind selector normal branch")
		}
		if _, normal := branches.Normal(); normal {
			normalRoutes++
		}
		if access.fragment.residual && !residualFound {
			residual = access
			residualFound = true
		}
		return true
	}) || frozenErrors == 0 || normalRoutes == 0 || !residualFound || !residual.Valid() {
		t.Fatalf("kind split frozen:%d normal:%d residual:%v", frozenErrors, normalRoutes, residual.Valid())
	}
	residualBranches, residualOK := schema.RawDelete(residual, MutationLicence{})
	normal, normalOK := residualBranches.Normal()
	if !residualOK || !normalOK {
		t.Fatal("residual normal branch")
	}
	world, worldOK := normal.WorldAt(0)
	object, objectOK := world.Exact()
	if !worldOK || !objectOK {
		t.Fatal("residual successor exact world")
	}
	frozenState := initializerPartitionState(t, object, frozenSelector)
	raw, rawOK := frozenState.Raw()
	if !rawOK || raw != RawPresent {
		t.Fatalf("kind residual mutated InitialFrozen slot: %b/%v", raw, rawOK)
	}

	// Object header freeze is independent: a mutable boot slot must error when
	// its table header is frozen.
	var headerAccess RawAccess
	frozenSchema, frozenKey, frozenFact, frozenSlots := rawBootFixture(t, FrozenFrozen)
	if !frozenSchema.VisitRawAccess(frozenKey, frozenFact, materialization.Exact, frozenSlots["mutable"], func(access RawAccess) bool {
		if !access.InitialFrozen() {
			headerAccess = access
		}
		return true
	}) || !headerAccess.Valid() {
		t.Fatal("header-frozen mutable access")
	}
	headerBranches, headerOK := frozenSchema.RawDelete(headerAccess, MutationLicence{})
	if !headerOK || !headerBranches.FrozenError() {
		t.Fatal("table.freeze did not reject mutable boot slot")
	}
	if _, normal := headerBranches.Normal(); normal {
		t.Fatal("table.freeze produced normal mutation")
	}
}

func TestRawAccessTagsSharePayloadOnlyAndFenceOwners(t *testing.T) {
	linked, schema := heapFixture(t, "heap_raw_access_tags")
	key, fields := rawStringFields(t, linked, schema)
	if len(fields) < 2 {
		t.Fatal("fixture omitted two exact string fields")
	}
	object := mutableObject(t, schema)
	for _, field := range fields[:2] {
		next, ok := overwriteObjectCell(object, field.selector, field.state)
		if !ok {
			t.Fatal("field object")
		}
		object = next
	}
	one, oneOK := schema.One(key, object)
	fact, factOK := schema.Relation(key, one)
	strings, stringsOK := schema.KindsSelector(runtimekind.Bit(runtimekind.String))
	if !oneOK || !factOK || !stringsOK {
		t.Fatal("tag fixture")
	}
	tags := make(map[RawPayloadTag]Present)
	if !schema.VisitRawAccess(key, fact, materialization.Recent, strings, func(access RawAccess) bool {
		present, ok := access.cell.PresentAt(0)
		if !ok {
			return true
		}
		tag, tagOK := access.PayloadTag(present)
		if !tagOK {
			t.Fatal("payload tag")
		}
		if prior, duplicate := tags[tag]; duplicate && comparePresent(prior, present) != 0 {
			t.Fatal("distinct payloads cross-paired")
		}
		tags[tag] = present
		return true
	}) || len(tags) < 2 {
		t.Fatalf("distinct payload tags=%d, want at least two", len(tags))
	}

	// Two complete world alternatives carrying the same sealed payload source
	// intentionally receive the same Payload tag; their own Heap containment
	// remains attached to their separate RawAccess values.
	other, otherOK := schema.Object(ShapeIneligible, FrozenMutable, noneContainment(t, schema))
	other, otherOK = overwriteObjectCell(other, fields[0].selector, fields[0].state)
	first, firstOK := schema.One(key, object)
	second, secondOK := schema.One(key, other)
	multi, multiOK := schema.Relation(key, first, second)
	if !otherOK || !firstOK || !secondOK || !multiOK {
		t.Fatal("shared payload worlds")
	}
	var shared RawPayloadTag
	count := 0
	if !schema.VisitRawAccess(key, multi, materialization.Recent, fields[0].selector, func(access RawAccess) bool {
		present, ok := access.cell.PresentAt(0)
		if !ok {
			return true
		}
		tag, ok := access.PayloadTag(present)
		if !ok {
			t.Fatal("shared tag")
		}
		if count != 0 && tag != shared {
			t.Fatal("same payload did not share tag")
		}
		shared, count = tag, count+1
		return true
	}) || count != 2 {
		t.Fatalf("shared payload routes=%d, want two", count)
	}

	_, foreign := heapFixture(t, "heap_raw_access_foreign")
	foreignKey, foreignSlot, foreignPayload := allocationKeyWithField(t, foreign)
	foreignSelector := exactSelectorForSlot(t, foreign, foreignSlot)
	foreignFact := valueWithFieldContainment(t, foreign, foreignKey, foreignSelector, foreignSlot, foreignPayload, noneContainment(t, foreign))
	var foreignPresent Present
	if !foreign.VisitRawAccess(foreignKey, foreignFact, materialization.Recent, foreignSelector, func(access RawAccess) bool {
		foreignPresent, _ = access.cell.PresentAt(0)
		return true
	}) || !foreignPresent.valid() {
		t.Fatal("foreign present")
	}
	var local RawAccess
	if !schema.VisitRawAccess(key, fact, materialization.Recent, fields[0].selector, func(access RawAccess) bool {
		if access.cell.PresentCount() == 1 {
			local = access
		}
		return true
	}) || !local.Valid() {
		t.Fatal("local access")
	}
	if _, ok := local.PayloadTag(foreignPresent); ok {
		t.Fatal("foreign payload crossed Heap owner fence")
	}
	if _, ok := foreign.RawDelete(local, MutationLicence{}); ok {
		t.Fatal("foreign Heap accepted a local raw access")
	}
}

func TestRawAccessStagedTransportProjections(t *testing.T) {
	linked, schema := heapFixture(t, "heap_raw_access_staged_transport")
	key, fields := rawStringFields(t, linked, schema)
	if len(fields) < 2 {
		t.Fatal("fixture omitted two exact string fields")
	}
	object := mutableObject(t, schema)
	for _, field := range fields[:2] {
		next, ok := overwriteObjectCell(object, field.selector, field.state)
		if !ok {
			t.Fatal("field object")
		}
		object = next
	}
	world, worldOK := schema.One(key, object)
	fact, factOK := schema.Relation(key, world)
	strings, stringsOK := schema.KindsSelector(runtimekind.Bit(runtimekind.String))
	if !worldOK || !factOK || !stringsOK {
		t.Fatal("staged transport fixture")
	}

	route, routeOK := schema.RouteTag(key, materialization.Recent)
	if !routeOK || route == 0 {
		t.Fatal("issue staged route")
	}
	if want, ok := rawRouteTag(key, materialization.Recent); !ok || route != want {
		t.Fatal("public route projection changed its canonical code")
	}
	var direct, staged []RawAccess
	if !schema.VisitRawAccess(key, fact, materialization.Recent, strings, func(access RawAccess) bool {
		direct = append(direct, access)
		return true
	}) {
		t.Fatal("direct raw projection")
	}
	if !schema.VisitRawAccessRoute(route, fact, strings, func(access RawAccess) bool {
		staged = append(staged, access)
		return true
	}) {
		t.Fatal("staged raw projection")
	}
	if !reflect.DeepEqual(staged, direct) {
		t.Fatal("staged raw projection diverged from VisitRawAccess")
	}

	var wantPayload Payload
	var payloadTag RawPayloadTag
	for _, access := range staged {
		cell, ok := access.Cell()
		if !ok || cell.PresentCount() != 1 {
			continue
		}
		present, ok := cell.PresentAt(0)
		if !ok {
			t.Fatal("single staged present")
		}
		wantPayload, ok = present.Payload()
		if !ok {
			t.Fatal("staged present payload")
		}
		payloadTag, ok = access.PayloadTag(present)
		if !ok || payloadTag == 0 {
			t.Fatal("issue staged payload")
		}
		break
	}
	gotPayload, payloadOK := schema.PayloadForRawTag(payloadTag)
	if !payloadOK || gotPayload != wantPayload {
		t.Fatal("staged payload did not resolve to its sealed source")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = schema.RouteTag(key, materialization.Recent)
		_, _ = schema.PayloadForRawTag(payloadTag)
	}); allocations != 0 {
		t.Fatalf("staged tag issue/resolve allocations=%v, want zero", allocations)
	}

	if _, ok := schema.RouteTag(Key{}, materialization.Recent); ok {
		t.Fatal("zero key issued a route")
	}
	if _, ok := schema.RouteTag(key, materialization.Invalid); ok {
		t.Fatal("invalid role issued a route")
	}
	if schema.VisitRawAccessRoute(0, fact, strings, func(RawAccess) bool { return true }) {
		t.Fatal("zero route visited raw access")
	}
	invalidRoute := RawRouteTag(uint64(len(schema.owner.roots)+1)<<8 | uint64(materialization.Recent))
	if schema.VisitRawAccessRoute(invalidRoute, fact, strings, func(RawAccess) bool { return true }) {
		t.Fatal("out-of-schema route visited raw access")
	}
	if _, ok := schema.PayloadForRawTag(0); ok {
		t.Fatal("zero payload tag resolved")
	}
	if _, ok := schema.PayloadForRawTag(RawPayloadTag(len(schema.owner.payloads) + 1)); ok {
		t.Fatal("out-of-schema payload tag resolved")
	}

	_, foreign := heapFixture(t, "heap_raw_access_staged_transport_foreign")
	foreignKey, foreignSlot, foreignPayload := allocationKeyWithField(t, foreign)
	foreignSelector := exactSelectorForSlot(t, foreign, foreignSlot)
	foreignFact := valueWithFieldContainment(t, foreign, foreignKey, foreignSelector, foreignSlot, foreignPayload, noneContainment(t, foreign))
	foreignRoute, foreignRouteOK := foreign.RouteTag(foreignKey, materialization.Recent)
	if !foreignRouteOK {
		t.Fatal("foreign route")
	}
	if _, ok := schema.RouteTag(foreignKey, materialization.Recent); ok {
		t.Fatal("foreign key crossed route issuer fence")
	}
	if schema.VisitRawAccessRoute(foreignRoute, foreignFact, foreignSelector, func(RawAccess) bool { return true }) {
		t.Fatal("foreign fact and selector crossed staged route fence")
	}
}

func TestRawAccessInitialPayloadRetainsSelectedBootSource(t *testing.T) {
	schema, key, fact, slots := rawBootFixture(t, FrozenMutable)
	var raw RawAccess
	var present Present
	if !schema.VisitRawAccess(key, fact, materialization.Exact, slots["frozen"], func(access RawAccess) bool {
		cell, cellOK := access.Cell()
		var selected bool
		present, selected = cell.PresentAt(0)
		raw = access
		return cellOK && selected
	}) || !raw.Valid() {
		t.Fatal("boot raw present")
	}
	root, initial, projected := raw.InitialPayload(present)
	if !projected || root == (linkhost.BootRoot{}) || initial == 0 {
		t.Fatal("selected boot payload did not retain root and initial source")
	}
	if _, _, projected := raw.InitialPayload(Present{}); projected {
		t.Fatal("foreign initial payload crossed selected raw route")
	}
}

func TestRawPayloadTagVisitIsCanonicalCompleteAndWarm(t *testing.T) {
	_, schema := heapFixture(t, "heap_raw_payload_tag_visit")
	tags := make([]RawPayloadTag, 0)
	if !schema.VisitRawPayloadTags(func(tag RawPayloadTag, payload Payload) bool {
		resolved, ok := schema.PayloadForRawTag(tag)
		if !ok || !payload.valid() || resolved != payload || tag != RawPayloadTag(len(tags)+1) {
			t.Fatal("raw payload tag visit lost canonical payload correlation")
		}
		tags = append(tags, tag)
		return true
	}) || len(tags) == 0 {
		t.Fatal("raw payload tag visit omitted sealed universe")
	}
	if _, ok := schema.PayloadForRawTag(0); ok {
		t.Fatal("zero raw payload tag was accepted")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !schema.VisitRawPayloadTags(func(RawPayloadTag, Payload) bool { return true }) {
			panic("warm raw payload tag visit")
		}
	}); allocations != 0 {
		t.Fatalf("warm raw payload tag visit allocated %v times", allocations)
	}
}

func TestRawAccessTopIsExplicitNormalAndFrozenPossibility(t *testing.T) {
	_, schema := heapFixture(t, "heap_raw_access_top")
	key, slot, _ := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	var top RawAccess
	if !schema.VisitRawAccess(key, schema.Top(), materialization.Recent, selector, func(access RawAccess) bool {
		top = access
		return true
	}) || !top.Valid() || !top.IsTop() {
		t.Fatal("Heap Top did not produce an explicit raw route")
	}
	if _, ok := top.Object(); ok {
		t.Fatal("Heap Top fabricated an Object")
	}
	if _, ok := top.Cell(); ok {
		t.Fatal("Heap Top fabricated a CellState")
	}
	branches, ok := schema.RawDelete(top, MutationLicence{})
	normal, normalOK := branches.Normal()
	if !ok || !normalOK || !normal.IsTop() || !branches.FrozenError() {
		t.Fatalf("Top delete = normal:%v top:%v frozen:%v ok:%v", normalOK, normal.IsTop(), branches.FrozenError(), ok)
	}
	wantRoute, routeOK := rawRouteTag(key, materialization.Recent)
	if !routeOK || top.RouteTag() != wantRoute {
		t.Fatal("Top lost root/role route identity")
	}
}

type rawField struct {
	selector KeySelector
	state    CellState
}

func rawStringFields(t testing.TB, linked *proglink.Link, schema Schema) (Key, []rawField) {
	t.Helper()
	for rootIndex := 0; rootIndex < schema.KeyCount(); rootIndex++ {
		key, ok := schema.KeyAt(rootIndex)
		if !ok || key.Kind() != RootAllocation {
			continue
		}
		fields := make([]rawField, 0)
		for fieldIndex := 0; fieldIndex < schema.FieldCount(key); fieldIndex++ {
			field, ok := schema.FieldAt(key, fieldIndex)
			if !ok {
				continue
			}
			slot, slotOK := schema.SlotForField(field)
			payload, payloadOK := schema.PayloadForField(field)
			kind, exact, _, _, originOK := slot.Origin()
			literal, literalOK := linked.Project().Keys().Exact(exact)
			if !slotOK || !payloadOK || !originOK || kind != SlotExact || !literalOK || literal.Kind != keyspace.LiteralString {
				continue
			}
			selector := exactSelectorForSlot(t, schema, slot)
			fields = append(fields, rawField{selector: selector, state: stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))})
		}
		if len(fields) >= 2 {
			return key, fields
		}
	}
	t.Fatal("no allocation root with two exact string fields")
	return Key{}, nil
}

func rawBootFixture(t testing.TB, frozen Frozen) (Schema, Key, Value, map[string]KeySelector) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "raw_boot.lua", Text: []byte("return 1\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "frozen"}, Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 1}, Mutability: target.InitialFrozen},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "mutable"}, Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 2}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialFrozen},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "raw-boot", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("seal boot Heap")
	}
	var key Key
	slots := make(map[string]KeySelector)
	none := noneContainment(t, schema)
	init, initOK := schema.BeginObject(ShapeEligible, frozen, none)
	if !initOK {
		t.Fatal("boot initializer")
	}
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, keyOK := entry.Key()
		slot, slotOK := entry.Slot()
		raw, payload, projectionOK := entry.Projection()
		kind, exact, _, _, originOK := slot.Origin()
		literal, literalOK := linked.Project().Keys().Exact(exact)
		selector, selectorOK := schema.SelectorForSlot(slot)
		if !entryOK || !keyOK || !slotOK || !projectionOK || !originOK || kind != SlotExact || !literalOK || literal.Kind != keyspace.LiteralString || !selectorOK {
			t.Fatal("boot entry projection")
		}
		key = entryKey
		slots[literal.String] = selector
		var state CellState
		var stateOK bool
		switch raw {
		case RawAbsent:
			state, stateOK = schema.CellAbsent()
		case RawPresent:
			state, stateOK = schema.CellPresent(slot, payload, none, none)
		default:
			t.Fatal("invalid boot raw presence")
		}
		if !stateOK || !init.Apply(selector, state) {
			t.Fatal("apply boot entry")
		}
	}
	object, objectOK := init.Finish()
	world, worldOK := schema.Exact(key, object)
	value, valueOK := schema.Relation(key, world)
	if !objectOK || !worldOK || !valueOK || len(slots) != 3 {
		t.Fatal("boot value")
	}
	return schema, key, value, slots
}
