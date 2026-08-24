package linkexecutionplan

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	modulecomposition "github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

type boundEdgeLawProgram struct {
	sourceMount programmount.Program
	targetMount programmount.Program
	request     programschema.ModuleRequest
	importRow   programschema.ModuleImport
	body        programschema.Body
}

type boundEdgeSCCFixture struct {
	graph      *equation.Graph
	directory  executioncontext.Directory
	layout     contextfiber.Layout
	points     [2]equation.Point
	modules    [2]identity.ContentID
	contexts   [2]executioncontext.Context
	forward    BoundEdge
	reverse    BoundEdge
	forwardRow modulecomposition.ModuleCallTransition
	reverseRow modulecomposition.ModuleCallTransition
	forwardGen modulecomposition.InitGeneration
	reverseGen modulecomposition.InitGeneration
}

func boundEdgeLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("link-execution-plan/bound-edge-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func boundEdgeSpec(t *testing.T, directory executioncontext.Directory, row modulecomposition.ModuleCallTransition, generation modulecomposition.InitGeneration) BoundEdgeSpec {
	t.Helper()
	activation, ok := directory.ActivationEdge(row.FromContextID(), row.ToContextID())
	if !ok {
		t.Fatal("activation")
	}
	return BoundEdgeSpec{TransitionID: activation.ID(), GenerationID: generation.ID(), FromContextID: row.FromContextID(), ToContextID: row.ToContextID()}
}

