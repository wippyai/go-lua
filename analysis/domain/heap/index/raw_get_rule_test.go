package index

import (
	"context"
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestRawGetRuleAssembledScalarReceiverHasNoCandidate(t *testing.T) {
	heapSchema, valueSchema, callSchema, packSchema, topology, access, seed := rawGetScalarFixture(t)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(1), rawGetKey(2), valueSchema)
	calls, callsOK := callowner.Declare(composition, rawGetKey(3), callSchema)
	heap, heapOK := heapowner.Declare(composition, rawGetKey(4), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawGetKey(5), packSchema)
	source, sourceOK := valuesource.Declare(composition, rawGetKey(6), rawGetKey(7), rawGetKey(8), values)
	rawGet, rawGetOK := DeclareRawGet(composition, rawGetKey(9), rawGetKey(10), rawGetKey(11), topology, values, calls, heap, packs)
	if !valuesOK || !callsOK || !heapOK || !packsOK || !sourceOK || !rawGetOK {
		t.Fatal("raw-get declarations")
	}

	var read engine.QueryRead[engine.OrderedCells[valuedomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: rawGetKey(12),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, ok := engine.QueryValue(row, read)
				if !ok || cells.Count() != 1 {
					return false
				}
				_, present, available := cells.At(0)
				return rows == 1 && available && !present
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: rawGetKey(13),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var ok bool
		read, ok = engine.QueryReadFrom(query, values.ExactRead())
		return ok
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("raw-get query/seal")
	}

	resultCoordinate, resultOK := access.Result()
	resultRef, resultRefOK := values.Locate(resultCoordinate)
	sourceInstance, sourceInstanceOK := source.Instance(seed)
	rawGetInstance, rawGetInstanceOK := rawGet.Instance(access)
	if !resultOK || !resultRefOK || !sourceInstanceOK || !rawGetInstanceOK {
		t.Fatal("raw-get instances")
	}
	result := testlaw.RunNInputs(context.Background(), testlaw.NInputFixture[
		valuedomain.Value, valuedomain.SourceSeed,
		valuedomain.Value, Access,
		bool,
	]{
		Composition: composition,
		Predecessor: sourceInstance,
		Target:      rawGetInstance,
		Query:       query,
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, resultRef)
		},
		PredecessorSite:       rawGetKey(14),
		PredecessorOccurrence: rawGetKey(15),
		TargetSite:            rawGetKey(16),
		TargetOccurrence:      rawGetKey(17),
		BoundarySemantics: []engine.SemanticKey{
			rawGetKey(18), rawGetKey(19), rawGetKey(20), rawGetKey(21),
		},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("raw-get scalar result = status:%v available:%t value:%t", result.Status, result.ValueAvailable, result.Value)
	}
}

