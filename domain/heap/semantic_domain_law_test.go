package heap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// These laws use Heap's post-seal artifact receipts.  The fixture helper is
// deliberately local to the test archive: it does not recreate the retired
// Link/Flow declarations that production transfer must no longer consume.

func TestHeapReceiptAgeJoinWidenMuAndRankLaws(t *testing.T) {
	fixture := newSemanticHeapFixture(t, "heap_receipt_algebra", `
local child = { value = 1 }
local record = { child = child, name = child }
return record
`, declaration.Spec{Semantics: domaincontract.NewSemantics()})
	key, _, _, _, _ := semanticExactField(t, fixture.schema)
	keys := semanticAllocationKeys(t, fixture.schema)
	if len(keys) < 2 {
		t.Fatal("receipt algebra fixture omitted two allocation roots")
	}
	other := keys[0]
	if other == key {
		other = keys[1]
	}

	none, noneOK := fixture.schema.ContainmentNone()
	otherRecent, otherRecentOK := fixture.schema.Reference(other, materialization.Recent)
	otherMeta, otherMetaOK := fixture.schema.ContainmentExact(otherRecent)
	leftObject, leftObjectOK := fixture.schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	rightObject, rightObjectOK := fixture.schema.Object(heapdomain.ShapeIneligible, heapdomain.FrozenFrozen, otherMeta)
	left, leftOK := semanticRelation(fixture.schema, key, leftObject)
	right, rightOK := semanticRelation(fixture.schema, key, rightObject)
	joined, joinedOK := heapdomain.Join(left, right)
	if !noneOK || !otherRecentOK || !otherMetaOK || !leftObjectOK || !rightObjectOK || !leftOK || !rightOK || !joinedOK || joined.WorldCount() != 2 {
		t.Fatal("receipt Join erased complete-world alternatives")
	}
	if !heapdomain.LessOrEq(left, joined) || !heapdomain.LessOrEq(right, joined) {
		t.Fatal("receipt Join is not an upper bound")
	}

	widened, widenedOK := heapdomain.Widen(left, right)
	if !widenedOK || widened.WorldCount() != 1 || heapdomain.Equal(joined, widened) || !heapdomain.LessOrEq(left, widened) || !heapdomain.LessOrEq(right, widened) {
		t.Fatal("receipt Mu did not coalesce one complete family above Join")
	}
	rank, rankOK := heapdomain.NewWidenRank(fixture.schema)
	if !rankOK || rank.Width() != 3 {
		t.Fatal("receipt WidenRank")
	}
	strictDescent := false
	for component := 0; component < rank.Width(); component++ {
		before := rank.At(key, left, component)
		after := rank.At(key, widened, component)
		if after > before {
			t.Fatalf("Mu rank ascended at component %d: %d -> %d", component, before, after)
		}
		strictDescent = strictDescent || after < before
	}
	if !strictDescent {
		t.Fatal("Mu widening did not descend the finite rank")
	}
	idempotent, idempotentOK := heapdomain.Widen(joined, joined)
	if !idempotentOK || !heapdomain.Equal(idempotent, joined) {
		t.Fatal("Widen(v,v) changed an already joined value")
	}

	// Age transports every selected Recent reference, including nested
	// metatable and raw value/key containment, while leaving the predecessor
	// immutable and an unrelated root's Recent reference unchanged.
	_, field, slot, payload, selector := semanticExactField(t, fixture.schema)
	_ = field
	selectedRecent, selectedRecentOK := fixture.schema.Reference(key, materialization.Recent)
	selectedMeta, selectedMetaOK := fixture.schema.ContainmentExact(selectedRecent)
	selectedValue, selectedValueOK := fixture.schema.ContainmentExact(selectedRecent)
	otherKey, otherKeyOK := fixture.schema.ContainmentExact(otherRecent)
	deepCell, deepCellOK := fixture.schema.CellPresent(slot, payload, selectedValue, otherKey)
	initializer, initializerOK := fixture.schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, selectedMeta)
	if initializerOK {
		initializerOK = initializer.Apply(selector, deepCell)
	}
	deepObject, deepObjectOK := initializer.Finish()
	input, inputOK := semanticRelation(fixture.schema, key, deepObject)
	beforeFingerprint, beforeFingerprintOK := fixture.schema.Fingerprint(input)
	aged, agedOK := fixture.schema.Age(input, key)
	if !selectedRecentOK || !selectedMetaOK || !selectedValueOK || !otherKeyOK || !deepCellOK || !initializerOK || !deepObjectOK || !inputOK || !beforeFingerprintOK || !agedOK || heapdomain.Equal(input, aged) {
		t.Fatal("Age did not produce a changed immutable nested image")
	}
	if afterFingerprint, fingerprintOK := fixture.schema.Fingerprint(input); !fingerprintOK || afterFingerprint != beforeFingerprint {
		t.Fatal("Age mutated its predecessor")
	}
	agedWorld, agedWorldOK := aged.WorldAt(0)
	agedObject, agedObjectOK := agedWorld.Recent()
	agedMeta, agedMetaOK := agedObject.MetatableAt(0)
	selectedSummary, selectedSummaryOK := fixture.schema.Reference(key, materialization.Summary)
	if !agedWorldOK || !agedObjectOK || !agedMetaOK || !selectedSummaryOK || agedMeta != selectedSummary {
		t.Fatal("Age failed to transport the metatable Recent reference")
	}
	if summarySelector, summarySelectorOK := fixture.schema.ReferenceSelector(selectedSummary); summarySelectorOK || summarySelector.Valid() {
		t.Fatal("Summary reference acquired an exact key selector")
	}
	seenTransportedContainment := false
	if !fixture.schema.VisitRawAccess(key, aged, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		cell, cellOK := access.Cell()
		present, presentOK := cell.PresentAt(0)
		valueChild, keyChild, containmentOK := present.Containment()
		valueReference, valueOK := valueChild.Reference()
		keyReference, keyOK := keyChild.Reference()
		if cellOK && presentOK && containmentOK && valueOK && keyOK && valueReference == selectedSummary && keyReference == otherRecent {
			seenTransportedContainment = true
		}
		return true
	}) || !seenTransportedContainment {
		t.Fatal("Age lost nested value/key containment transport")
	}
	agedLeft, agedLeftOK := fixture.schema.Age(left, key)
	agedRight, agedRightOK := fixture.schema.Age(right, key)
	agedJoin, agedJoinOK := fixture.schema.Age(joined, key)
	joinedAges, joinedAgesOK := heapdomain.Join(agedLeft, agedRight)
	if !agedLeftOK || !agedRightOK || !agedJoinOK || !joinedAgesOK || !heapdomain.Equal(agedJoin, joinedAges) {
		t.Fatal("Age did not preserve Join")
	}
	if !heapdomain.LessOrEq(left, joined) || !heapdomain.LessOrEq(agedLeft, agedJoin) {
		t.Fatal("Age was not monotone across a Join upper bound")
	}
	ageAgain, ageAgainOK := fixture.schema.Age(aged, key)
	if !ageAgainOK || !heapdomain.Same(ageAgain, aged) {
		t.Fatal("Age was not idempotent")
	}
	for _, value := range []heapdomain.Value{fixture.schema.Bottom(), fixture.schema.Top()} {
		agedValue, valueOK := fixture.schema.Age(value, key)
		if !valueOK || !heapdomain.Equal(agedValue, value) {
			t.Fatal("Age changed a lattice endpoint")
		}
	}
}