func newBoundEdgeLawProgram(t *testing.T, sourceModule, targetModule identity.ContentID) boundEdgeLawProgram {
	t.Helper()
	importID := boundEdgeLawID(t, "import")
	callID := boundEdgeLawID(t, "call")
	requestID := boundEdgeLawID(t, "request")
	valueID := boundEdgeLawID(t, "request-value")
	importRow, ok := programschema.NewModuleImport(importID, callID, identity.ContentID{}, 0, 1, false)
	if !ok {
		t.Fatal("module import")
	}
	request, ok := programschema.NewModuleRequest(requestID, importID, valueID, keyspace.Key(7))
	if !ok {
		t.Fatal("module request")
	}
	schemaID := boundEdgeLawID(t, "program-schema")
	catalogID, ok := programcatalog.CatalogID(schemaID)
	if !ok {
		t.Fatal("program catalog")
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("program store")
	}
	bodyID := boundEdgeLawID(t, "entry-body")
	body, ok := programschema.NewBody(bodyID, boundEdgeLawID(t, "body-context"), boundEdgeLawID(t, "body-entry"), identity.ContentID{}, identity.ContentID{}, 0, 1, 0, 0, 0, 5, false)
	if !ok {
		t.Fatal("entry body")
	}
	newOutcome := func(label string, kind programschema.OutcomeKind) programschema.Outcome {
		t.Helper()
		outcome, outcomeOK := programschema.NewOutcome(boundEdgeLawID(t, label), bodyID, identity.ContentID{}, identity.ContentID{}, kind, 0, 0, 0, 0, false, false)
		if !outcomeOK {
			t.Fatalf("%s outcome", label)
		}
		return outcome
	}
	normal := newOutcome("normal-outcome", programschema.OutcomeNormal)
	returned := newOutcome("return-outcome", programschema.OutcomeReturn)
	thrown := newOutcome("throw-outcome", programschema.OutcomeThrow)
	yielded := newOutcome("yield-outcome", programschema.OutcomeYield)
	canceled := newOutcome("cancel-outcome", programschema.OutcomeCancel)
	entry, ok := programschema.NewModuleEntry(boundEdgeLawID(t, "return-entry"), returned.ID(), 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("return entry")
	}
	dispatchPointID := boundEdgeLawID(t, "call-dispatch-point")
	summaryPointID := boundEdgeLawID(t, "call-summary-point")
	effectPointID := boundEdgeLawID(t, "call-effect-point")
	basePointID := boundEdgeLawID(t, "call-base-point")
	callOccurrence, ok := programschema.NewOccurrence(programschema.OccurrenceCall, callID, bodyID, 0, 0, 1, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	if !ok {
		t.Fatal("call occurrence")
	}
	dispatchPoint, ok := programschema.NewOccurrencePoint(dispatchPointID)
	summaryPoint, summaryPointOK := programschema.NewOccurrencePoint(summaryPointID)
	effectPoint, effectPointOK := programschema.NewOccurrencePoint(effectPointID)
	if !ok || !summaryPointOK || !effectPointOK {
		t.Fatal("call occurrence points")
	}
	basePoint, basePointOK := programschema.NewOccurrencePoint(basePointID)
	// One call occurrence issues the whole canonical stage chain
	// Base -> CallDispatch -> CallSummary -> CallEffect, so the module-call
	// join finds both the dispatch point it departs from and the effect point
	// the callee's return state lands on.
	dispatchRule, ok := programschema.NewRuleOccurrenceWithInputs(schema.Key("bound-edge-law-call-dispatch"), schema.Key("bound-edge-law-call-dispatch-axis"), 0, dispatchPointID, []identity.ContentID{basePointID}, programissuance.StageCallDispatch, programissuance.InputPreviousStage, programschema.RuleOccurrenceRoute{}, true, programschema.RuleOccurrenceSource{})
	summaryRule, summaryRuleOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("bound-edge-law-call-summary"), schema.Key("bound-edge-law-call-summary-axis"), 0, summaryPointID, []identity.ContentID{dispatchPointID}, programissuance.StageCallSummary, programissuance.InputCallDispatchStage, programschema.RuleOccurrenceRoute{}, true, programschema.RuleOccurrenceSource{})
	effectRule, effectRuleOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("bound-edge-law-call-effect"), schema.Key("bound-edge-law-call-effect-axis"), 0, effectPointID, []identity.ContentID{summaryPointID}, programissuance.StageCallEffect, programissuance.InputCallSummaryStage, programschema.RuleOccurrenceRoute{}, true, programschema.RuleOccurrenceSource{})
	if !ok || !summaryRuleOK || !effectRuleOK || !basePointOK {
		t.Fatal("call stage rule occurrences")
	}
	frozen, ok := (programpublication.Publication{
		ModuleImports:    []programschema.ModuleImport{importRow},
		ModuleRequests:   []programschema.ModuleRequest{request},
		Occurrences:      []programschema.Occurrence{callOccurrence},
		OccurrencePoints: []programschema.OccurrencePoint{basePoint, dispatchPoint, summaryPoint, effectPoint},
		RuleOccurrences:  []programschema.RuleOccurrence{dispatchRule, summaryRule, effectRule},
		Bodies:           []programschema.Body{body},
		Outcomes:         []programschema.Outcome{normal, returned, thrown, yielded, canceled},
		ModuleEntries:    []programschema.ModuleEntry{entry},
	}).Seal(catalogID, store)
	if !ok {
		t.Fatal("program publication")
	}
	program := programschema.Program{
		Frozen: frozen, ArtifactID: boundEdgeLawID(t, "artifact"), ProgramID: boundEdgeLawID(t, "program"), SchemaID: schemaID,
		EntryBodyID: body.ID(),
	}
	sourceMount := programmount.Program{ModuleKey: sourceModule, Program: program}
	targetMount := programmount.Program{ModuleKey: targetModule, Program: program}
	if !sourceMount.Available() || !targetMount.Available() {
		t.Fatal("program mounts")
	}
	return boundEdgeLawProgram{sourceMount: sourceMount, targetMount: targetMount, request: request, importRow: importRow, body: body}
}