func TestRawGetReductionReadsExactStoredFieldPayload(t *testing.T) {
	_, heapSchema, valueSchema, callSchema, packSchema, topology, access, key := rawGetFieldFixture(t)
	field, fieldOK := heapSchema.FieldAt(key, 0)
	slot, slotOK := heapSchema.SlotForField(field)
	payload, payloadOK := heapSchema.PayloadForField(field)
	selector, selectorOK := heapSchema.SelectorForSlot(slot)
	none, noneOK := heapSchema.ContainmentNone()
	cell, cellOK := heapSchema.CellPresent(slot, payload, none, none)
	object, objectOK := heapSchema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if objectOK {
		objectOK = object.Apply(selector, cell)
	}
	sealedObject, sealedObjectOK := object.Finish()
	world, worldOK := heapSchema.One(key, sealedObject)
	heapFact, factOK := heapSchema.Relation(key, world)
	receiverAtom, atomOK := valueSchema.Allocation(key, materialization.Recent)
	receiver, receiverOK := valueSchema.Singleton(receiverAtom)
	routeTag, routeOK := heapSchema.RouteTag(key, materialization.Recent)
	if !fieldOK || !slotOK || !payloadOK || !selectorOK || !noneOK || !cellOK || !objectOK ||
		!sealedObjectOK || !worldOK || !factOK || !atomOK || !receiverOK || !routeOK {
		t.Fatal("raw-get field relation")
	}

	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(30), rawGetKey(31), valueSchema)
	calls, callsOK := callowner.Declare(composition, rawGetKey(32), callSchema)
	heap, heapOK := heapowner.Declare(composition, rawGetKey(33), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawGetKey(34), packSchema)
	rule, ruleOK := DeclareRawGet(composition, rawGetKey(35), rawGetKey(36), rawGetKey(37), topology, values, calls, heap, packs)
	if !valuesOK || !callsOK || !heapOK || !packsOK || !ruleOK || len(rule.sources) != 1 {
		t.Fatal("raw-get field declaration")
	}
	var sourceFact valuedomain.Value
	for index := 0; index < valueSchema.CoordinateCount(); index++ {
		seed, ok := valueSchema.SourceSeedAt(index)
		coordinate, value, resultOK := seed.Result()
		if ok && resultOK && coordinate == rule.sources[0].coordinate {
			sourceFact = value
			break
		}
	}
	if valueSchema.Equal(sourceFact, valueSchema.Bottom()) {
		t.Fatal("raw-get field source fact")
	}
	scratch := rule.takeScratch()
	defer rule.putScratch(scratch)
	result, present, valid := rule.reduce(access, receiver, rawGetView{
		scratch:   scratch,
		keyCount:  0,
		callCount: 0,
		call: func(uint64) rawSelected[calldomain.Value] {
			return rawSelected[calldomain.Value]{valid: true}
		},
		heapCount: 1,
		heap: func(tag heapdomain.RawRouteTag, got heapdomain.Key) rawSelected[heapdomain.Value] {
			return rawSelected[heapdomain.Value]{value: heapFact, present: true, found: tag == routeTag && got == key, valid: true}
		},
		packCount: 0,
		pack: func(heapdomain.RawPayloadTag) rawSelected[pack.Value] {
			return rawSelected[pack.Value]{valid: true}
		},
		sourceCount: 1,
		source: func(tag rawSourceTag) rawSelected[valuedomain.Value] {
			return rawSelected[valuedomain.Value]{value: sourceFact, present: true, found: tag == 1, valid: true}
		},
	})
	if !valid || !present || !valueSchema.Equal(result, sourceFact) {
		t.Fatal("raw-get exact field payload")
	}

	empty, emptyOK := heapSchema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	emptyObject, emptyObjectOK := empty.Finish()
	emptyWorld, emptyWorldOK := heapSchema.One(key, emptyObject)
	emptyFact, emptyFactOK := heapSchema.Relation(key, emptyWorld)
	if !emptyOK || !emptyObjectOK || !emptyWorldOK || !emptyFactOK {
		t.Fatal("raw-get absent field relation")
	}
	result, present, valid = rule.reduce(access, receiver, rawGetView{
		scratch:   scratch,
		keyCount:  0,
		callCount: 0,
		call: func(uint64) rawSelected[calldomain.Value] {
			return rawSelected[calldomain.Value]{valid: true}
		},
		heapCount: 1,
		heap: func(tag heapdomain.RawRouteTag, got heapdomain.Key) rawSelected[heapdomain.Value] {
			return rawSelected[heapdomain.Value]{value: emptyFact, present: true, found: tag == routeTag && got == key, valid: true}
		},
		packCount: 0,
		pack: func(heapdomain.RawPayloadTag) rawSelected[pack.Value] {
			return rawSelected[pack.Value]{valid: true}
		},
		sourceCount: 0,
		source: func(rawSourceTag) rawSelected[valuedomain.Value] {
			return rawSelected[valuedomain.Value]{valid: true}
		},
	})
	if !valid || present || !valueSchema.Equal(result, valueSchema.Bottom()) {
		t.Fatal("IndexGetRaw staged a result for raw absence instead of continuing candidate routing")
	}

	presentTop, presentTopOK := valueSchema.FilterPresent(valueSchema.Top())
	result, present, valid = rule.reduce(access, receiver, rawGetView{
		scratch:   scratch,
		keyCount:  0,
		callCount: 0,
		call: func(uint64) rawSelected[calldomain.Value] {
			return rawSelected[calldomain.Value]{valid: true}
		},
		heapCount: 1,
		heap: func(tag heapdomain.RawRouteTag, got heapdomain.Key) rawSelected[heapdomain.Value] {
			return rawSelected[heapdomain.Value]{value: heapSchema.Top(), present: true, found: tag == routeTag && got == key, valid: true}
		},
		packCount: 0,
		pack: func(heapdomain.RawPayloadTag) rawSelected[pack.Value] {
			return rawSelected[pack.Value]{valid: true}
		},
		sourceCount: 0,
		source: func(rawSourceTag) rawSelected[valuedomain.Value] {
			return rawSelected[valuedomain.Value]{valid: true}
		},
	})
	if !presentTopOK || !valid || !present || !valueSchema.Equal(result, presentTop) || valueSchema.RuntimeKinds(result).Contains(runtimekind.Nil) {
		t.Fatal("IndexGetRaw Heap.Top retained raw-absent nil")
	}

	opaqueTable, opaqueOK := valueSchema.OpaqueKind(runtimekind.Table)
	unknownReceiver, unknownOK := valueSchema.Singleton(opaqueTable)
	result, present, valid = rule.reduce(access, unknownReceiver, rawGetView{
		scratch:   scratch,
		keyCount:  0,
		callCount: 0,
		call: func(uint64) rawSelected[calldomain.Value] {
			return rawSelected[calldomain.Value]{valid: true}
		},
		heapCount: 0,
		heap: func(heapdomain.RawRouteTag, heapdomain.Key) rawSelected[heapdomain.Value] {
			return rawSelected[heapdomain.Value]{valid: true}
		},
		packCount: 0,
		pack: func(heapdomain.RawPayloadTag) rawSelected[pack.Value] {
			return rawSelected[pack.Value]{valid: true}
		},
		sourceCount: 0,
		source: func(rawSourceTag) rawSelected[valuedomain.Value] {
			return rawSelected[valuedomain.Value]{valid: true}
		},
	})
	if !opaqueOK || !unknownOK || !valid || !present || !valueSchema.Equal(result, presentTop) || valueSchema.RuntimeKinds(result).Contains(runtimekind.Nil) {
		t.Fatal("IndexGetRaw unknown table route retained raw-absent nil")
	}
}