func TestCreateSuccessorIsWidenProgressNotInclusion(t *testing.T) {
	fixture := newSemanticHeapFixture(t, "heap_create_successor", `local t = { x = 1 }; return t`, declaration.Spec{Semantics: domaincontract.NewSemantics()})
	key, _, _, _, _ := semanticExactField(t, fixture.schema)
	zero, zeroOK := fixture.schema.EmptyObject(key)
	object, objectOK := fixture.schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, mustContainmentNone(t, fixture.schema))
	created, createdOK := fixture.schema.Create(zero, key, object)
	if !zeroOK || !objectOK || !createdOK {
		t.Fatal("Create successor fixture")
	}
	if heapdomain.LessOrEq(zero, created) || heapdomain.LessOrEq(created, zero) {
		t.Fatal("Create successor must remain a distinct control family")
	}
	widened, widenedOK := heapdomain.Widen(zero, created)
	if !widenedOK || !heapdomain.LessOrEq(zero, widened) || !heapdomain.LessOrEq(created, widened) {
		t.Fatal("Create successor must have a defined widening upper bound")
	}
}

func TestHeapReceiptAlgebraRejectsForeignKeysAndContainments(t *testing.T) {
	local := newSemanticHeapFixture(t, "heap_receipt_owner_local", `local t = { x = 1 }; return t`, declaration.Spec{Semantics: domaincontract.NewSemantics()})
	foreign := newSemanticHeapFixture(t, "heap_receipt_owner_foreign", `local t = { x = 1 }; return t`, declaration.Spec{Semantics: domaincontract.NewSemantics()})
	localKey, _, _, _, _ := semanticExactField(t, local.schema)
	foreignKey, _, _, _, _ := semanticExactField(t, foreign.schema)
	localEmpty, localEmptyOK := local.schema.EmptyObject(localKey)
	foreignObject, foreignObjectOK := foreign.schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, mustContainmentNone(t, foreign.schema))
	if !localEmptyOK || !foreignObjectOK {
		t.Fatal("foreign algebra fixture")
	}
	if _, ok := local.schema.Age(localEmpty, foreignKey); ok {
		t.Fatal("Age accepted a foreign allocation key")
	}
	if _, ok := local.schema.Create(localEmpty, foreignKey, foreignObject); ok {
		t.Fatal("Create accepted a foreign allocation key/object")
	}
	if _, ok := local.schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, mustContainmentNone(t, foreign.schema)); ok {
		t.Fatal("Object constructor accepted foreign containment")
	}
	foreignValue, foreignValueOK := foreign.schema.Relation(foreignKey, mustWorld(t, foreign.schema, foreignKey, foreignObject))
	if !foreignValueOK {
		t.Fatal("foreign relation")
	}
	if _, ok := heapdomain.Join(localEmpty, foreignValue); ok {
		t.Fatal("Join accepted values from different Heap owners")
	}
	if _, ok := heapdomain.Widen(localEmpty, foreignValue); ok {
		t.Fatal("Widen accepted values from different Heap owners")
	}
}