func newBoundEdgeSCCFixture(t *testing.T) boundEdgeSCCFixture {
	t.Helper()
	graph, _ := realPlanLawGraph(t)
	leftModule := boundEdgeLawID(t, "module-left")
	rightModule := boundEdgeLawID(t, "module-right")
	program := newBoundEdgeLawProgram(t, leftModule, rightModule)
	link := boundEdgeLawID(t, "link")
	actor := boundEdgeLawID(t, "actor")
	representative := boundEdgeLawID(t, "representative")
	left, leftOK := executioncontext.NewContext(link, leftModule, actor, representative)
	right, rightOK := executioncontext.NewContext(link, rightModule, actor, representative)
	if !leftOK || !rightOK {
		t.Fatal("contexts")
	}
	leftRoot, leftRootOK := executioncontext.NewRootContext(link, boundEdgeLawID(t, "left-root"), left.ID())
	rightRoot, rightRootOK := executioncontext.NewRootContext(link, boundEdgeLawID(t, "right-root"), right.ID())
	if !leftRootOK || !rightRootOK {
		t.Fatal("roots")
	}
	forwardTransition, forwardTransitionOK := executioncontext.NewTransition(link, left.ID(), right.ID())
	reverseTransition, reverseTransitionOK := executioncontext.NewTransition(link, right.ID(), left.ID())
	if !forwardTransitionOK || !reverseTransitionOK {
		t.Fatal("transitions")
	}
	directory, directoryOK := executioncontext.Seal(link, []executioncontext.Context{left, right}, []executioncontext.RootContext{leftRoot, rightRoot}, []executioncontext.Transition{forwardTransition, reverseTransition})
	if !directoryOK {
		t.Fatal("directory")
	}
	owners := make([]contextfiber.PointOwner, 2)
	owners[0], _ = contextfiber.Mounted(leftModule)
	owners[1], _ = contextfiber.Mounted(rightModule)
	index, indexOK := contextfiber.New(directory, len(owners), identity.Generation(1))
	if !indexOK {
		t.Fatal("index")
	}
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, directory, owners, identity.Generation(1), graph)
	if !layoutOK {
		t.Fatal("layout")
	}
	points := [2]equation.Point{}
	for index := range points {
		point, pointOK := graph.PointAt(schedule.Node(index))
		if !pointOK {
			t.Fatalf("point %d", index)
		}
		points[index] = point
	}
	forwardResolved, forwardResolvedOK := modulecomposition.NewResolvedImport(link, program.sourceMount, program.request, program.targetMount.ModuleKey)
	reverseResolved, reverseResolvedOK := modulecomposition.NewResolvedImport(link, program.targetMount, program.request, program.sourceMount.ModuleKey)
	if !forwardResolvedOK || !reverseResolvedOK {
		t.Fatal("resolved imports")
	}
	forwardCache, forwardCacheOK := modulecomposition.NewCacheIngress(forwardResolved, boundEdgeLawID(t, "forward-from-root"), boundEdgeLawID(t, "forward-to-root"), left, right)
	reverseCache, reverseCacheOK := modulecomposition.NewCacheIngress(reverseResolved, boundEdgeLawID(t, "reverse-from-root"), boundEdgeLawID(t, "reverse-to-root"), right, left)
	if !forwardCacheOK || !reverseCacheOK {
		t.Fatal("cache ingress")
	}
	forwardGeneration, forwardGenerationOK := modulecomposition.NewInitGeneration(forwardCache, program.targetMount, program.body)
	reverseGeneration, reverseGenerationOK := modulecomposition.NewInitGeneration(reverseCache, program.sourceMount, program.body)
	if !forwardGenerationOK || !reverseGenerationOK {
		t.Fatal("generations")
	}
	forwardRow, forwardRowOK := modulecomposition.NewModuleCallTransition(forwardCache, forwardGeneration, program.sourceMount, program.importRow, forwardTransition)
	reverseRow, reverseRowOK := modulecomposition.NewModuleCallTransition(reverseCache, reverseGeneration, program.targetMount, program.importRow, reverseTransition)
	if !forwardRowOK || !reverseRowOK {
		t.Fatal("module-call transitions")
	}
	forward, forwardOK := NewBoundEdge(graph, layout, directory, points[0], points[1], boundEdgeSpec(t, directory, forwardRow, forwardGeneration))
	reverse, reverseOK := NewBoundEdge(graph, layout, directory, points[1], points[0], boundEdgeSpec(t, directory, reverseRow, reverseGeneration))
	if !forwardOK || !reverseOK {
		t.Fatal("bound edges")
	}
	return boundEdgeSCCFixture{
		graph: graph, directory: directory, layout: layout, points: points,
		modules: [2]identity.ContentID{leftModule, rightModule}, contexts: [2]executioncontext.Context{left, right},
		forward: forward, reverse: reverse, forwardRow: forwardRow, reverseRow: reverseRow,
		forwardGen: forwardGeneration, reverseGen: reverseGeneration,
	}
}

