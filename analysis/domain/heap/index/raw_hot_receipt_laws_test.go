package index_test

import (
	"context"
	"crypto/sha256"
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	indexdomain "github.com/wippyai/go-lua/analysis/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type rawHotQueryObservation struct {
	Rows        int
	Present     int
	Absent      int
	Top         int
	Bottom      int
	Table       int
	NonTable    int
	EqualSource int
	Unavailable int
}

type rawHotHeapObservation struct {
	Rows        int
	Present     int
	Absent      int
	Top         int
	Bottom      int
	EqualSeed   int
	Changed     int
	Unavailable int
}

type rawHotSeed struct {
	Capability engine.RuleSlotCapability
	Point      keyspace.ContentID
	Occurrence keyspace.ContentID
}

type rawHotValueSeed struct {
	rawHotSeed
	Implementation *valueowner.RuleImplementation[keyspace.ContentID]
	Coordinate     valuedomain.Coordinate
	Value          valuedomain.Value
}

type rawHotHeapSeed struct {
	rawHotSeed
	Implementation *heapowner.RuleImplementation[keyspace.ContentID]
	Key            heapdomain.Key
	Value          heapdomain.Value
}

func rawHotQuerySpec(values *valuedomain.Schema, expected valuedomain.Value) (engine.HotExactQuerySpec[valuedomain.Value, rawHotQueryObservation], bool) {
	if values == nil || !values.Equal(expected, expected) {
		return engine.HotExactQuerySpec[valuedomain.Value, rawHotQueryObservation]{}, false
	}
	tables, tablesOK := values.ForRuntimeKinds(runtimekind.Bit(runtimekind.Table))
	if !tablesOK {
		return engine.HotExactQuerySpec[valuedomain.Value, rawHotQueryObservation]{}, false
	}
	return engine.HotExactQuerySpec[valuedomain.Value, rawHotQueryObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[valuedomain.Value], rawHotQueryObservation]{
			Begin: func() rawHotQueryObservation { return rawHotQueryObservation{} },
			Accumulate: func(result rawHotQueryObservation, cells engine.OrderedCells[valuedomain.Value]) (rawHotQueryObservation, bool) {
				result.Rows++
				for index := 0; index < cells.Count(); index++ {
					value, present, available := cells.At(index)
					if !available {
						result.Unavailable++
						continue
					}
					if !present {
						result.Absent++
						continue
					}
					result.Present++
					if value.IsTop() {
						result.Top++
					} else if value.IsBottom() {
						result.Bottom++
					} else if values.LessOrEq(value, tables) {
						result.Table++
					} else {
						result.NonTable++
					}
					if values.Equal(value, expected) {
						result.EqualSource++
					}
				}
				return result, true
			},
		},
		Result: engine.FrozenResult[rawHotQueryObservation]{
			Semantic: rawHotSemantic(13),
			Freeze:   func(value rawHotQueryObservation) rawHotQueryObservation { return value },
			Clone:    func(value rawHotQueryObservation) rawHotQueryObservation { return value },
			Equal:    func(left, right rawHotQueryObservation) bool { return left == right },
			Fingerprint: func(value rawHotQueryObservation) uint64 {
				return uint64(value.Rows)<<56 | uint64(value.Present)<<44 | uint64(value.Absent)<<32 | uint64(value.Top)<<24 | uint64(value.Bottom)<<16 | uint64(value.Table)<<8 | uint64(value.NonTable)<<4 | uint64(value.EqualSource)
			},
		},
	}, true
}

func rawHotHeapQuerySpec(heap heapdomain.Schema, seed heapdomain.Value) (engine.HotExactQuerySpec[heapdomain.Value, rawHotHeapObservation], bool) {
	if !heap.Valid() || !seed.Valid() {
		return engine.HotExactQuerySpec[heapdomain.Value, rawHotHeapObservation]{}, false
	}
	return engine.HotExactQuerySpec[heapdomain.Value, rawHotHeapObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[heapdomain.Value], rawHotHeapObservation]{
			Begin: func() rawHotHeapObservation { return rawHotHeapObservation{} },
			Accumulate: func(result rawHotHeapObservation, cells engine.OrderedCells[heapdomain.Value]) (rawHotHeapObservation, bool) {
				result.Rows++
				for index := 0; index < cells.Count(); index++ {
					value, present, available := cells.At(index)
					if !available {
						result.Unavailable++
						continue
					}
					if !present {
						result.Absent++
						continue
					}
					result.Present++
					if value.IsTop() {
						result.Top++
					} else if value.IsBottom() {
						result.Bottom++
					} else if heapdomain.Equal(value, seed) {
						result.EqualSeed++
					} else {
						result.Changed++
					}
				}
				return result, true
			},
		},
		Result: engine.FrozenResult[rawHotHeapObservation]{
			Semantic: rawHotSemantic(15),
			Freeze:   func(value rawHotHeapObservation) rawHotHeapObservation { return value },
			Clone:    func(value rawHotHeapObservation) rawHotHeapObservation { return value },
			Equal:    func(left, right rawHotHeapObservation) bool { return left == right },
			Fingerprint: func(value rawHotHeapObservation) uint64 {
				return uint64(value.Rows)<<56 | uint64(value.Present)<<46 | uint64(value.Absent)<<36 | uint64(value.Top)<<28 | uint64(value.Bottom)<<20 | uint64(value.EqualSeed)<<10 | uint64(value.Changed)
			},
		},
	}, true
}

