package index_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	"github.com/wippyai/go-lua/domain/runtimekind"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	Point      identity.ContentID
	Occurrence identity.ContentID
}

type rawHotValueSeed struct {
	rawHotSeed
	Implementation *valueowner.RuleImplementation[identity.ContentID]
}

type rawHotHeapSeed struct {
	rawHotSeed
	Implementation *heapowner.RuleImplementation[identity.ContentID]
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
			Present: func(value rawHotQueryObservation) bool { return value.Rows != 0 },
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
			Present: func(value rawHotHeapObservation) bool { return value.Rows != 0 },
		},
	}, true
}

func TestRawHotMountedPathSolve(t *testing.T) {
	fixture := rawHotMountedFixture(t)
	if fixture.getOccurrence == (identity.ContentID{}) || fixture.setOccurrence == (identity.ContentID{}) {
		t.Fatal("missing raw occurrences")
	}
	if fixture.getRule == nil || fixture.setRule == nil {
		t.Fatal("missing hot rules")
	}
	if fixture.binding == nil || fixture.mount.Template == nil || fixture.queryImplementation == nil {
		t.Fatal("missing assembly")
	}
	if fixture.heapQueryImplementationA == nil || len(fixture.valueSeeds) != 4 || len(fixture.heapSeeds) != 2 {
		t.Fatal("missing owner-issued seeds")
	}
	getImpl, getImplOK := fixture.getRule.Implementation()
	setImpl, setImplOK := fixture.setRule.Implementation()
	getCap, getCapOK := engine.RuleSlotCapability{}, false
	setCap, setCapOK := engine.RuleSlotCapability{}, false
	if getImplOK {
		getCap, getCapOK = getImpl.MountedCapability()
	}
	if setImplOK {
		setCap, setCapOK = setImpl.MountedCapability()
	}
	getDeclaration, getDeclarationOK := indexdomain.SealRawGetProgramRule(fixture.getRule)
	setDeclaration, setDeclarationOK := indexdomain.SealRawSetProgramRule(fixture.setRule)
	if !getImplOK || !setImplOK || !getCapOK || !setCapOK || !getDeclarationOK || !setDeclarationOK {
		t.Fatalf("raw mounted declarations getImpl=%t setImpl=%t getCapability=%t setCapability=%t getRule=%t setRule=%t", getImplOK, setImplOK, getCapOK, setCapOK, getDeclarationOK, setDeclarationOK)
	}
	mounted := make([]engine.MountedRuleAdmission, 0, len(fixture.valueSeeds)+len(fixture.heapSeeds)+2)
	for _, seed := range fixture.valueSeeds {
		admission, admissionOK := rawHotValueSeedAdmission(fixture.mountID, seed)
		if !admissionOK {
			t.Fatal("raw value seed")
		}
		mounted = append(mounted, admission)
	}
	for _, seed := range fixture.heapSeeds {
		admission, admissionOK := rawHotHeapSeedAdmission(fixture.mountID, seed)
		if !admissionOK {
			t.Fatal("raw heap seed")
		}
		mounted = append(mounted, admission)
	}
	mounted = append(mounted,
		engine.MountedRuleAdmission{Declaration: getDeclaration, Capability: getCap, Mount: fixture.mountID, Point: fixture.getPoint, Occurrence: fixture.getOccurrence},
		engine.MountedRuleAdmission{Declaration: setDeclaration, Capability: setCap, Mount: fixture.mountID, Point: fixture.setPoint, Occurrence: fixture.setOccurrence},
	)
	valueQueryAdmission, valueQueryOK := engine.NewExactQueryAdmission(fixture.queryImplementation, fixture.queryID, fixture.mountID, fixture.getPoint)
	heapQueryAdmission, heapQueryOK := engine.NewExactQueryAdmission(fixture.heapQueryImplementationA, fixture.heapQueryIDA, fixture.mountID, fixture.setPoint)
	if !valueQueryOK || !heapQueryOK {
		t.Fatalf("raw mounted query rows valueQuery=%t heapQuery=%t", valueQueryOK, heapQueryOK)
	}
	program, refusal, committed := engine.ConstructProgram(engine.ProgramDeclaration{
		Binding:   fixture.binding,
		Mounts:    []engine.MountedProgramArtifact{fixture.mount},
		Bootstrap: fixture.bootstrap,
		Admission: engine.MountedProgramAdmission{
			Mounted: mounted,
			Queries: []engine.ProgramQueryAdmission{valueQueryAdmission, heapQueryAdmission},
		},
	})
	if !committed || program == nil {
		ordinal, artifactRow := refusal.ArtifactRowOrdinal()
		_, mountedRole := refusal.MountedRole()
		t.Fatalf("raw mounted construct stage=%v lowered=%t lowering=%v seal=%v commit=%v schedule=%d artifactRow=%d/%t mountedRole=%t", refusal.Stage(), refusal.Lowered(), refusal.LoweringFailure(), refusal.Seal(), refusal.Commit(), refusal.ScheduleRow(), ordinal, artifactRow, mountedRole)
	}
	// Every occurrence the declaration admitted must be addressable as a
	// published member of the committed program.
	for _, row := range mounted {
		if _, memberOK := program.MountedRuleMember(row.Capability, row.Mount, row.Point, row.Occurrence); !memberOK {
			t.Fatalf("raw mounted member point=%x occurrence=%x", row.Point, row.Occurrence)
		}
	}
	query, queryOK := program.Query(fixture.queryID)
	heapQueryA, heapQueryAOK := program.Query(fixture.heapQueryIDA)
	if !queryOK || !heapQueryAOK {
		t.Fatal("raw query member")
	}
	solver, sealFailure, solverOK := program.Seal(nil)
	if !solverOK || solver == nil {
		t.Fatalf("raw solver seal=%v", sealFailure)
	}
	state, status, report := solver.SolveWithReport(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("raw solve status=%v state=%t reason=%v phase=%v point=%x group=%x member=%x rule=%x", status, state != nil, report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule())
	}
	sealed, publishedOK := solver.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("raw solver published no snapshot")
	}
	published := sealed.Snapshot()
	plan, planOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.QueryFamily())
	if !planOK {
		t.Fatal("raw query family did not open")
	}
	queryKey, queryKeyOK := query.PublicationKey()
	if !queryKeyOK {
		t.Fatal("raw query has no snapshot key")
	}
	queryAnswer, queryStatus := snapshot.Query(&published, plan, queryKey)
	observation, observationOK := engine.AnswerValue[rawHotQueryObservation](queryAnswer)
	if queryStatus != snapshot.ReadHit {
		observationOK = false
	}
	// RawSet targets a only. The subsequent read of b.source must retain the
	// seeded table; absence here would prove an unsound cross-root fan-out.
	if !observationOK || observation.Rows == 0 || observation.Present == 0 || observation.Table != observation.Present || observation.EqualSource != observation.Present || observation.Absent != 0 || observation.Top != 0 || observation.Bottom != 0 || observation.NonTable != 0 || observation.Unavailable != 0 {
		t.Fatalf("raw query observation=%#v readable=%t", observation, observationOK)
	}
	heapKey, heapKeyOK := heapQueryA.PublicationKey()
	if !heapKeyOK {
		t.Fatal("raw heap query has no snapshot key")
	}
	heapAnswer, heapStatus := snapshot.Query(&published, plan, heapKey)
	observationA, observationAOK := engine.AnswerValue[rawHotHeapObservation](heapAnswer)
	if heapStatus != snapshot.ReadHit {
		observationAOK = false
	}
	if !observationAOK || observationA.Rows == 0 || observationA.Unavailable != 0 || observationA.Top != 0 || observationA.Bottom != 0 || observationA.Changed == 0 || observationA.EqualSeed != 0 {
		t.Fatalf("raw set target observation=%#v readable=%t", observationA, observationAOK)
	}
}