func TestHeapReceiptOccurrenceMountInversesRoundTripForeignAndWarm(t *testing.T) {
	fixture := newSemanticHeapFixture(t, "heap_receipt_occurrence_local", `
local table = {}
return table.field
`, semanticIndexSpec())
	issuer, issuerOK := fixture.schema.OccurrenceMountForModule(fixture.module)
	if !issuerOK {
		t.Fatal("occurrence mount issuer")
	}
	allocationCount := issuer.AllocationCount()
	if allocationCount == 0 {
		t.Fatal("occurrence fixture omitted allocation receipts")
	}
	allocationID, allocationKey, allocationOK := issuer.AllocationAt(0)
	root, rootOK := issuer.AllocationRootForOccurrence(allocationID)
	ordinal, ordinalOK := issuer.AllocationOrdinal(allocationID)
	if !allocationOK || !rootOK || root != allocationKey || !ordinalOK || ordinal != 0 {
		t.Fatal("allocation occurrence inverse roundtrip")
	}
	if _, ok := issuer.AllocationRootForOccurrence(identity.ContentID{}); ok {
		t.Fatal("zero allocation occurrence was admitted")
	}
	if allocations := testing.AllocsPerRun(200, func() {
		got, ok := issuer.AllocationRootForOccurrence(allocationID)
		if !ok || got != allocationKey {
			t.Fatal("warm allocation occurrence inverse")
		}
	}); allocations != 0 {
		t.Fatalf("warm allocation occurrence inverse allocated %v", allocations)
	}

	indexCount := fixture.schema.IndexAccessCount()
	if indexCount == 0 {
		t.Fatal("occurrence fixture omitted index accesses")
	}
	var firstAccess heapdomain.IndexAccess
	var firstID identity.ContentID
	var firstRead bool
	for index := 0; index < indexCount; index++ {
		access, accessOK := fixture.schema.IndexAccessAt(index)
		module, occurrence, read, inverseOK := fixture.schema.IndexAccessOccurrence(access)
		if !accessOK || !inverseOK || module != fixture.module || !occurrence.Available() {
			t.Fatalf("index occurrence row %d", index)
		}
		mounted, mountedOK := fixture.schema.OccurrenceMountForModule(module)
		got, gotOK := mounted.IndexAccessForOccurrence(occurrence, read)
		if !mountedOK || !gotOK || got != access {
			t.Fatalf("index occurrence inverse row %d", index)
		}
		if firstID == (identity.ContentID{}) {
			firstAccess, firstID, firstRead = access, occurrence, read
		}
	}
	if allocations := testing.AllocsPerRun(200, func() {
		got, ok := issuer.IndexAccessForOccurrence(firstID, firstRead)
		if !ok || got != firstAccess {
			t.Fatal("warm index occurrence inverse")
		}
	}); allocations != 0 {
		t.Fatalf("warm index occurrence inverse allocated %v", allocations)
	}

	foreign := newSemanticHeapFixture(t, "heap_receipt_occurrence_foreign", `local table = {}; return table.field`, semanticIndexSpec())
	foreignIssuer, foreignIssuerOK := foreign.schema.OccurrenceMountForModule(foreign.module)
	if !foreignIssuerOK {
		t.Fatal("foreign occurrence mount issuer")
	}
	foreignID, _, foreignAllocationOK := foreignIssuer.AllocationAt(0)
	foreignAccess, foreignAccessOK := foreignIssuer.IndexAccessForOccurrence(firstID, firstRead)
	if !foreignAllocationOK || foreignAccessOK {
		t.Fatal("foreign occurrence crossed the artifact fence")
	}
	if _, ok := fixture.schema.OccurrenceMountForModule(foreign.module); ok {
		t.Fatal("foreign module acquired a local occurrence issuer")
	}
	_ = foreignID
	_ = foreignAccess
	if _, ok := issuer.IndexAccessForOccurrence(identity.ContentID{}, firstRead); ok {
		t.Fatal("zero index occurrence was admitted")
	}
}