func TestRawHotMountedPathSolve(t *testing.T) {
	fixture := rawHotMountedFixture(t)
	if fixture.getOccurrence == (keyspace.ContentID{}) || fixture.setOccurrence == (keyspace.ContentID{}) {
		t.Fatal("missing raw occurrences")
	}
	if fixture.getRule == nil || fixture.setRule == nil {
		t.Fatal("missing hot rules")
	}
	if fixture.assembly == nil || fixture.binding == nil || fixture.queryImplementation == nil {
		t.Fatal("missing assembly")
	}
	if fixture.heapQueryImplementationA == nil || len(fixture.valueSeeds) != 4 || len(fixture.heapSeeds) != 2 {
		t.Fatal("missing owner-issued seeds")
	}
	for _, seed := range fixture.valueSeeds {
		if !attachRawHotValueSeed(fixture.assembly, fixture.values, fixture.mountID, seed) {
			t.Fatal("raw value seed")
		}
	}
	for _, seed := range fixture.heapSeeds {
		if !attachRawHotHeapSeed(fixture.assembly, fixture.heap, fixture.mountID, seed) {
			t.Fatal("raw heap seed")
		}
	}
	_, getOK := fixture.getRule.AttachMountedOccurrence(fixture.assembly, fixture.mountID, fixture.getPoint, fixture.getOccurrence)
	_, setOK := fixture.setRule.AttachMountedOccurrence(fixture.assembly, fixture.mountID, fixture.setPoint, fixture.setOccurrence)
	sourcesOK := getOK && setOK && fixture.assembly.SealSources()
	valueQueryOK := sourcesOK && engine.AddMountedExactQuery(fixture.assembly, fixture.queryImplementation, fixture.queryID, fixture.mountID, fixture.getPoint)
	heapQueryOK := valueQueryOK && engine.AddMountedExactQuery(fixture.assembly, fixture.heapQueryImplementationA, fixture.heapQueryIDA, fixture.mountID, fixture.setPoint)
	if !getOK || !setOK || !sourcesOK || !valueQueryOK || !heapQueryOK {
		failure, failureOK := fixture.assembly.SealFailure()
		sourceFailure, _ := failure.Source()
		artifactFailure, _ := failure.ArtifactRow()
		ruleFailure, _ := failure.RuleSource()
		finalizerFailure, _ := failure.Finalizer()
		t.Fatalf("raw mounted rows get=%t set=%t sources=%t valueQuery=%t heapQuery=%t sealFailure=%t phase=%v ordinal=%d sourceFailure=%v artifactFailure=%v ruleFailure=%v finalizerFailure=%v", getOK, setOK, sourcesOK, valueQueryOK, heapQueryOK, failureOK, failure.Phase(), failure.Ordinal(), sourceFailure, artifactFailure, ruleFailure, finalizerFailure)
	}
	topology, graph, committed := fixture.assembly.Commit()
	if !committed || topology == nil || graph == nil {
		failure, failureOK := fixture.assembly.CommitFailure()
		precondition, _ := failure.Precondition()
		topologyFailure, _ := failure.Topology()
		semantic, _ := failure.SemanticRows()
		publish, _ := failure.Publish()
		schedule, _ := failure.Schedule()
		t.Fatalf("raw mounted commit failure=%t phase=%v precondition=%v topology=%v semantic=%v publish=%v schedule=%v", failureOK, failure.Phase(), precondition, topologyFailure, semantic, publish, schedule)
	}
	compilation, compilationOK := engine.BeginReceiptTopologyCompilation(fixture.binding, graph)
	if !compilationOK || compilation == nil {
		t.Fatal("raw topology compilation")
	}
	if _, ok := fixture.getRule.AttachMountedReceiptMember(compilation, graph, fixture.mountID, fixture.getPoint, fixture.getOccurrence); !ok {
		t.Fatal("raw get member")
	}
	if _, ok := fixture.setRule.AttachMountedReceiptMember(compilation, graph, fixture.mountID, fixture.setPoint, fixture.setOccurrence); !ok {
		t.Fatal("raw set member")
	}
	for _, seed := range fixture.valueSeeds {
		if _, ok := attachRawHotValueSeedMember(compilation, graph, seed.Implementation, fixture.mountID, seed); !ok {
			t.Fatal("raw value seed member")
		}
	}
	for _, seed := range fixture.heapSeeds {
		if _, ok := attachRawHotHeapSeedMember(compilation, graph, seed.Implementation, fixture.mountID, seed); !ok {
			t.Fatal("raw heap seed member")
		}
	}
	query, queryOK := graph.Query(fixture.queryID)
	heapQueryA, heapQueryAOK := graph.Query(fixture.heapQueryIDA)
	if !queryOK || !heapQueryAOK || !engine.AttachReceiptExactQuery(compilation, fixture.queryImplementation, query) || !engine.AttachReceiptExactQuery(compilation, fixture.heapQueryImplementationA, heapQueryA) {
		t.Fatal("raw query member")
	}
	solver, solverOK := compilation.Solver()
	if !solverOK || solver == nil {
		t.Fatal("raw solver")
	}
	state, status, report := solver.SolveWithReport(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("raw solve status=%v state=%t reason=%v phase=%v point=%x group=%x member=%x rule=%x", status, state != nil, report.Reason(), report.Phase(), report.Point(), report.Group(), report.Member(), report.Rule())
	}
	observation, observationOK := engine.ReceiptQueryResult[rawHotQueryObservation](query, solver, state)
	// RawSet targets a only. The subsequent read of b.source must retain the
	// seeded table; absence here would prove an unsound cross-root fan-out.
	if !observationOK || observation.Rows == 0 || observation.Present == 0 || observation.Table != observation.Present || observation.EqualSource != observation.Present || observation.Absent != 0 || observation.Top != 0 || observation.Bottom != 0 || observation.NonTable != 0 || observation.Unavailable != 0 {
		t.Fatalf("raw query observation=%#v readable=%t", observation, observationOK)
	}
	observationA, observationAOK := engine.ReceiptQueryResult[rawHotHeapObservation](heapQueryA, solver, state)
	if !observationAOK || observationA.Rows == 0 || observationA.Unavailable != 0 || observationA.Top != 0 || observationA.Bottom != 0 || observationA.Changed == 0 || observationA.EqualSeed != 0 {
		t.Fatalf("raw set target observation=%#v readable=%t", observationA, observationAOK)
	}
}