func rawHotValueSeedAdmission(mount identity.ContentID, seed rawHotValueSeed) (engine.MountedRuleAdmission, bool) {
	implementation, implementationOK := valueowner.ResolveRuleImplementation(seed.Implementation)
	if !implementationOK {
		return engine.MountedRuleAdmission{}, false
	}
	declaration, declarationOK := engine.SealProgramRule(implementation)
	if !declarationOK {
		return engine.MountedRuleAdmission{}, false
	}
	return engine.MountedRuleAdmission{Declaration: declaration, Capability: seed.Capability, Mount: mount, Point: seed.Point, Occurrence: seed.Occurrence}, true
}

func rawHotHeapSeedAdmission(mount identity.ContentID, seed rawHotHeapSeed) (engine.MountedRuleAdmission, bool) {
	implementation, implementationOK := heapowner.ResolveRuleImplementation(seed.Implementation)
	if !implementationOK {
		return engine.MountedRuleAdmission{}, false
	}
	declaration, declarationOK := engine.SealProgramRule(implementation)
	if !declarationOK {
		return engine.MountedRuleAdmission{}, false
	}
	return engine.MountedRuleAdmission{Declaration: declaration, Capability: seed.Capability, Mount: mount, Point: seed.Point, Occurrence: seed.Occurrence}, true
}