func TestHeapReceiptInitializerPartitionMetatableAndRawAccessLaws(t *testing.T) {
	fixture := newSemanticHeapFixture(t, "heap_receipt_initializer", `
local child = {}
local record = { child = child }
return record.field
`, semanticIndexSpec())
	key, _, slot, payload, selector := semanticExactField(t, fixture.schema)
	none := mustContainmentNone(t, fixture.schema)
	present, presentOK := fixture.schema.CellPresent(slot, payload, none, none)
	absent, absentOK := fixture.schema.CellAbsent()
	initializer, initializerOK := fixture.schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if initializerOK {
		initializerOK = initializer.Apply(selector, present)
	}
	prefix := initializer
	deleted := prefix
	retained := prefix
	if initializerOK {
		initializerOK = deleted.Apply(selector, absent) && retained.Apply(selector, present)
	}
	deletedObject, deletedOK := deleted.Finish()
	retainedObject, retainedOK := retained.Finish()
	prefixObject, prefixOK := prefix.Finish()
	_, initializerFinishOK := initializer.Finish()
	if !presentOK || !absentOK || !initializerOK || !initializerFinishOK || !deletedOK || !retainedOK || !prefixOK {
		t.Fatal("receipt initializer branch construction")
	}
	deletedValue := mustRelation(t, fixture.schema, key, deletedObject)
	retainedValue := mustRelation(t, fixture.schema, key, retainedObject)
	prefixValue := mustRelation(t, fixture.schema, key, prefixObject)
	if rawMasks(t, fixture.schema, key, deletedValue, selector) != heapdomain.RawAbsent {
		t.Fatal("exact initializer delete did not replace the prior cell")
	}
	if rawMasks(t, fixture.schema, key, retainedValue, selector) != heapdomain.RawPresent || rawMasks(t, fixture.schema, key, prefixValue, selector) != heapdomain.RawPresent {
		t.Fatal("initializer branch mutated its immutable prefix")
	}
	if initializer.Apply(selector, present) {
		t.Fatal("consumed initializer accepted reuse")
	}
	if _, ok := initializer.Finish(); ok {
		t.Fatal("consumed initializer published twice")
	}

	// A kind selector is a weak partition update. It must preserve the exact
	// exception while also retaining the residual possibility for the kind.
	weak, weakOK := fixture.schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	kinds, kindsOK := fixture.schema.KindSelector()
	if weakOK {
		weakOK = weak.Apply(selector, present) && weak.Apply(kinds, absent)
	}
	weakObject, weakObjectOK := weak.Finish()
	weakValue, weakValueOK := semanticRelation(fixture.schema, key, weakObject)
	if !weakOK || !kindsOK || !weakObjectOK || !weakValueOK {
		t.Fatal("weak partition initializer")
	}
	exactRaw := rawMasks(t, fixture.schema, key, weakValue, selector)
	if exactRaw != heapdomain.RawAbsent|heapdomain.RawPresent {
		t.Fatalf("weak update erased exact exception: %b", exactRaw)
	}
	weakRaw := rawMasks(t, fixture.schema, key, weakValue, kinds)
	if weakRaw != heapdomain.RawAbsent|heapdomain.RawPresent {
		t.Fatalf("weak update lost residual possibility: %b", weakRaw)
	}

	otherKey := semanticOtherAllocation(t, fixture.schema, key)
	exactReference := mustReference(t, fixture.schema, otherKey, materialization.Recent)
	unknown := mustContainmentUnknown(t, fixture.schema)
	exactContainment := mustContainmentExact(t, fixture.schema, exactReference)
	noMeta, noMetaOK := fixture.schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	unknownMeta, unknownMetaOK := fixture.schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, unknown)
	exactMeta, exactMetaOK := fixture.schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, exactContainment)
	if !noMetaOK || !unknownMetaOK || !exactMetaOK || !noMeta.MayHaveNoMetatable() || !unknownMeta.MayHaveUnknownMetatable() || exactMeta.MetatableCount() != 1 {
		t.Fatal("metatable carrier alternatives")
	}
	meta, metaOK := exactMeta.MetatableAt(0)
	if !metaOK || meta != exactReference {
		t.Fatal("exact metatable carrier lost its reference")
	}

	// RawAccess carries complete provenance and policy, not a naked cell.
	var localAccess heapdomain.RawAccess
	if !fixture.schema.VisitRawAccess(key, retainedValue, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		localAccess = access
		return true
	}) || !localAccess.Valid() || localAccess.IsTop() {
		t.Fatal("exact RawAccess provenance")
	}
	cell, cellOK := localAccess.Cell()
	presentTuple, tupleOK := cell.PresentAt(0)
	payloadTag, payloadTagOK := localAccess.PayloadTag(presentTuple)
	payloadFromTag, payloadFromTagOK := fixture.schema.PayloadForRawTag(payloadTag)
	if !cellOK || !tupleOK || !payloadTagOK || !payloadFromTagOK || payloadFromTag != payload {
		t.Fatal("RawAccess payload tag lost typed provenance")
	}
	if _, _, initialPayloadOK := localAccess.InitialPayload(presentTuple); initialPayloadOK {
		t.Fatal("Program payload acquired Target initial provenance")
	}
	route, routeOK := fixture.schema.RouteTag(key, materialization.Recent)
	var routedAccess heapdomain.RawAccess
	if !routeOK || !fixture.schema.VisitRawAccessRoute(route, retainedValue, selector, func(access heapdomain.RawAccess) bool {
		routedAccess = access
		return true
	}) || !routedAccess.Valid() {
		t.Fatal("staged RawAccess route diverged from direct route")
	}
	routedCell, routedCellOK := routedAccess.Cell()
	routedRaw, routedRawOK := routedCell.Raw()
	if !routedCellOK || !routedRawOK || routedRaw != rawPresent() {
		t.Fatal("staged RawAccess changed its selected cell")
	}

	// Top has no fabricated object/cell, but RawDelete(Top) deliberately
	// retains both outcomes: the ordinary Top successor and the frozen/error
	// possibility.  InitialFrozen below is a different policy and is frozen
	// only.
	topAccesses := 0
	if !fixture.schema.VisitRawAccess(key, fixture.schema.Top(), materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		topAccesses++
		if !access.IsTop() {
			t.Fatal("Top RawAccess lost its top marker")
		}
		if _, ok := access.Object(); ok {
			t.Fatal("Top RawAccess fabricated an object")
		}
		if _, ok := access.Cell(); ok {
			t.Fatal("Top RawAccess fabricated a cell")
		}
		branches, ok := fixture.schema.RawDelete(access, heapdomain.MutationLicence{})
		if !ok || !branches.FrozenError() {
			t.Fatal("Top RawAccess did not retain its frozen mutation branch")
		}
		normal, normalOK := branches.Normal()
		if !normalOK || !heapdomain.Equal(normal, fixture.schema.Top()) {
			t.Fatal("Top RawAccess lost its normal Top branch")
		}
		return true
	}) || topAccesses != 1 {
		t.Fatal("Top RawAccess route")
	}

	// Typed geometry retains read/write direction, payload placement, and
	// occurrence identity in the same sealed schema used by RawAccess.
	reads, writes := 0, 0
	for index := 0; index < fixture.schema.IndexAccessCount(); index++ {
		access, accessOK := fixture.schema.IndexAccessAt(index)
		geometry, geometryOK := fixture.schema.IndexAccessGeometry(access)
		module, occurrence, read, occurrenceOK := fixture.schema.IndexAccessOccurrence(access)
		if !accessOK || !geometryOK || !occurrenceOK || module != fixture.module || !occurrence.Available() || geometry.Module != fixture.module || !geometry.ProgramID.Available() || geometry.Read != read {
			t.Fatalf("typed geometry row %d", index)
		}
		if read {
			reads++
			if geometry.Position != -1 {
				t.Fatalf("read geometry position=%d", geometry.Position)
			}
			if _, payloadOK := fixture.schema.PayloadForIndexAccess(access); payloadOK {
				t.Fatal("read geometry retained a write payload")
			}
			if _, tagOK := fixture.schema.RawPayloadTagForIndexAccess(access); tagOK {
				t.Fatal("read geometry retained a payload tag")
			}
		} else {
			writes++
			if geometry.Position < 0 {
				t.Fatalf("write geometry position=%d", geometry.Position)
			}
			payload, payloadOK := fixture.schema.PayloadForIndexAccess(access)
			tag, tagOK := fixture.schema.RawPayloadTagForIndexAccess(access)
			resolved, resolvedOK := fixture.schema.PayloadForRawTag(tag)
			if !payloadOK || !tagOK || !resolvedOK || payload != resolved {
				t.Fatal("write geometry lost payload/tag provenance")
			}
		}
	}
	if reads == 0 {
		t.Fatalf("typed geometry reads=%d writes=%d", reads, writes)
	}

	foreign := newSemanticHeapFixture(t, "heap_receipt_raw_foreign", `local t = { x = 1 }; return t.field`, declaration.Spec{Semantics: domaincontract.NewSemantics()})
	foreignKey, _, foreignSlot, foreignPayload, foreignSelector := semanticExactField(t, foreign.schema)
	foreignCell, foreignCellOK := foreign.schema.CellPresent(foreignSlot, foreignPayload, mustContainmentNone(t, foreign.schema), mustContainmentNone(t, foreign.schema))
	foreignObject, foreignObjectOK := foreign.schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, mustContainmentNone(t, foreign.schema))
	if foreignObjectOK {
		foreignObjectOK = foreignObject.Apply(foreignSelector, foreignCell)
	}
	foreignFinished, foreignFinishedOK := foreignObject.Finish()
	foreignValue, foreignValueOK := semanticRelation(foreign.schema, foreignKey, foreignFinished)
	var foreignAccess heapdomain.RawAccess
	if foreignObjectOK && foreignCellOK && foreignFinishedOK && foreignValueOK {
		foreign.schema.VisitRawAccess(foreignKey, foreignValue, materialization.Recent, foreignSelector, func(access heapdomain.RawAccess) bool {
			foreignAccess = access
			return true
		})
	}
	foreignCellValue, foreignCellValueOK := foreignAccess.Cell()
	foreignPresent, foreignPresentOK := foreignCellValue.PresentAt(0)
	if !foreignAccess.Valid() || !foreignCellValueOK || !foreignPresentOK {
		t.Fatal("foreign RawAccess fixture")
	}
	if _, ok := localAccess.PayloadTag(foreignPresent); ok {
		t.Fatal("foreign Present crossed RawAccess owner fence")
	}
	if _, ok := fixture.schema.RawDelete(foreignAccess, heapdomain.MutationLicence{}); ok {
		t.Fatal("foreign RawAccess crossed mutation owner fence")
	}
}