func attachRawHotValueSeed(assembly *engine.ReceiptAssembly, owner *valueowner.HotOwner, mount keyspace.ContentID, seed rawHotValueSeed) bool {
	implementation, implementationOK := valueowner.ResolveRuleImplementation(seed.Implementation)
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(seed.Capability, mount, seed.Point, seed.Occurrence)
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, seed.Occurrence)
	ref, refOK := owner.Ref(seed.Coordinate)
	if !implementationOK || !occurrenceOK || !transactionOK || !refOK || !engine.AddExactWrite(transaction, ref) {
		return false
	}
	return assembly.QueueMountedRuleFinalizer(seed.Capability, func() bool {
		source, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		write, writeOK := implementation.ReceiptWritePart(source, 0)
		if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
}

func attachRawHotHeapSeed(assembly *engine.ReceiptAssembly, owner *heapowner.HotOwner, mount keyspace.ContentID, seed rawHotHeapSeed) bool {
	implementation, implementationOK := heapowner.ResolveRuleImplementation(seed.Implementation)
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(seed.Capability, mount, seed.Point, seed.Occurrence)
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, seed.Occurrence)
	ref, refOK := owner.Ref(seed.Key)
	if !implementationOK || !occurrenceOK || !transactionOK || !refOK || !engine.AddExactWrite(transaction, ref) {
		return false
	}
	return assembly.QueueMountedRuleFinalizer(seed.Capability, func() bool {
		source, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		write, writeOK := implementation.ReceiptWritePart(source, 0)
		if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
}

func attachRawHotValueSeedMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, implementation *valueowner.RuleImplementation[keyspace.ContentID], mount keyspace.ContentID, seed rawHotValueSeed) (*engine.ReceiptMember, bool) {
	member, memberOK := graph.MountedRuleMember(seed.Capability, mount, seed.Point, seed.Occurrence)
	resolved, resolvedOK := valueowner.ResolveRuleImplementation(implementation)
	if !memberOK || !resolvedOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, resolved, member, seed.Occurrence)
}

func attachRawHotHeapSeedMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, implementation *heapowner.RuleImplementation[keyspace.ContentID], mount keyspace.ContentID, seed rawHotHeapSeed) (*engine.ReceiptMember, bool) {
	member, memberOK := graph.MountedRuleMember(seed.Capability, mount, seed.Point, seed.Occurrence)
	resolved, resolvedOK := heapowner.ResolveRuleImplementation(implementation)
	if !memberOK || !resolvedOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, resolved, member, seed.Occurrence)
}

type rawHotMountedPlan struct {
	getID, getPoint  keyspace.ContentID
	setID, setPoint  keyspace.ContentID
	valueCoordinates []valuedomain.Coordinate
	valueFacts       []valuedomain.Value
	heapKeys         []heapdomain.Key
	heapFacts        []heapdomain.Value
}