type rawHotMountedPlan struct {
	getID, getPoint  identity.ContentID
	setID, setPoint  identity.ContentID
	valueCoordinates []valuedomain.Coordinate
	valueFacts       []valuedomain.Value
	heapKeys         []heapdomain.Key
	heapFacts        []heapdomain.Value
}

// rawHotRuleRows resolves the canonical RuleOccurrence ordinal to its parent
// occurrence. RuleOccurrence carries placement metadata only; the parent
// Program row remains the authority for the occurrence identity consumed by
// the mounted domain schemas.
type rawHotRuleRow struct {
	rule       programschema.RuleOccurrence
	occurrence programschema.Occurrence
}

func rawHotRuleRows(artifact *programartifact.Artifact, key string) []rawHotRuleRow {
	program := artifact.Program()
	count, published := program.RuleOccurrenceCountForKey(key)
	if !published {
		return nil
	}
	rows := make([]rawHotRuleRow, 0, count)
	for index := 0; index < count; index++ {
		rule, ruleOK := program.RuleOccurrenceForKeyAt(key, index)
		ordinal, ordinalOK := rule.Occurrence()
		occurrence, occurrenceOK := program.OccurrenceAt(int(ordinal))
		if ruleOK && ordinalOK && occurrenceOK {
			rows = append(rows, rawHotRuleRow{rule: rule, occurrence: occurrence})
		}
	}
	return rows
}