func TestHeapReceiptBootstrapRawAccessInitialPolicy(t *testing.T) {
	fixture := newSemanticHeapFixture(t, "heap_receipt_boot_policy", `return 1`, semanticBootSpec())
	bootID, bootIDOK := fixture.schema.BootIDAt(0)
	key, keyOK := fixture.schema.KeyForBootID(bootID)
	frozenSelector, frozenEntryOK := semanticFrozenBootSelector(t, fixture.schema, key)
	value, valueOK := semanticBootValue(t, fixture.schema, key)
	if !bootIDOK || !keyOK || !frozenEntryOK || !valueOK {
		t.Fatal("bootstrap RawAccess policy fixture")
	}
	seenFrozen := false
	if !fixture.schema.VisitRawAccess(key, value, materialization.Exact, frozenSelector, func(access heapdomain.RawAccess) bool {
		if !access.Valid() || !access.InitialFrozen() {
			t.Fatal("initial frozen RawAccess lost policy")
		}
		cell, cellOK := access.Cell()
		present, presentOK := cell.PresentAt(0)
		rootID, initial, initialOK := access.InitialPayload(present)
		if !cellOK || !presentOK || !initialOK || rootID != bootID || initial == 0 {
			t.Fatal("initial frozen RawAccess lost Target payload provenance")
		}
		branches, branchesOK := fixture.schema.RawDelete(access, heapdomain.MutationLicence{})
		if !branchesOK || !branches.FrozenError() {
			t.Fatal("initial frozen RawAccess admitted a delete")
		}
		// Unlike Heap.Top, an InitialFrozen cell has no ordinary successor.
		if _, normal := branches.Normal(); normal {
			t.Fatal("initial frozen RawAccess produced a normal mutation")
		}
		seenFrozen = true
		return true
	}) || !seenFrozen {
		t.Fatal("initial frozen RawAccess route")
	}
}