func rawGetFieldFixture(t testing.TB) (*link.Link, heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *pack.Schema, *Topology, Access, heapdomain.Key) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "raw_get_field.lua", Text: []byte(`return ({ field = 1 }).field`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, heapOK := heapdomain.Seal(linked)
	valueSchema, valuesOK := valuedomain.Seal(linked, heapSchema)
	callSchema, callsOK := calldomain.New(linked)
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := static.Seal(linked, types)
	packSchema, packsOK := pack.Seal(linked, statics)
	topology, topologyOK := Seal(heapSchema, valueSchema, callSchema)
	if !heapOK || !valuesOK || !callsOK || !typesOK || staticErr != nil || !packsOK || !topologyOK {
		t.Fatal("raw-get field schemas")
	}
	var root heapdomain.Key
	for index := 0; index < heapSchema.KeyCount(); index++ {
		candidate, ok := heapSchema.KeyAt(index)
		_, _, kind, originOK := candidate.ProgramAllocation()
		if ok && originOK && kind == heapdomain.AllocationTable && heapSchema.FieldCount(candidate) == 1 {
			root = candidate
			break
		}
	}
	var access Access
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, ok := heapSchema.IndexAccessAt(index)
		if got, found := topology.Access(candidate); ok && found && got.Read() {
			access = got
			break
		}
	}
	if !root.Valid() || access == (Access{}) {
		t.Fatal("raw-get field origins")
	}
	return linked, heapSchema, valueSchema, callSchema, packSchema, topology, access, root
}

func rawGetScalarFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *pack.Schema, *Topology, Access, valuedomain.SourceSeed) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "raw_get_scalar.lua", Text: []byte(`return (1).field`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, heapOK := heapdomain.Seal(linked)
	valueSchema, valuesOK := valuedomain.Seal(linked, heapSchema)
	callSchema, callsOK := calldomain.New(linked)
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := static.Seal(linked, types)
	packSchema, packsOK := pack.Seal(linked, statics)
	topology, topologyOK := Seal(heapSchema, valueSchema, callSchema)
	if !heapOK || !valuesOK || !callsOK || !typesOK || staticErr != nil || !packsOK || !topologyOK {
		t.Fatal("raw-get schemas")
	}
	var access Access
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, ok := heapSchema.IndexAccessAt(index)
		if !ok {
			t.Fatal("raw-get index access")
		}
		if got, found := topology.Access(candidate); found && got.Read() {
			access = got
			break
		}
	}
	receiver, receiverOK := access.Receiver()
	if !receiverOK {
		t.Fatal("raw-get receiver")
	}
	var seed valuedomain.SourceSeed
	for index := 0; index < valueSchema.CoordinateCount(); index++ {
		candidate, ok := valueSchema.SourceSeedAt(index)
		coordinate, _, resultOK := candidate.Result()
		if ok && resultOK && coordinate == receiver {
			seed = candidate
			break
		}
	}
	if _, _, ok := seed.Result(); !ok {
		t.Fatal("raw-get receiver seed")
	}
	return heapSchema, valueSchema, callSchema, packSchema, topology, access, seed
}

func rawGetKey(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[30], digest[31] = 0x72, value
	key, _ := engine.NewSemanticKey(digest, 1)
	return key
}