func buildRawHotMountedPlan(t testing.TB, linked *link.Link, module keyspace.ContentID, artifact *programartifact.Artifact, heapMount heapdomain.ArtifactMount, heapSchema heapdomain.Schema, valueSchema *valuedomain.Schema, packs *packdomain.Schema) rawHotMountedPlan {
	t.Helper()
	var getID, setID keyspace.ContentID
	var getGeometry, setGeometry heapdomain.IndexGeometry
	var getAccess heapdomain.IndexAccess
	var nilSourceAccess, tableSourceAccess heapdomain.IndexAccess
	var nilSource, tableSource packdomain.SemanticSource
	var mountedCount, fixedCount, tailCount, nilCount, sourceFactCount, nilSourceCount, tableSourceCount int
	for index := 0; index < artifact.RuleOccurrenceCount(programartifact.RuleRoleRawGet); index++ {
		row, rowOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleRawGet, index)
		receipt, receiptOK := heapMount.IndexAccessReceipt(row.ID(), true)
		access, accessOK := heapSchema.IndexAccessForReceipt(receipt)
		geometry, geometryOK := heapSchema.IndexAccessGeometry(access)
		if rowOK && receiptOK && accessOK && geometryOK && !getID.Available() {
			getID, getAccess, getGeometry = row.ID(), access, geometry
		}
	}
	for index := 0; index < artifact.RuleOccurrenceCount(programartifact.RuleRoleRawSet); index++ {
		row, rowOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleRawSet, index)
		if !rowOK {
			continue
		}
		receipt, receiptOK := heapMount.IndexAccessReceipt(row.ID(), false)
		access, accessOK := heapSchema.IndexAccessForReceipt(receipt)
		geometry, geometryOK := heapSchema.IndexAccessGeometry(access)
		if !receiptOK || !accessOK || !geometryOK {
			continue
		}
		mounted, mountedOK := packs.PayloadForMounted(module, geometry.ValuesID, geometry.Position)
		if !mountedOK {
			continue
		}
		mountedCount++
		switch mounted.Kind() {
		case packdomain.MountedPayloadFixed:
			fixedCount++
		case packdomain.MountedPayloadTail:
			tailCount++
		case packdomain.MountedPayloadNil:
			nilCount++
		}
		candidate, sourceOK := mounted.Fixed()
		if mounted.Kind() != packdomain.MountedPayloadFixed || !sourceOK {
			continue
		}
		candidateModule := candidate.Module()
		if candidateModule != module {
			continue
		}
		boundaryValue, boundaryOK := linked.Boundary().Values().ForMountedSemantic(candidateModule, candidate.ID())
		boundaryID, boundaryIDOK := linked.Boundary().Values().ID(boundaryValue)
		if !boundaryOK || !boundaryIDOK {
			continue
		}
		_, sourceFact, sourceFactOK := rawHotScalarValue(boundaryID, valueSchema)
		if sourceFactOK {
			sourceFactCount++
			if valueSchema.RuntimeKinds(sourceFact) == runtimekind.Bit(runtimekind.Nil) {
				nilSourceCount++
				if !setID.Available() {
					setID, setGeometry, nilSourceAccess, nilSource = row.ID(), geometry, access, candidate
				}
			}
		}
		// Allocation-backed table sources deliberately have no scalar
		// SourceValueID fact. Recognize them through the exact Heap key and
		// Value AllocationResult issued for this semantic source.
		sourceKey, sourceKeyOK := heapKeyForRootValue(heapSchema, boundaryID)
		allocationResult, allocationResultOK := valueSchema.AllocationResultFor(sourceKey)
		allocationFact, allocationFactOK := allocationResult.Fresh()
		if sourceKeyOK && allocationResultOK && allocationFactOK && valueSchema.RuntimeKinds(allocationFact) == runtimekind.Bit(runtimekind.Table) {
			tableSourceCount++
			if !tableSource.Available() {
				tableSourceAccess, tableSource = access, candidate
			}
		}
	}
	if !setID.Available() || nilSourceAccess == (heapdomain.IndexAccess{}) || !nilSource.Available() || tableSourceAccess == (heapdomain.IndexAccess{}) || !tableSource.Available() || nilSourceCount != 1 || tableSourceCount != 1 {
		t.Fatalf("raw mounted nil/fixed write plan writes=%d mounted=%d fixed=%d tail=%d nil=%d facts=%d nilSources=%d tableSources=%d", artifact.RuleOccurrenceCount(programartifact.RuleRoleRawSet), mountedCount, fixedCount, tailCount, nilCount, sourceFactCount, nilSourceCount, tableSourceCount)
	}
	if getID == (keyspace.ContentID{}) {
		t.Fatal("raw mounted read occurrence")
	}
	if getGeometry.BaseValueID == setGeometry.BaseValueID {
		for index := 0; index < artifact.RuleOccurrenceCount(programartifact.RuleRoleRawGet); index++ {
			row, rowOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleRawGet, index)
			if !rowOK {
				continue
			}
			receipt, receiptOK := heapMount.IndexAccessReceipt(row.ID(), true)
			access, accessOK := heapSchema.IndexAccessForReceipt(receipt)
			geometry, geometryOK := heapSchema.IndexAccessGeometry(access)
			_, pointOK := row.PointAt(0)
			if receiptOK && accessOK && geometryOK && pointOK && geometry.BaseValueID != setGeometry.BaseValueID {
				getID, getAccess, getGeometry = row.ID(), access, geometry
				break
			}
		}
	}
	if getGeometry.BaseValueID == setGeometry.BaseValueID {
		t.Fatal("raw mounted read did not target foreign root")
	}
	var getPoint, setPoint keyspace.ContentID
	var getPointOK, setPointOK bool
	for index := 0; index < artifact.RuleOccurrenceCount(programartifact.RuleRoleRawGet); index++ {
		candidate, candidateOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleRawGet, index)
		if candidateOK && candidate.ID() == getID {
			getPoint, getPointOK = candidate.PointAt(0)
			break
		}
	}
	for index := 0; index < artifact.RuleOccurrenceCount(programartifact.RuleRoleRawSet); index++ {
		candidate, candidateOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleRawSet, index)
		if candidateOK && candidate.ID() == setID {
			setPoint, setPointOK = candidate.PointAt(0)
			break
		}
	}
	if !getPointOK || !setPointOK {
		t.Fatal("raw mounted occurrence lookup")
	}
	tableBoundaryValue, tableBoundaryOK := linked.Boundary().Values().ForMountedSemantic(module, tableSource.ID())
	tableBoundaryID, tableBoundaryIDOK := linked.Boundary().Values().ID(tableBoundaryValue)
	nilBoundaryValue, nilBoundaryOK := linked.Boundary().Values().ForMountedSemantic(module, nilSource.ID())
	nilBoundaryID, nilBoundaryIDOK := linked.Boundary().Values().ID(nilBoundaryValue)
	sourceKey, sourceKeyOK := heapKeyForRootValue(heapSchema, tableBoundaryID)
	outerKeys, outerKeysOK := rawHotOuterTableKeys(heapSchema, sourceKey)
	if !tableBoundaryOK || !tableBoundaryIDOK || !nilBoundaryOK || !nilBoundaryIDOK || !sourceKeyOK || !outerKeysOK {
		t.Fatalf("raw mounted root identities tableBoundary=%t tableBoundaryID=%t nilBoundary=%t nilBoundaryID=%t source=%t outers=%t", tableBoundaryOK, tableBoundaryIDOK, nilBoundaryOK, nilBoundaryIDOK, sourceKeyOK, outerKeysOK)
	}
	baseA, baseB := outerKeys[0], outerKeys[1]
	aSlot, aSlotOK := heapSchema.SlotForIndexAccess(nilSourceAccess)
	tableSlot, tableSlotOK := heapSchema.SlotForIndexAccess(tableSourceAccess)
	tablePayload, tablePayloadOK := heapSchema.PayloadForIndexAccess(tableSourceAccess)
	if !aSlotOK || !tableSlotOK || !tablePayloadOK {
		t.Fatal("raw mounted seed payload")
	}
	none, noneOK := heapSchema.ContainmentNone()
	reference, referenceOK := heapSchema.Reference(sourceKey, materialization.Recent)
	valueContainment, valueContainmentOK := heapSchema.ContainmentExact(reference)
	if !noneOK || !referenceOK || !valueContainmentOK {
		t.Fatal("raw mounted source containment")
	}
	makeHeapFact := func(key heapdomain.Key, slot heapdomain.Slot, present bool) heapdomain.Value {
		initializer, initializerOK := heapSchema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
		cell, cellOK := heapSchema.CellAbsent()
		if present {
			cell, cellOK = heapSchema.CellPresent(slot, tablePayload, valueContainment, none)
		}
		selector, selectorOK := heapSchema.SelectorForSlot(slot)
		if !initializerOK || !cellOK || !selectorOK || !initializer.Apply(selector, cell) {
			t.Fatal("raw mounted seed object")
		}
		object, objectOK := initializer.Finish()
		seed, seedOK := heapSchema.EmptyObject(key)
		fact, factOK := heapSchema.Create(seed, key, object)
		if !objectOK || !seedOK || !factOK {
			t.Fatal("raw mounted seed heap fact")
		}
		return fact
	}
	baseACoordinate, baseACoordinateOK := valueSchema.CoordinateForID(setGeometry.BaseValueID)
	baseBCoordinate, baseBCoordinateOK := valueSchema.CoordinateForID(getGeometry.BaseValueID)
	baseAResult, baseAResultOK := valueSchema.AllocationResultFor(baseA)
	baseAFact, baseAFactOK := baseAResult.Fresh()
	baseBResult, baseBResultOK := valueSchema.AllocationResultFor(baseB)
	baseBFact, baseBFactOK := baseBResult.Fresh()
	sourceResult, sourceResultOK := valueSchema.AllocationResultFor(sourceKey)
	sourceCoordinate, sourceCoordinateOK := sourceResult.Coordinate()
	sourceFact, sourceValueOK := sourceResult.Fresh()
	nilCoordinate, nilFact, nilValueOK := rawHotScalarValue(nilBoundaryID, valueSchema)
	tableCoordinate, tableCoordinateOK := valueSchema.CoordinateForID(tableBoundaryID)
	if !baseACoordinateOK || !baseBCoordinateOK || !baseAResultOK || !baseAFactOK || !baseBResultOK || !baseBFactOK || !sourceResultOK || !sourceCoordinateOK || !sourceValueOK || !tableCoordinateOK || sourceCoordinate != tableCoordinate || valueSchema.RuntimeKinds(baseAFact) != runtimekind.Bit(runtimekind.Table) || valueSchema.RuntimeKinds(baseBFact) != runtimekind.Bit(runtimekind.Table) || valueSchema.RuntimeKinds(sourceFact) != runtimekind.Bit(runtimekind.Table) || !nilValueOK {
		t.Fatal("raw mounted value seed facts")
	}
	resultID, resultIDOK := heapSchema.IndexAccessResultID(getAccess)
	resultCoordinate, resultValueOK := valueSchema.CoordinateForID(resultID)
	_, resultIndexOK := valueSchema.CoordinateIndex(resultCoordinate)
	_, targetIndexOK := heapSchema.KeyIndex(baseA)
	if !resultIDOK || !resultValueOK || !resultIndexOK || !targetIndexOK {
		t.Fatal("raw mounted result coordinate")
	}
	return rawHotMountedPlan{
		getID: getID, getPoint: getPoint, setID: setID, setPoint: setPoint,
		valueCoordinates: []valuedomain.Coordinate{baseACoordinate, baseBCoordinate, sourceCoordinate, nilCoordinate},
		valueFacts:       []valuedomain.Value{baseAFact, baseBFact, sourceFact, nilFact},
		heapKeys:         []heapdomain.Key{baseA, baseB}, heapFacts: []heapdomain.Value{makeHeapFact(baseA, aSlot, true), makeHeapFact(baseB, tableSlot, true)},
	}
}