type semanticHeapFixtureRecord struct {
	linked  *link.Link
	schema  heapdomain.Schema
	mount   programmount.MountedArtifact
	module  identity.ContentID
	program identity.ContentID
}

func newSemanticHeapFixture(t testing.TB, name, text string, spec declaration.Spec) semanticHeapFixtureRecord {
	t.Helper()
	if spec.Semantics == nil {
		spec.Semantics = domaincontract.NewSemantics()
	}
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programOK := linked.Project().Mounts().ProgramID(shard)
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	compiled, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, compiled), module)
	schema, sealFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	if !shardOK || !moduleOK || !programOK || !compilationOK || !executionSchemaID.Available() || !issuanceOK || failure.Available() || !mountOK || sealFailure != heapdomain.SealFailureNone {
		t.Fatalf("receipt Heap fixture shard=%t module=%t program=%t compilation=%t artifact=%v mount=%t seal=%v", shardOK, moduleOK, programOK, compilationOK, failure, mountOK, sealFailure)
	}
	return semanticHeapFixtureRecord{linked: linked, schema: schema, mount: mount, module: module, program: programID}
}

func semanticAllocationKeys(t testing.TB, schema heapdomain.Schema) []heapdomain.Key {
	t.Helper()
	keys := make([]heapdomain.Key, 0)
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if ok && key.Kind() == heapdomain.RootAllocation {
			keys = append(keys, key)
		}
	}
	return keys
}