func planRegionNodes(plan *LinkExecutionPlan, regionIndex int) map[schedule.Node]struct{} {
	region, ok := plan.Schedule().RegionAt(regionIndex)
	if !ok {
		return nil
	}
	nodes := make(map[schedule.Node]struct{})
	for eventIndex := region.Enter; eventIndex <= region.Exit; eventIndex++ {
		event, eventOK := plan.Schedule().EventAt(eventIndex)
		if eventOK && event.Kind == schedule.EventNode {
			nodes[event.Node] = struct{}{}
		}
	}
	return nodes
}

func TestBoundEdgesAloneCreateCrossContextSCC(t *testing.T) {
	fixture := newBoundEdgeSCCFixture(t)
	without, withoutOK := New(fixture.graph, fixture.layout, fixture.directory, nil)
	withForward, withForwardOK := New(fixture.graph, fixture.layout, fixture.directory, []BoundEdge{fixture.forward})
	withCycle, withCycleOK := New(fixture.graph, fixture.layout, fixture.directory, []BoundEdge{fixture.forward, fixture.reverse})
	if !withoutOK || !withForwardOK || !withCycleOK || without == nil || withForward == nil || withCycle == nil {
		t.Fatal("bound-edge plan construction")
	}
	if without.EdgeCount()+1 != withForward.EdgeCount() || withForward.EdgeCount()+1 != withCycle.EdgeCount() {
		t.Fatalf("edge counts without/forward/cycle=%d/%d/%d", without.EdgeCount(), withForward.EdgeCount(), withCycle.EdgeCount())
	}
	if withCycle.StateEdgeCount() != withForward.StateEdgeCount()+1 {
		t.Fatalf("state edge count forward/cycle=%d/%d", withForward.StateEdgeCount(), withCycle.StateEdgeCount())
	}
	forwardState := [2]contextfiber.StateOrdinal{fixture.forward.From(), fixture.forward.To()}
	reverseState := [2]contextfiber.StateOrdinal{fixture.reverse.From(), fixture.reverse.To()}
	if forwardState[0] != reverseState[1] || forwardState[1] != reverseState[0] {
		t.Fatalf("bound state pairs forward=%v reverse=%v", forwardState, reverseState)
	}
	if withForward.Schedule().RegionCount() != without.Schedule().RegionCount() {
		t.Fatalf("one-way edge changed SCC region count without cycle: %d/%d", without.Schedule().RegionCount(), withForward.Schedule().RegionCount())
	}
	cycleRegion := -1
	for _, plan := range []*LinkExecutionPlan{without, withForward} {
		for regionIndex := 0; regionIndex < plan.Schedule().RegionCount(); regionIndex++ {
			nodes := planRegionNodes(plan, regionIndex)
			if _, left := nodes[schedule.Node(forwardState[0])]; left {
				if _, right := nodes[schedule.Node(forwardState[1])]; right {
					t.Fatalf("plan without a bound cycle admitted both state rows in region %d", regionIndex)
				}
			}
		}
	}
	for regionIndex := 0; regionIndex < withCycle.Schedule().RegionCount(); regionIndex++ {
		nodes := planRegionNodes(withCycle, regionIndex)
		if _, left := nodes[schedule.Node(forwardState[0])]; left {
			if _, right := nodes[schedule.Node(forwardState[1])]; right {
				cycleRegion = regionIndex
				break
			}
		}
	}
	if cycleRegion < 0 {
		t.Fatal("authenticated bound edges did not create one cross-context SCC")
	}
	if len(planRegionNodes(withCycle, cycleRegion)) != 2 {
		t.Fatalf("bound-only SCC region nodes=%v, want exactly two state rows", planRegionNodes(withCycle, cycleRegion))
	}
	seenForward, seenReverse := false, false
	for index := 0; index < withCycle.StateEdgeCount(); index++ {
		edge, edgeOK := withCycle.StateEdgeAt(index)
		if !edgeOK {
			t.Fatalf("state edge %d", index)
		}
		switch edge.TransitionID() {
		case fixture.forwardRow.TransitionID():
			seenForward = true
			if edge.From() != fixture.forward.From() || edge.To() != fixture.forward.To() {
				t.Fatal("forward BoundEdge state projection changed")
			}
		case fixture.reverseRow.TransitionID():
			seenReverse = true
			if edge.From() != fixture.reverse.From() || edge.To() != fixture.reverse.To() {
				t.Fatal("reverse BoundEdge state projection changed")
			}
		}
	}
	if !seenForward || !seenReverse {
		t.Fatalf("bound transition metadata forward/reverse=%v/%v", seenForward, seenReverse)
	}
}