func rawHotScalarValue(id keyspace.ContentID, schema *valuedomain.Schema) (valuedomain.Coordinate, valuedomain.Value, bool) {
	if schema == nil {
		return valuedomain.Coordinate{}, valuedomain.Value{}, false
	}
	coordinate, coordinateOK := schema.CoordinateForID(id)
	fact, factOK := schema.SourceValueID(id)
	return coordinate, fact, coordinateOK && factOK
}

func heapKeyForRootValue(schema heapdomain.Schema, rootValueID keyspace.ContentID) (heapdomain.Key, bool) {
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		candidate, candidateOK := schema.AllocationRootValueID(key)
		if keyOK && candidateOK && candidate == rootValueID {
			return key, true
		}
	}
	return heapdomain.Key{}, false
}

func rawHotOuterTableKeys(schema heapdomain.Schema, source heapdomain.Key) ([2]heapdomain.Key, bool) {
	// The receipt fixture supplies the receiver flow facts explicitly. It must
	// not infer a Heap allocation identity from an SSA/Boundary receiver ID.
	// This program has exactly the RHS table plus two independent outer tables;
	// require that closed denominator instead of reconstructing provenance.
	var result [2]heapdomain.Key
	count := 0
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		receipt, receiptOK := key.AllocationReceipt()
		if !keyOK || !receiptOK || receipt.Kind() != heapdomain.AllocationTable || key == source {
			continue
		}
		if count == len(result) {
			return [2]heapdomain.Key{}, false
		}
		result[count] = key
		count++
	}
	return result, count == len(result)
}

func mustIndexOccurrenceID(t testing.TB, schema heapdomain.Schema, access heapdomain.IndexAccess) keyspace.ContentID {
	t.Helper()
	module, occurrence, _, ok := schema.IndexAccessOccurrence(access)
	if !ok || !module.Available() || !occurrence.Available() {
		t.Fatal("raw mounted occurrence identity")
	}
	return occurrence
}

type rawHotFixture struct {
	mountID                  keyspace.ContentID
	getPoint                 keyspace.ContentID
	getOccurrence            keyspace.ContentID
	setPoint                 keyspace.ContentID
	setOccurrence            keyspace.ContentID
	getRule                  *indexdomain.RawGetHotRule
	setRule                  *indexdomain.RawSetHotRule
	assembly                 *engine.ReceiptAssembly
	binding                  *engine.SchemaBinding
	values                   *valueowner.HotOwner
	heap                     *heapowner.HotOwner
	valueSeeds               []rawHotValueSeed
	heapSeeds                []rawHotHeapSeed
	queryID                  keyspace.ContentID
	queryImplementation      *engine.ExactQueryImplementation[valuedomain.Value, rawHotQueryObservation]
	heapQueryIDA             keyspace.ContentID
	heapQueryImplementationA *engine.ExactQueryImplementation[heapdomain.Value, rawHotHeapObservation]
}