func semanticExactField(t testing.TB, schema heapdomain.Schema) (heapdomain.Key, heapdomain.Field, heapdomain.Slot, heapdomain.Payload, heapdomain.KeySelector) {
	t.Helper()
	for _, key := range semanticAllocationKeys(t, schema) {
		for index := 0; index < schema.FieldCount(key); index++ {
			field, fieldOK := schema.FieldAt(key, index)
			slot, slotOK := schema.SlotForField(field)
			payload, payloadOK := schema.PayloadForField(field)
			kind, _, _, originOK := slot.Origin()
			selector, selectorOK := schema.SelectorForSlot(slot)
			if fieldOK && slotOK && payloadOK && originOK && selectorOK && kind == heapdomain.SlotExact && selector.Kind() == heapdomain.KeySelectorAtom {
				return key, field, slot, payload, selector
			}
		}
	}
	t.Fatal("receipt fixture omitted an exact allocation field")
	return heapdomain.Key{}, heapdomain.Field{}, heapdomain.Slot{}, heapdomain.Payload{}, heapdomain.KeySelector{}
}

func semanticOtherAllocation(t testing.TB, schema heapdomain.Schema, selected heapdomain.Key) heapdomain.Key {
	t.Helper()
	for _, key := range semanticAllocationKeys(t, schema) {
		if key != selected {
			return key
		}
	}
	t.Fatal("receipt fixture omitted an independent allocation")
	return heapdomain.Key{}
}

func semanticRelation(schema heapdomain.Schema, key heapdomain.Key, object heapdomain.Object) (heapdomain.Value, bool) {
	world, worldOK := schema.One(key, object)
	if !worldOK {
		return heapdomain.Value{}, false
	}
	return schema.Relation(key, world)
}

func mustRelation(t testing.TB, schema heapdomain.Schema, key heapdomain.Key, object heapdomain.Object) heapdomain.Value {
	t.Helper()
	value, ok := semanticRelation(schema, key, object)
	if !ok {
		t.Fatal("Heap relation")
	}
	return value
}