func buildRawHotMountedPlan(t testing.TB, linked *link.Link, module identity.ContentID, artifact *programartifact.Artifact, heapMount heapdomain.ArtifactMount, heapSchema heapdomain.Schema, valueSchema *valuedomain.Schema, packs *packdomain.Schema) rawHotMountedPlan {
	t.Helper()
	occurrenceMount, occurrenceMountOK := heapSchema.OccurrenceMountForModule(module)
	if !occurrenceMountOK {
		t.Fatal("raw mounted occurrence issuer")
	}
	var getID, setID identity.ContentID
	var getGeometry, setGeometry heapdomain.IndexGeometry
	var getAccess heapdomain.IndexAccess
	var nilSourceAccess, tableSourceAccess heapdomain.IndexAccess
	var nilSource, tableSource packdomain.SemanticSource
	var mountedCount, fixedCount, tailCount, nilCount, sourceFactCount, nilSourceCount, tableSourceCount int
	getRows := rawHotRuleRows(artifact, "raw-get")
	setRows := rawHotRuleRows(artifact, "raw-set")
	for _, entry := range getRows {
		occurrenceID := entry.occurrence.ID()
		access, accessOK := occurrenceMount.IndexAccessForOccurrence(occurrenceID, true)
		geometry, geometryOK := heapSchema.IndexAccessGeometry(access)
		if accessOK && geometryOK && !getID.Available() {
			getID, getAccess, getGeometry = occurrenceID, access, geometry
		}
	}
	for _, entry := range setRows {
		occurrenceID := entry.occurrence.ID()
		access, accessOK := occurrenceMount.IndexAccessForOccurrence(occurrenceID, false)
		geometry, geometryOK := heapSchema.IndexAccessGeometry(access)
		if !accessOK || !geometryOK {
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
					setID, setGeometry, nilSourceAccess, nilSource = occurrenceID, geometry, access, candidate
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
		t.Fatalf("raw mounted nil/fixed write plan writes=%d mounted=%d fixed=%d tail=%d nil=%d facts=%d nilSources=%d tableSources=%d", len(setRows), mountedCount, fixedCount, tailCount, nilCount, sourceFactCount, nilSourceCount, tableSourceCount)
	}
	if getID == (identity.ContentID{}) {
		t.Fatal("raw mounted read occurrence")
	}
	if getGeometry.BaseValueID == setGeometry.BaseValueID {
		for _, entry := range getRows {
			occurrenceID := entry.occurrence.ID()
			access, accessOK := occurrenceMount.IndexAccessForOccurrence(occurrenceID, true)
			geometry, geometryOK := heapSchema.IndexAccessGeometry(access)
			pointOK := entry.rule.PointID().Available()
			if accessOK && geometryOK && pointOK && geometry.BaseValueID != setGeometry.BaseValueID {
				getID, getAccess, getGeometry = occurrenceID, access, geometry
				break
			}
		}
	}
	if getGeometry.BaseValueID == setGeometry.BaseValueID {
		t.Fatal("raw mounted read did not target foreign root")
	}
	var getPoint, setPoint identity.ContentID
	var getPointOK, setPointOK bool
	for _, entry := range getRows {
		if entry.occurrence.ID() == getID {
			getPoint, getPointOK = entry.rule.PointID(), entry.rule.PointID().Available()
			break
		}
	}
	for _, entry := range setRows {
		if entry.occurrence.ID() == setID {
			setPoint, setPointOK = entry.rule.PointID(), entry.rule.PointID().Available()
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

func rawHotScalarValue(id identity.ContentID, schema *valuedomain.Schema) (valuedomain.Coordinate, valuedomain.Value, bool) {
	if schema == nil {
		return valuedomain.Coordinate{}, valuedomain.Value{}, false
	}
	coordinate, coordinateOK := schema.CoordinateForID(id)
	fact, factOK := schema.SourceValueID(id)
	return coordinate, fact, coordinateOK && factOK
}

func heapKeyForRootValue(schema heapdomain.Schema, rootValueID identity.ContentID) (heapdomain.Key, bool) {
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
		_, _, _, kind, _, sourceOK := schema.AllocationOriginForKey(key)
		if !keyOK || !sourceOK || kind != heapdomain.AllocationTable || key == source {
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

func mustIndexOccurrenceID(t testing.TB, schema heapdomain.Schema, access heapdomain.IndexAccess) identity.ContentID {
	t.Helper()
	module, occurrence, _, ok := schema.IndexAccessOccurrence(access)
	if !ok || !module.Available() || !occurrence.Available() {
		t.Fatal("raw mounted occurrence identity")
	}
	return occurrence
}

type rawHotFixture struct {
	mountID                  identity.ContentID
	getPoint                 identity.ContentID
	getOccurrence            identity.ContentID
	setPoint                 identity.ContentID
	setOccurrence            identity.ContentID
	getRule                  *indexdomain.RawGetHotRule
	setRule                  *indexdomain.RawSetHotRule
	binding                  *engine.SchemaBinding
	mount                    engine.MountedProgramArtifact
	bootstrap                engine.ProgramBootstrap
	valueSeeds               []rawHotValueSeed
	heapSeeds                []rawHotHeapSeed
	queryID                  identity.ContentID
	queryImplementation      *engine.ExactQueryImplementation[valuedomain.Value, rawHotQueryObservation]
	heapQueryIDA             identity.ContentID
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
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mount identity")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("artifact: %v", failure)
	}
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	callMount := calldomain.MountedArtifact{Program: snapshottest.MustMount(t, artifact, module), Snapshot: snapshottest.MustLower(t, artifact)}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	valueSchema, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{callMount})
	packMount, packMountOK := packdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	staticMount := staticdomain.MountedProgram{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}
	types, typeErr := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()})
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedProgram{staticMount})
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
	valueFragment, valueFragmentOK := valueowner.DeclareSchema(builder, rawHotKey(1), rawHotKey(2), rawHotKey(101))
	callFragment, callFragmentOK := callowner.DeclareSchema(builder, rawHotKey(3))
	heapFragment, heapFragmentOK := heapowner.DeclareSchema(builder, rawHotKey(4), rawHotKey(16))
	packFragment, packFragmentOK := packowner.DeclareSchema(builder, rawHotKey(5))
	getFragment, getFragmentOK := indexdomain.DeclareRawGetSchema(builder, rawHotKey(6), rawHotKey(7), valueFragment, callFragment, heapFragment, packFragment)
	setFragment, setFragmentOK := indexdomain.DeclareRawSetSchema(builder, rawHotKey(9), rawHotKey(10), valueFragment, heapFragment, packFragment)
	valueSeedRules := make([]*engine.RuleSlot[valuedomain.Value, identity.ContentID], len(plan.valueCoordinates))
	valueSeedWrites := make([]engine.SchemaWriteSlot[valuedomain.Value], len(plan.valueCoordinates))
	valueSeedOK := true
	for index := range valueSeedRules {
		seedRule, seedRuleOK := engine.DeclareRuleSlot[valuedomain.Value, identity.ContentID](builder, engine.SchemaRuleSpec[valuedomain.Value]{Semantic: rawHotKey(byte(20 + index*3)), OperandFamily: rawHotKey(byte(21 + index*3)), Inputs: 0, Output: valueFragment.Ref()})
		seedWrite, seedWriteOK := engine.SchemaWrite(seedRule, valueFragment.ExactWrite())
		valueSeedRules[index], valueSeedWrites[index], valueSeedOK = seedRule, seedWrite, valueSeedOK && seedRuleOK && seedWriteOK
	}
	heapSeedRules := make([]*engine.RuleSlot[heapdomain.Value, identity.ContentID], len(plan.heapKeys))
	heapSeedWrites := make([]engine.SchemaWriteSlot[heapdomain.Value], len(plan.heapKeys))
	heapSeedOK := true
	for index := range heapSeedRules {
		seedRule, seedRuleOK := engine.DeclareRuleSlot[heapdomain.Value, identity.ContentID](builder, engine.SchemaRuleSpec[heapdomain.Value]{Semantic: rawHotKey(byte(40 + index*3)), OperandFamily: rawHotKey(byte(41 + index*3)), Inputs: 0, Output: heapFragment.Ref()})
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
	valueSeedImplementations := make([]*valueowner.RuleImplementation[identity.ContentID], len(valueSeedRules))
	valueSeedBindOK := true
	for index, seedRule := range valueSeedRules {
		fact := plan.valueFacts[index]
		coordinateIndex, coordinateIndexOK := valueSchema.CoordinateIndex(plan.valueCoordinates[index])
		if !coordinateIndexOK {
			t.Fatal("raw value seed coordinate")
		}
		local := uint64(coordinateIndex)
		occurrence := rawHotID(byte(61 + index))
		implementation, implementationOK := valueowner.BindExactWriteRule(values, seedRule, valueSeedWrites[index], engine.HotRuleSpec[valuedomain.Value, identity.ContentID]{
			OperandContent: func(value identity.ContentID) (identity.ContentID, [32]byte, bool) {
				return value, sha256.Sum256(value[:]), value.Available()
			},
			Fold: func(frame engine.Frame[valuedomain.Value, identity.ContentID]) engine.RuleResult[valuedomain.Value] {
				return engine.Staged(frame, fact)
			},
		}, func(operand identity.ContentID) (uint64, bool) { return local, operand == occurrence })
		valueSeedBindOK = valueSeedBindOK && implementationOK && implementation.InstallOperandResolver(func(coords engine.OperandCoords) (identity.ContentID, bool) {
			return coords.Occurrence, coords.Occurrence == occurrence
		})
		valueSeedImplementations[index] = implementation
	}
	heapSeedImplementations := make([]*heapowner.RuleImplementation[identity.ContentID], len(heapSeedRules))
	heapSeedBindOK := true
	for index, seedRule := range heapSeedRules {
		fact := plan.heapFacts[index]
		keyIndex, keyIndexOK := heapSchema.KeyIndex(plan.heapKeys[index])
		if !keyIndexOK || keyIndex < 0 {
			t.Fatal("raw heap seed key")
		}
		local := uint64(keyIndex)
		occurrence := rawHotID(byte(70 + index))
		implementation, implementationOK := heapowner.BindExactWriteRule(heap, seedRule, heapSeedWrites[index], engine.HotRuleSpec[heapdomain.Value, identity.ContentID]{
			OperandContent: func(value identity.ContentID) (identity.ContentID, [32]byte, bool) {
				return value, sha256.Sum256(value[:]), value.Available()
			},
			Fold: func(frame engine.Frame[heapdomain.Value, identity.ContentID]) engine.RuleResult[heapdomain.Value] {
				return engine.Staged(frame, fact)
			},
		}, func(operand identity.ContentID) (uint64, bool) { return local, operand == occurrence })
		heapSeedBindOK = heapSeedBindOK && implementationOK && implementation.InstallOperandResolver(func(coords engine.OperandCoords) (identity.ContentID, bool) {
			return coords.Occurrence, coords.Occurrence == occurrence
		})
		heapSeedImplementations[index] = implementation
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
		valueSeeds[index] = rawHotValueSeed{rawHotSeed: rawHotSeed{Capability: valueSeedCapabilities[index], Point: seedPoint, Occurrence: rawHotID(byte(61 + index))}, Implementation: valueSeedImplementations[index]}
	}
	heapSeeds := make([]rawHotHeapSeed, len(plan.heapKeys))
	for index := range heapSeeds {
		heapSeeds[index] = rawHotHeapSeed{rawHotSeed: rawHotSeed{Capability: heapSeedCapabilities[index], Point: seedPoint, Occurrence: rawHotID(byte(70 + index))}, Implementation: heapSeedImplementations[index]}
	}
	mount, mountOK := rawHotScalarMount(artifact, module, cold.ID().Digest(), getCapability, setCapability, plan.getPoint, plan.setPoint, plan.getID, plan.setID, seedPoint, rawHotSeedIDs(valueSeeds), rawHotSeedIDs(heapSeeds))
	if !mountOK {
		t.Fatal("raw scalar mount")
	}
	bootstrap, bootstrapOK := engine.NewProgramBootstrap(rawHotID(50), rawHotID(51))
	if !bootstrapOK {
		t.Fatal("raw bootstrap witness")
	}
	return rawHotFixture{mountID: module, getPoint: plan.getPoint, getOccurrence: plan.getID, setPoint: plan.setPoint, setOccurrence: plan.setID, getRule: getRule, setRule: setRule, binding: binding, mount: mount, bootstrap: bootstrap, valueSeeds: valueSeeds, heapSeeds: heapSeeds, queryID: rawHotID(18), queryImplementation: queryImplementation, heapQueryIDA: rawHotID(19), heapQueryImplementationA: heapQueryImplementationA}
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

func rawHotScalarMount(artifact *programartifact.Artifact, module identity.ContentID, schemaID [32]byte, getCapability, setCapability engine.RuleSlotCapability, getPoint, setPoint, getID, setID, seedPoint identity.ContentID, valueSeeds, heapSeeds []rawHotSeed) (engine.MountedProgramArtifact, bool) {
	// Observe the read only after the write. A pre-write read would let the
	// cross-root non-interference law pass even if RawSet later fanned out.
	points := []rows.ArtifactScalarPoint{{ID: seedPoint, Initial: true}, {ID: setPoint}, {ID: getPoint}}
	regionID, bodyID := rawHotID(60), rawHotID(61)
	members := make([]identity.ContentID, len(points))
	for index, point := range points {
		members[index] = point.ID
	}
	type scalarRuleBinding struct {
		capability engine.RuleSlotCapability
		rule       rows.ArtifactScalarRule
	}
	rules := make([]scalarRuleBinding, 0, len(valueSeeds)+len(heapSeeds)+2)
	for _, seed := range append(append([]rawHotSeed(nil), valueSeeds...), heapSeeds...) {
		rules = append(rules, scalarRuleBinding{capability: seed.Capability, rule: rows.ArtifactScalarRule{Stage: rows.ArtifactRuleStageBase, Point: seed.Point, ID: seed.Occurrence}})
	}
	rules = append(rules,
		scalarRuleBinding{capability: getCapability, rule: rows.ArtifactScalarRule{Stage: rows.ArtifactRuleStageLocal, Point: getPoint, Input: setPoint, ID: getID}},
		scalarRuleBinding{capability: setCapability, rule: rows.ArtifactScalarRule{Stage: rows.ArtifactRuleStageLocal, Point: setPoint, Input: seedPoint, ID: setID}},
	)
	spec, specOK := rows.NewArtifactScalarSpec(artifact.ID(), artifact.CompileKey().ProgramID(), identity.ContentID(schemaID), rows.ArtifactScalarCapacity{Roles: len(rules), Points: len(points), Transfers: 2, Regions: 1, Events: 5, Rules: len(rules), Bodies: 1})
	if !specOK {
		return engine.MountedProgramArtifact{}, false
	}
	roleByCapability := make(map[engine.RuleSlotCapability]rows.ArtifactScalarRole, len(rules))
	roleOrder := make([]scalarRuleBinding, 0, len(rules))
	for _, row := range rules {
		if _, declared := roleByCapability[row.capability]; declared {
			continue
		}
		role, roleOK := spec.DeclareRole(rawHotID(byte(0x80 + len(roleOrder))))
		if !roleOK {
			return engine.MountedProgramArtifact{}, false
		}
		roleByCapability[row.capability] = role
		roleOrder = append(roleOrder, row)
	}
	for _, point := range points {
		if _, ok := spec.AddPoint(point); !ok {
			return engine.MountedProgramArtifact{}, false
		}
	}
	if _, ok := spec.AddTransfer(rows.ArtifactScalarTransfer{ID: rawHotID(0xD0), From: seedPoint, To: setPoint, Full: true}); !ok {
		return engine.MountedProgramArtifact{}, false
	}
	if _, ok := spec.AddTransfer(rows.ArtifactScalarTransfer{ID: rawHotID(0xD1), From: setPoint, To: getPoint, Full: true}); !ok {
		return engine.MountedProgramArtifact{}, false
	}
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: regionID, Head: members[0]})
	if !regionOK {
		return engine.MountedProgramArtifact{}, false
	}
	for _, member := range members {
		if !spec.AddRegionMember(region, member) {
			return engine.MountedProgramArtifact{}, false
		}
	}
	for _, event := range []rows.ArtifactScalarEvent{{Kind: rows.ArtifactEventEnter, Region: regionID}, {Kind: rows.ArtifactEventPoint, Point: members[0]}, {Kind: rows.ArtifactEventPoint, Point: members[1]}, {Kind: rows.ArtifactEventPoint, Point: members[2]}, {Kind: rows.ArtifactEventExit, Region: regionID}} {
		if !spec.AddEvent(event) {
			return engine.MountedProgramArtifact{}, false
		}
	}
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: bodyID})
	if !bodyOK || !spec.AddBodyEntry(body, members[0]) || !spec.AddBodyExit(body, members[len(members)-1]) {
		return engine.MountedProgramArtifact{}, false
	}
	for _, row := range rules {
		row.rule.Role = roleByCapability[row.capability]
		if !spec.AddRule(row.rule) {
			return engine.MountedProgramArtifact{}, false
		}
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	if !templateOK {
		return engine.MountedProgramArtifact{}, false
	}
	roles := make([]engine.MountedProgramRole, 0, len(roleOrder))
	for _, row := range roleOrder {
		roles = append(roles, engine.MountedProgramRole{Scalar: roleByCapability[row.capability], Capability: row.capability})
	}
	return engine.MountedProgramArtifact{Template: template, Roles: roles, Module: module}, true
}

func rawHotKey(value byte) identity.SemanticKey {
	return rawHotSemantic(value)
}

func rawHotID(value byte) identity.ContentID {
	digest := sha256.Sum256([]byte{0xE1, value})
	return identity.ContentID(digest)
}

func rawHotSemantic(value byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xE2, value})
	key, _ := identity.NewSemanticKey(digest, 1)
	return key
}
