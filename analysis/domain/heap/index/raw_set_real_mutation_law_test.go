package index

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// TestRawSetTransferEvidenceUsesRealRouteAndHeapMutation exercises the shared
// reducer that both transfer and evidence invoke. The route is an actual
// Heap RawRoute, and the two cases reach the owner RawStore and RawDelete
// branches respectively. Allocation counts are recorded after warm-up so a
// future rule cannot replace the owner mutation with a synthetic shape-only
// disposition.
func TestRawSetTransferEvidenceUsesRealRouteAndHeapMutation(t *testing.T) {
	cases := []struct {
		name   string
		source string
		kind   rawPayloadKind
	}{
		{name: "store", source: `local id = {}; local tags = {}; tags["source"] = id; return tags`, kind: rawPayloadFixed},
		{name: "delete", source: `local record = {}; record.first, record.second = 1; return record`, kind: rawPayloadNil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := rawSetRealMutationFixture(t, testCase.source, testCase.kind)
			if !rawSetSelectionShape(fixture.access, fixture.descriptor, fixture.view) {
				t.Fatal("RawSet transfer/evidence route shape")
			}
			if !fixture.visitRoute() {
				t.Fatal("Heap RawRoute visit")
			}
			result, reduced := fixture.rule.mutateRoute(fixture.access, fixture.route, fixture.fact, fixture.view)
			if !reduced || !result.Valid() || result.IsBottom() || heapdomain.Equal(result, fixture.fact) {
				t.Fatal("RawSet real Heap mutation")
			}
			routeAllocs := testing.AllocsPerRun(50, func() {
				if !fixture.visitRoute() {
					panic("Heap RawRoute visit")
				}
			})
			mutationAllocs := testing.AllocsPerRun(20, func() {
				if next, nextOK := fixture.rule.mutateRoute(fixture.access, fixture.route, fixture.fact, fixture.view); !nextOK || !next.Valid() {
					panic("RawSet real Heap mutation")
				}
			})
			// The route walk is a solve-local read and should remain allocation
			// free once its sealed values are warm. RawStore/RawDelete are
			// immutable Heap transitions, so they are expected to allocate a
			// successor; the law records that they were actually reached.
			if routeAllocs != 0 {
				t.Fatalf("warm Heap RawRoute walk allocated %v times", routeAllocs)
			}
			if mutationAllocs == 0 {
				t.Fatalf("RawSet %s mutation did not reach an immutable Heap transition (allocs=%v)", testCase.name, mutationAllocs)
			}
			t.Logf("route allocations=%v, %s mutation allocations=%v", routeAllocs, testCase.name, mutationAllocs)
		})
	}
}

type rawSetRealMutation struct {
	rule       *RawSetRule
	access     Access
	descriptor rawPayload
	view       rawSetView
	route      heapdomain.RawRouteTag
	fact       heapdomain.Value
	visitRoute func() bool
}

func rawSetRealMutationFixture(t testing.TB, source string, want rawPayloadKind) rawSetRealMutation {
	t.Helper()
	topology, valueSchema, heapSchema, _, packSchema := rawSetPayloadTopology(t, source)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawSetKey(101), rawSetKey(102), valueSchema)
	heap, heapOK := heapowner.Declare(composition, rawSetKey(103), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawSetKey(104), packSchema)
	rule, ruleOK := DeclareRawSet(composition, rawSetKey(105), rawSetKey(106), rawSetKey(107), topology, values, heap, packs)
	if !valuesOK || !heapOK || !packsOK || !ruleOK || rule == nil {
		t.Fatal("real RawSet declaration")
	}

	var access Access
	var descriptor rawSetPayload
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, candidateOK := heapSchema.IndexAccessAt(index)
		candidateAccess, accessOK := topology.Access(candidate)
		if !candidateOK || !accessOK || !candidateAccess.Write() {
			continue
		}
		candidateDescriptor, descriptorOK := rule.payloadForWrite(candidateAccess)
		if descriptorOK && candidateDescriptor.descriptor.kind == want {
			access, descriptor = candidateAccess, candidateDescriptor
			break
		}
	}
	if !rule.owns(access) {
		t.Fatalf("missing %d write access", want)
	}
	selector, _, selectorOK := staticSetSelector(rule, access)
	if !selectorOK {
		t.Fatal("real RawSet key selector")
	}
	none, noneOK := heapSchema.ContainmentNone()
	slot, slotOK := access.Slot()
	payload, payloadOK := heapSchema.PayloadForIndexAccess(access.indexAccess)
	if !noneOK || !slotOK || !payloadOK {
		t.Fatal("real RawSet slot/payload")
	}

	// Pick a real Heap root/role for which this existing write slot is
	// supported. No root identity is constructed: RouteTag and Relation are
	// both reissued by the sealed Heap Schema.
	var route heapdomain.RawRouteTag
	var fact heapdomain.Value
	var visit func() bool
	for keyIndex := 0; keyIndex < heapSchema.KeyCount() && route == 0; keyIndex++ {
		key, keyOK := heapSchema.KeyAt(keyIndex)
		if !keyOK {
			continue
		}
		for _, role := range []materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary} {
			candidateRoute, routeOK := heapSchema.RouteTag(key, role)
			if !routeOK {
				continue
			}
			builder, builderOK := heapSchema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
			if !builderOK {
				continue
			}
			// RawAccess only exposes an existing selected coordinate. Seed the
			// same authored cell for both cases: the fixed row then exercises
			// RawStore replacement, while NilFill exercises RawDelete.
			cell, cellOK := heapSchema.CellPresent(slot, payload, none, none)
			if !cellOK || !builder.Apply(selector, cell) {
				continue
			}
			object, objectOK := builder.Finish()
			world, worldOK := heapSchema.One(key, object)
			candidateFact, factOK := heapSchema.Relation(key, world)
			if !objectOK || !worldOK || !factOK {
				continue
			}
			visits := func() bool {
				count := 0
				walked := heapSchema.VisitRawAccessRoute(candidateRoute, candidateFact, selector, func(raw heapdomain.RawAccess) bool {
					if raw.Valid() {
						count++
					}
					return true
				})
				return walked && count != 0
			}
			if !visits() {
				continue
			}
			route, fact, visit = candidateRoute, candidateFact, visits
		}
	}
	if route == 0 || !fact.Valid() || visit == nil {
		t.Fatal("no real Heap RawRoute supports write slot")
	}

	view := rawSetView{keyCount: 0, heapCount: 1, packCount: 0, sourceCount: len(descriptor.descriptor.sources)}
	if descriptor.descriptor.kind == rawPayloadTail {
		t.Fatal("real mutation fixture expected fixed or NilFill payload")
	}
	if descriptor.descriptor.kind == rawPayloadFixed {
		tag, tagOK := descriptor.descriptor.byValue[descriptor.descriptor.fixed]
		atom, atomOK := valueSchema.OpaqueKind(runtimekind.Table)
		sourceFact, sourceOK := valueSchema.Singleton(atom)
		if !tagOK || !atomOK || !sourceOK {
			t.Fatal("real RawSet source fact")
		}
		view.source = func(want rawSourceTag) rawSelected[valuedomain.Value] {
			if want != tag {
				return rawSelected[valuedomain.Value]{valid: true}
			}
			return rawSelected[valuedomain.Value]{value: sourceFact, present: true, found: true, valid: true}
		}
	}
	return rawSetRealMutation{rule: rule, access: access, descriptor: descriptor.descriptor, view: view, route: route, fact: fact, visitRoute: visit}
}