func mustWorld(t testing.TB, schema heapdomain.Schema, key heapdomain.Key, object heapdomain.Object) heapdomain.World {
	t.Helper()
	world, ok := schema.One(key, object)
	if !ok {
		t.Fatal("Heap world")
	}
	return world
}

func mustContainmentNone(t testing.TB, schema heapdomain.Schema) heapdomain.Containment {
	t.Helper()
	containment, ok := schema.ContainmentNone()
	if !ok {
		t.Fatal("Heap None containment")
	}
	return containment
}

func mustContainmentUnknown(t testing.TB, schema heapdomain.Schema) heapdomain.Containment {
	t.Helper()
	containment, ok := schema.ContainmentUnknown()
	if !ok {
		t.Fatal("Heap Unknown containment")
	}
	return containment
}

func mustContainmentExact(t testing.TB, schema heapdomain.Schema, reference heapdomain.Reference) heapdomain.Containment {
	t.Helper()
	containment, ok := schema.ContainmentExact(reference)
	if !ok {
		t.Fatal("Heap Exact containment")
	}
	return containment
}

func mustReference(t testing.TB, schema heapdomain.Schema, key heapdomain.Key, role materialization.Role) heapdomain.Reference {
	t.Helper()
	reference, ok := schema.Reference(key, role)
	if !ok {
		t.Fatal("Heap reference")
	}
	return reference
}

func rawMasks(t testing.TB, schema heapdomain.Schema, key heapdomain.Key, value heapdomain.Value, selector heapdomain.KeySelector) heapdomain.RawPresence {
	t.Helper()
	var mask heapdomain.RawPresence
	if !schema.VisitRawAccess(key, value, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		cell, cellOK := access.Cell()
		raw, rawOK := cell.Raw()
		if !cellOK || !rawOK {
			t.Fatal("RawAccess cell")
		}
		mask |= raw
		return true
	}) {
		t.Fatal("RawAccess projection")
	}
	return mask
}

func rawPresent() heapdomain.RawPresence { return heapdomain.RawPresent }

func semanticBootSpec() declaration.Spec {
	return declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "frozen"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueInteger, Integer: 1}, Mutability: vocabulary.InitialFrozen},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
	}
}

func semanticIndexSpec() declaration.Spec {
	return declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}}
}

func semanticBootValue(t testing.TB, schema heapdomain.Schema, key heapdomain.Key) (heapdomain.Value, bool) {
	t.Helper()
	frozen, frozenOK := schema.BootFrozen(key)
	none, noneOK := schema.ContainmentNone()
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, frozen, none)
	if !frozenOK || !noneOK || !initializerOK {
		return heapdomain.Value{}, false
	}
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, entryKeyOK := entry.Key()
		if !entryOK || !entryKeyOK || entryKey != key {
			continue
		}
		slot, slotOK := entry.Slot()
		selector, selectorOK := schema.SelectorForSlot(slot)
		raw, payload, projectionOK := entry.Projection()
		if !slotOK || !selectorOK || !projectionOK {
			return heapdomain.Value{}, false
		}
		var state heapdomain.CellState
		var stateOK bool
		if raw == heapdomain.RawAbsent {
			state, stateOK = schema.CellAbsent()
		} else {
			containment, containmentOK := entry.ValueContainment()
			if !containmentOK {
				return heapdomain.Value{}, false
			}
			state, stateOK = schema.CellPresent(slot, payload, containment, none)
		}
		if !stateOK || !initializer.Apply(selector, state) {
			return heapdomain.Value{}, false
		}
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		return heapdomain.Value{}, false
	}
	world, worldOK := schema.Exact(key, object)
	if !worldOK {
		return heapdomain.Value{}, false
	}
	return schema.Relation(key, world)
}

func semanticFrozenBootSelector(t testing.TB, schema heapdomain.Schema, key heapdomain.Key) (heapdomain.KeySelector, bool) {
	t.Helper()
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, keyOK := entry.Key()
		mutability, mutabilityOK := entry.Mutability()
		if !entryOK || !keyOK || !mutabilityOK || entryKey != key || mutability != vocabulary.InitialFrozen {
			continue
		}
		slot, slotOK := entry.Slot()
		selector, selectorOK := schema.SelectorForSlot(slot)
		return selector, slotOK && selectorOK
	}
	return heapdomain.KeySelector{}, false
}