func rawHotMountedFixture(t testing.TB) rawHotFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "raw_hot_mounted.lua", Text: []byte(`local a = {}
local b = {}
b.source = {}
a.source = nil
return b.source`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := programschema.Global()
	if !receiptOK {
		t.Fatal("program schema")
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mount identity")
	}
	artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), receipt)
	if failure.Available() || artifact == nil {
		t.Fatalf("artifact: %v", failure)
	}
	heapMount, heapMountOK := heapdomain.NewArtifactMount(artifact, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(artifact, module, programID)
	callMount := calldomain.MountedArtifact{ModuleKey: module, Artifact: artifact}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	valueSchema, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount})
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{callMount})
	packMount, packMountOK := packdomain.NewArtifactMount(artifact, module, programID)
	staticMount := staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
	types, typeErr := typeauthority.SealArtifactRows(linked.ContentID(), []*programartifact.Artifact{artifact})
	statics, _, staticErr := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedArtifact{staticMount})
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []packdomain.ArtifactMount{packMount})
	if !heapMountOK || !valueMountOK || !packMountOK || heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || !callsOK || typeErr != nil || types == nil || staticErr != nil || statics == nil || !packsOK {
		t.Fatalf("domain schemas heapMount=%t valueMount=%t packMount=%t heapFailure=%v valueFailure=%v calls=%t typeErr=%v types=%t staticErr=%v statics=%t packs=%t", heapMountOK, valueMountOK, packMountOK, heapFailure, valueFailure, callsOK, typeErr, types != nil, staticErr, statics != nil, packsOK)
	}
	topology, topologyOK := indexdomain.Seal(heapSchema, valueSchema, calls, packs)
	if !topologyOK || topology == nil {
		_, diagnostic := indexdomain.SealWithFailure(heapSchema, valueSchema, calls, packs)
		t.Fatalf("index topology: %s", diagnostic)
	}
	plan := buildRawHotMountedPlan(t, linked, module, artifact, heapMount, heapSchema, valueSchema, packs)
	leftSeen, rightSeen := false, false
	if !topology.VisitReceiver(plan.valueFacts[0], nil, func(route indexdomain.Route) bool {
		key, _, rooted := route.Root()
		if rooted {
			switch key {
			case plan.heapKeys[0]:
				leftSeen = true
			case plan.heapKeys[1]:
				rightSeen = true
			}
		}
		return true
	}) || !leftSeen || rightSeen {
		t.Fatal("raw mounted receiver selected the wrong root")
	}

	builder := engine.NewSchema()
	valueFragment, valueFragmentOK := valueowner.DeclareSchema(builder, rawHotKey(1), rawHotKey(2))
	callFragment, callFragmentOK := callowner.DeclareSchema(builder, rawHotKey(3))
	heapFragment, heapFragmentOK := heapowner.DeclareSchema(builder, rawHotKey(4))
	packFragment, packFragmentOK := packowner.DeclareSchema(builder, rawHotKey(5))
	getFragment, getFragmentOK := indexdomain.DeclareRawGetSchema(builder, rawHotKey(6), rawHotKey(7), rawHotKey(8), valueFragment, callFragment, heapFragment, packFragment)
	setFragment, setFragmentOK := indexdomain.DeclareRawSetSchema(builder, rawHotKey(9), rawHotKey(10), rawHotKey(11), valueFragment, heapFragment, packFragment)
	valueSeedRules := make([]*engine.RuleSlot[valuedomain.Value, keyspace.ContentID], len(plan.valueCoordinates))
	valueSeedWrites := make([]engine.SchemaWriteSlot[valuedomain.Value], len(plan.valueCoordinates))
	valueSeedOK := true
	for index := range valueSeedRules {
		seedRule, seedRuleOK := engine.DeclareRuleSlot[valuedomain.Value, keyspace.ContentID](builder, engine.SchemaRuleSpec[valuedomain.Value]{Semantic: rawHotKey(byte(20 + index*3)), OperandFamily: rawHotKey(byte(21 + index*3)), Inputs: 0, Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisTrustedTheorem, Identity: rawHotKey(byte(22 + index*3))}, Output: valueFragment.Ref()})
		seedWrite, seedWriteOK := engine.SchemaWrite(seedRule, valueFragment.ExactWrite())
		valueSeedRules[index], valueSeedWrites[index], valueSeedOK = seedRule, seedWrite, valueSeedOK && seedRuleOK && seedWriteOK
	}
	heapSeedRules := make([]*engine.RuleSlot[heapdomain.Value, keyspace.ContentID], len(plan.heapKeys))
	heapSeedWrites := make([]engine.SchemaWriteSlot[heapdomain.Value], len(plan.heapKeys))
	heapSeedOK := true
	for index := range heapSeedRules {
		seedRule, seedRuleOK := engine.DeclareRuleSlot[heapdomain.Value, keyspace.ContentID](builder, engine.SchemaRuleSpec[heapdomain.Value]{Semantic: rawHotKey(byte(40 + index*3)), OperandFamily: rawHotKey(byte(41 + index*3)), Inputs: 0, Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisTrustedTheorem, Identity: rawHotKey(byte(42 + index*3))}, Output: heapFragment.Ref()})
		seedWrite, seedWriteOK := engine.SchemaWrite(seedRule, heapFragment.ExactWrite())
		heapSeedRules[index], heapSeedWrites[index], heapSeedOK = seedRule, seedWrite, heapSeedOK && seedRuleOK && seedWriteOK
	}
	valueQuery, valueQueryOK := engine.DeclareQuerySlot[rawHotQueryObservation](builder, engine.SchemaQuerySpec{Semantic: rawHotKey(12), Freezer: rawHotKey(13)})
	valueQueryReadOK := valueQuery != nil && engine.SchemaQueryRead(valueQuery, valueFragment.ExactRead())
	heapQueryA, heapQueryAOK := engine.DeclareQuerySlot[rawHotHeapObservation](builder, engine.SchemaQuerySpec{Semantic: rawHotKey(14), Freezer: rawHotKey(15)})
	heapQueryAReadOK := heapQueryA != nil && engine.SchemaQueryRead(heapQueryA, heapFragment.ExactRead())
	if !valueFragmentOK || !callFragmentOK || !heapFragmentOK || !packFragmentOK || !getFragmentOK || !setFragmentOK || !valueSeedOK || !heapSeedOK || !valueQueryOK || !valueQueryReadOK || !heapQueryAOK || !heapQueryAReadOK {
		t.Fatal("raw hot schema declarations")
	}
	cold, coldOK := builder.Seal()
	binding := engine.NewSchemaBinding(cold)
	values, valuesOK := valueowner.BindHot(binding, valueFragment, valueSchema)
	callsHot, callsHotOK := callowner.BindHot(binding, callFragment, calls)
	heap, heapOK := heapowner.BindHot(binding, heapFragment, heapSchema)
	packsHot, packsHotOK := packowner.BindHot(binding, packFragment, packs)
	getRule, getRuleOK := indexdomain.BindRawGetHot(binding, getFragment, topology, values, callsHot, heap, packsHot)
	setRule, setRuleOK := indexdomain.BindRawSetHot(binding, setFragment, topology, values, heap, packsHot)
	valueSeedImplementations := make([]*valueowner.RuleImplementation[keyspace.ContentID], len(valueSeedRules))
	valueSeedBindOK := true
	for index, seedRule := range valueSeedRules {
		fact := plan.valueFacts[index]
		implementation, implementationOK := valueowner.BindExactWriteRule(values, seedRule, valueSeedWrites[index], engine.HotRuleSpec[valuedomain.Value, keyspace.ContentID]{
			OperandContent: func(value keyspace.ContentID) (keyspace.ContentID, [32]byte, bool) {
				return value, sha256.Sum256(value[:]), value.Available()
			},
			Admission: engine.AdmitRuleByTrustedTheorem[valuedomain.Value, keyspace.ContentID](rawHotKey(byte(22 + index*3))),
			Transfer: func(access engine.Access[valuedomain.Value, keyspace.ContentID]) bool {
				return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, fact) })
			},
		})
		valueSeedImplementations[index], valueSeedBindOK = implementation, valueSeedBindOK && implementationOK
	}
	heapSeedImplementations := make([]*heapowner.RuleImplementation[keyspace.ContentID], len(heapSeedRules))
	heapSeedBindOK := true
	for index, seedRule := range heapSeedRules {
		fact := plan.heapFacts[index]
		implementation, implementationOK := heapowner.BindExactWriteRule(heap, seedRule, heapSeedWrites[index], engine.HotRuleSpec[heapdomain.Value, keyspace.ContentID]{
			OperandContent: func(value keyspace.ContentID) (keyspace.ContentID, [32]byte, bool) {
				return value, sha256.Sum256(value[:]), value.Available()
			},
			Admission: engine.AdmitRuleByTrustedTheorem[heapdomain.Value, keyspace.ContentID](rawHotKey(byte(42 + index*3))),
			Transfer: func(access engine.Access[heapdomain.Value, keyspace.ContentID]) bool {
				return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, fact) })
			},
		})
		heapSeedImplementations[index], heapSeedBindOK = implementation, heapSeedBindOK && implementationOK
	}
	querySpec, querySpecOK := rawHotQuerySpec(valueSchema, plan.valueFacts[2])
	queryBindOK := querySpecOK && valueowner.BindExactQuery(values, valueQuery, querySpec)
	heapQuerySpec, heapQuerySpecOK := rawHotHeapQuerySpec(heapSchema, plan.heapFacts[0])
	heapQueryBindOK := heapQuerySpecOK && heapowner.BindExactQuery(heap, heapQueryA, heapQuerySpec)
	if !coldOK || cold == nil || !valuesOK || !callsHotOK || !heapOK || !packsHotOK || !getRuleOK || !setRuleOK || !valueSeedBindOK || !heapSeedBindOK || !queryBindOK || !heapQueryBindOK {
		t.Fatalf("raw hot bind cold=%t values=%t calls=%t heap=%t packs=%t get=%t set=%t valueSeeds=%t heapSeeds=%t valueQuerySpec=%t valueQuery=%t heapQuerySpec=%t heapQuery=%t", coldOK && cold != nil, valuesOK, callsHotOK, heapOK, packsHotOK, getRuleOK, setRuleOK, valueSeedBindOK, heapSeedBindOK, querySpecOK, queryBindOK, heapQuerySpecOK, heapQueryBindOK)
	}
	getCapability, getCapabilityOK := engine.IssueMountedRuleCapability(binding, getFragment.RuleSlot())
	setCapability, setCapabilityOK := engine.IssueMountedRuleCapability(binding, setFragment.RuleSlot())
	valueSeedCapabilities := make([]engine.RuleSlotCapability, len(valueSeedRules))
	heapSeedCapabilities := make([]engine.RuleSlotCapability, len(heapSeedRules))
	seedCapabilityOK := true
	for index, seedRule := range valueSeedRules {
		capability, capabilityOK := engine.IssueMountedRuleCapability(binding, seedRule)
		valueSeedCapabilities[index], seedCapabilityOK = capability, seedCapabilityOK && capabilityOK && engine.RegisterRuleSlot(binding, seedRule, capability)
	}
	for index, seedRule := range heapSeedRules {
		capability, capabilityOK := engine.IssueMountedRuleCapability(binding, seedRule)
		heapSeedCapabilities[index], seedCapabilityOK = capability, seedCapabilityOK && capabilityOK && engine.RegisterRuleSlot(binding, seedRule, capability)
	}
	if !getCapabilityOK || !setCapabilityOK || !engine.RegisterRuleSlot(binding, getFragment.RuleSlot(), getCapability) || !engine.RegisterRuleSlot(binding, setFragment.RuleSlot(), setCapability) || !seedCapabilityOK || !binding.Seal() {
		t.Fatal("raw hot binding seal")
	}
	queryImplementation, queryImplementationOK := engine.ExactQueryImplementationAt[valuedomain.Value, rawHotQueryObservation](binding, valueQuery)
	heapQueryImplementationA, heapQueryImplementationAOK := engine.ExactQueryImplementationAt[heapdomain.Value, rawHotHeapObservation](binding, heapQueryA)
	if !queryImplementationOK || queryImplementation == nil || !heapQueryImplementationAOK || heapQueryImplementationA == nil {
		t.Fatal("raw query implementation")
	}
	valueSeeds := make([]rawHotValueSeed, len(plan.valueCoordinates))
	seedPoint := rawHotID(60)
	for index := range valueSeeds {
		valueSeeds[index] = rawHotValueSeed{rawHotSeed: rawHotSeed{Capability: valueSeedCapabilities[index], Point: seedPoint, Occurrence: rawHotID(byte(61 + index))}, Implementation: valueSeedImplementations[index], Coordinate: plan.valueCoordinates[index], Value: plan.valueFacts[index]}
	}
	heapSeeds := make([]rawHotHeapSeed, len(plan.heapKeys))
	for index := range heapSeeds {
		heapSeeds[index] = rawHotHeapSeed{rawHotSeed: rawHotSeed{Capability: heapSeedCapabilities[index], Point: seedPoint, Occurrence: rawHotID(byte(70 + index))}, Implementation: heapSeedImplementations[index], Key: plan.heapKeys[index], Value: plan.heapFacts[index]}
	}
	scalar, scalarOK := rawHotScalarReceipt(artifact, cold.ID().Digest(), getCapability, setCapability, plan.getPoint, plan.setPoint, plan.getID, plan.setID, seedPoint, rawHotSeedIDs(valueSeeds), rawHotSeedIDs(heapSeeds))
	if !scalarOK {
		t.Fatal("raw scalar receipt")
	}
	mounted, mountedOK := engine.NewMountedArtifactReceipt(scalar, module)
	bootstrap, bootstrapOK := engine.NewLinkBootstrapWitness(rawHotID(50), engine.LinkBootstrapPoint{PointID: rawHotID(51), Known: true}, nil)
	assembly, assemblyOK := engine.BeginMountedArtifactReceiptAssembly(binding, []engine.MountedArtifactReceipt{mounted}, bootstrap)
	if !mountedOK || !bootstrapOK || !assemblyOK {
		t.Fatal("raw receipt assembly")
	}
	return rawHotFixture{mountID: module, getPoint: plan.getPoint, getOccurrence: plan.getID, setPoint: plan.setPoint, setOccurrence: plan.setID, getRule: getRule, setRule: setRule, assembly: assembly, binding: binding, values: values, heap: heap, valueSeeds: valueSeeds, heapSeeds: heapSeeds, queryID: rawHotID(18), queryImplementation: queryImplementation, heapQueryIDA: rawHotID(19), heapQueryImplementationA: heapQueryImplementationA}
}