func TestBoundEdgeSCCPlanIsPermutationDeterministic(t *testing.T) {
	fixture := newBoundEdgeSCCFixture(t)
	forwardFirst, firstOK := New(fixture.graph, fixture.layout, fixture.directory, []BoundEdge{fixture.forward, fixture.reverse})
	reverseFirst, reverseOK := New(fixture.graph, fixture.layout, fixture.directory, []BoundEdge{fixture.reverse, fixture.forward})
	if !firstOK || !reverseOK || !reflect.DeepEqual(forwardFirst.Edges(), reverseFirst.Edges()) || !reflect.DeepEqual(forwardFirst.StateEdges(), reverseFirst.StateEdges()) {
		t.Fatal("bound-edge order changed the projected schedule")
	}
	for index := 0; index < forwardFirst.Schedule().EventCount(); index++ {
		left, leftOK := forwardFirst.Schedule().EventAt(index)
		right, rightOK := reverseFirst.Schedule().EventAt(index)
		if !leftOK || !rightOK || left != right {
			t.Fatalf("event %d=%+v/%+v", index, left, right)
		}
	}
}

func TestBoundEdgeSCCRejectsUnauthenticatedGeometry(t *testing.T) {
	fixture := newBoundEdgeSCCFixture(t)
	if _, ok := New(fixture.graph, fixture.layout, fixture.directory, []BoundEdge{{}}); ok {
		t.Fatal("zero BoundEdge admitted")
	}
	if _, ok := NewBoundEdge(fixture.graph, fixture.layout, fixture.directory, fixture.points[1], fixture.points[0], boundEdgeSpec(t, fixture.directory, fixture.forwardRow, fixture.forwardGen)); ok {
		t.Fatal("swapped endpoint BoundEdge admitted")
	}
	foreignGraph, _ := realPlanLawGraph(t)
	foreignLayout := newPlanLawLayout(t, foreignGraph, fixture.directory, []contextfiber.PointOwner{
		mustMounted(t, fixture.modules[0]), mustMounted(t, fixture.modules[1]),
	})
	if _, ok := New(foreignGraph, foreignLayout, fixture.directory, []BoundEdge{fixture.forward, fixture.reverse}); ok {
		t.Fatal("BoundEdges crossed an exact graph/layout fence")
	}
	foreignDirectory := newPlanLawDirectory(t, "foreign-left", "foreign-right")
	foreignOwners := []contextfiber.PointOwner{mustMounted(t, foreignDirectory.modules[0]), mustMounted(t, foreignDirectory.modules[1])}
	foreignLayout = newPlanLawLayout(t, fixture.graph, foreignDirectory.directory, foreignOwners)
	if _, ok := New(fixture.graph, foreignLayout, foreignDirectory.directory, []BoundEdge{fixture.forward, fixture.reverse}); ok {
		t.Fatal("BoundEdges crossed an exact directory fence")
	}
}

func mustMounted(t *testing.T, module identity.ContentID) contextfiber.PointOwner {
	t.Helper()
	owner, ok := contextfiber.Mounted(module)
	if !ok {
		t.Fatal("mounted owner")
	}
	return owner
}