func rawHotSeedIDs[T interface{ rawHotSeedValue() rawHotSeed }](seeds []T) []rawHotSeed {
	result := make([]rawHotSeed, len(seeds))
	for index, seed := range seeds {
		result[index] = seed.rawHotSeedValue()
	}
	return result
}

func (seed rawHotValueSeed) rawHotSeedValue() rawHotSeed { return seed.rawHotSeed }
func (seed rawHotHeapSeed) rawHotSeedValue() rawHotSeed  { return seed.rawHotSeed }

func rawHotScalarReceipt(artifact *programartifact.Artifact, schemaID [32]byte, getCapability, setCapability engine.RuleSlotCapability, getPoint, setPoint, getID, setID, seedPoint keyspace.ContentID, valueSeeds, heapSeeds []rawHotSeed) (*engine.ArtifactScalarReceipt, bool) {
	// Observe the read only after the write. A pre-write read would let the
	// cross-root non-interference law pass even if RawSet later fanned out.
	points := []engine.ArtifactScalarPoint{{ID: seedPoint, Initial: true}, {ID: setPoint}, {ID: getPoint}}
	regionID, bodyID := rawHotID(60), rawHotID(61)
	members := make([]keyspace.ContentID, len(points))
	for index, point := range points {
		members[index] = point.ID
	}
	type scalarRuleBinding struct {
		capability engine.RuleSlotCapability
		rule       engine.ArtifactScalarRule
	}
	rules := make([]scalarRuleBinding, 0, len(valueSeeds)+len(heapSeeds)+2)
	for _, seed := range append(append([]rawHotSeed(nil), valueSeeds...), heapSeeds...) {
		rules = append(rules, scalarRuleBinding{capability: seed.Capability, rule: engine.ArtifactScalarRule{Stage: engine.ArtifactRuleStageBase, Point: seed.Point, ID: seed.Occurrence}})
	}
	rules = append(rules,
		scalarRuleBinding{capability: getCapability, rule: engine.ArtifactScalarRule{Stage: engine.ArtifactRuleStageLocal, Point: getPoint, Input: setPoint, ID: getID}},
		scalarRuleBinding{capability: setCapability, rule: engine.ArtifactScalarRule{Stage: engine.ArtifactRuleStageLocal, Point: setPoint, Input: seedPoint, ID: setID}},
	)
	spec, specOK := engine.NewArtifactScalarSpec(artifact.ID(), artifact.CompileKey().ProgramID(), keyspace.ContentID(schemaID), engine.ArtifactScalarCapacity{Roles: len(rules), Points: len(points), Transfers: 2, Regions: 1, Events: 5, Rules: len(rules), Bodies: 1})
	if !specOK {
		return nil, false
	}
	roleByCapability := make(map[engine.RuleSlotCapability]engine.ArtifactScalarRole, len(rules))
	roleOrder := make([]scalarRuleBinding, 0, len(rules))
	for _, row := range rules {
		if _, declared := roleByCapability[row.capability]; declared {
			continue
		}
		role, roleOK := spec.DeclareRole(rawHotID(byte(0x80 + len(roleOrder))))
		if !roleOK {
			return nil, false
		}
		roleByCapability[row.capability] = role
		roleOrder = append(roleOrder, row)
	}
	for _, point := range points {
		if _, ok := spec.AddPoint(point); !ok {
			return nil, false
		}
	}
	if _, ok := spec.AddTransfer(engine.ArtifactScalarTransfer{ID: rawHotID(0xD0), From: seedPoint, To: setPoint, Full: true}); !ok {
		return nil, false
	}
	if _, ok := spec.AddTransfer(engine.ArtifactScalarTransfer{ID: rawHotID(0xD1), From: setPoint, To: getPoint, Full: true}); !ok {
		return nil, false
	}
	region, regionOK := spec.AddRegion(engine.ArtifactScalarRegion{ID: regionID, Head: members[0]})
	if !regionOK {
		return nil, false
	}
	for _, member := range members {
		if !spec.AddRegionMember(region, member) {
			return nil, false
		}
	}
	for _, event := range []engine.ArtifactScalarEvent{{Kind: engine.ArtifactEventEnter, Region: regionID}, {Kind: engine.ArtifactEventPoint, Point: members[0]}, {Kind: engine.ArtifactEventPoint, Point: members[1]}, {Kind: engine.ArtifactEventPoint, Point: members[2]}, {Kind: engine.ArtifactEventExit, Region: regionID}} {
		if !spec.AddEvent(event) {
			return nil, false
		}
	}
	body, bodyOK := spec.AddBody(engine.ArtifactScalarBody{ID: bodyID, Context: rawHotID(0xE0), SemanticEntry: rawHotID(0xE1)})
	if !bodyOK || !spec.AddBodyEntry(body, members[0]) || !spec.AddBodyExit(body, members[len(members)-1]) {
		return nil, false
	}
	for _, row := range rules {
		row.rule.Role = roleByCapability[row.capability]
		if !spec.AddRule(row.rule) {
			return nil, false
		}
	}
	template, templateOK := engine.NewArtifactScalarTemplate(spec)
	binding, bindingOK := engine.NewArtifactScalarBinding(template)
	if !templateOK || !bindingOK {
		return nil, false
	}
	for _, row := range roleOrder {
		if !binding.BindRole(roleByCapability[row.capability], row.capability) {
			return nil, false
		}
	}
	return engine.NewArtifactScalarReceipt(binding)
}

func rawHotKey(value byte) engine.SemanticKey {
	return rawHotSemantic(value)
}

func rawHotID(value byte) keyspace.ContentID {
	digest := sha256.Sum256([]byte{0xE1, value})
	return keyspace.ContentID(digest)
}

func rawHotSemantic(value byte) engine.SemanticKey {
	digest := sha256.Sum256([]byte{0xE2, value})
	key, _ := engine.NewSemanticKey(digest, 1)
	return key
}
