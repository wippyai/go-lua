package causal

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/recurrence"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/runtimeentry"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// causalFixture is assembled through the canonical upstream owners.  The
// causal tests deliberately retain no construction authority and never
// manufacture a Result, Edge, or CallBoundary row.
type causalFixture struct {
	sourceView source.View
	flow       authored.View
	bodies     *body.Result
	forest     *containment.Result
	outcomes   *outcome.Result
	control    *sourcecontrol.Result
	recurrence *recurrence.Result
	ports      *evaluation.Ports
	executable *executable.Result
	entries    *runtimeentry.Result
	result     *Result

	// These path views are captured while the SourceControl VertexCatalog
	// lease is live.  Semantic-matrix laws must not reopen it after release.
	capturedBodyEntryPath identity.ContentID
	capturedBodyTailPath  identity.ContentID
	capturedVertexPath    identity.ContentID
	capturedArcs          []causalArcCapture

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type causalSpec struct {
	name        string
	counts      [keyspace.FamilyCount]uint32
	rows        [][]keyspace.Term
	flow        authored.Input
	static      static.Input
	binds       []source.BindCells
	forms       []source.FunctionFormals
	nilOwners   []keyspace.Term
	boolOwners  []keyspace.Term
	intOwners   []keyspace.Term
	floatOwners []keyspace.Term
	floatBits   []uint64
	keys        []source.KeyInput
	exactAtoms  []keyspace.LiteralValue

	// Test-only exact point paths to capture before the catalog lease is
	// released. Zero leaves the corresponding path unspecified.
	captureBodyEntryPath keyspace.Term
	captureBodyTailPath  keyspace.Term
	captureVertexPath    keyspace.Term
	captureArcs          []causalArcSelector
	runtimeEntryProbe    func(*runtimeEntryFixture)
}

type runtimeEntryFixture struct {
	sourceView source.View
	flow       authored.View
	outcomes   *outcome.Result
	control    *sourcecontrol.Result
	ports      *evaluation.Ports
	executable *executable.Result
	entries    *runtimeentry.Result
	staticID   identity.ContentID
	moduleID   identity.ContentID
}

// causalArcSelector identifies one immutable structural witness to retain for
// a law after the transient VertexCatalog has been released.
type causalArcSelector struct {
	Source   keyspace.Term
	Target   keyspace.Term
	Decision keyspace.Term
	Truth    bool
}

// causalArcCapture is a test-only scalar snapshot. It deliberately retains
// neither an ArcRef nor any SourceControl lease/result authority.
type causalArcCapture struct {
	arc     sourcecontrol.Arc
	ordinal int
}

func openCausalFixture(t *testing.T, spec causalSpec) *causalFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 || len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatal("causal fixture requires one Source row per Body")
	}

	sourceDraft, err := source.Build(causalSourceInput(spec))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := spec.static
	// Static has its own denominator.  A semantic fixture with no static
	// expressions still needs one Body row and empty Function/Call contracts.
	staticInput.Counts = [keyspace.FamilyCount]uint32{}
	staticInput.Counts[keyspace.FamilyBody] = spec.counts[keyspace.FamilyBody]
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	staticInput.Counts[keyspace.FamilyTypeLiteral] = uint32(len(staticInput.Types.Literal))
	staticInput.Counts[keyspace.FamilyTypeOptional] = uint32(len(staticInput.Types.Optional))
	staticInput.Counts[keyspace.FamilyTypeUnion] = uint32(len(staticInput.Types.Union))
	staticInput.Counts[keyspace.FamilyTypeIntersection] = uint32(len(staticInput.Types.Intersection))
	staticInput.Counts[keyspace.FamilyTypeGeneric] = uint32(len(staticInput.Types.Generic))
	staticInput.Counts[keyspace.FamilyTypeArray] = uint32(len(staticInput.Types.Array))
	staticInput.Counts[keyspace.FamilyTypeMap] = uint32(len(staticInput.Types.Map))
	staticInput.Counts[keyspace.FamilyTypeRecord] = uint32(len(staticInput.Types.Record))
	staticInput.Counts[keyspace.FamilyTypeField] = uint32(len(staticInput.Types.Field))
	staticInput.Counts[keyspace.FamilyTypeAlias] = uint32(len(staticInput.Declarations.Alias))
	staticInput.Counts[keyspace.FamilyTypeInterface] = uint32(len(staticInput.Declarations.Interface))
	staticInput.Counts[keyspace.FamilyTypeParam] = uint32(len(staticInput.Declarations.TypeParam))
	staticInput.Counts[keyspace.FamilyDeclaredType] = uint32(len(staticInput.Declarations.DeclaredType))
	staticInput.Counts[keyspace.FamilyTypeFunction] = uint32(len(staticInput.Signatures.TypeFunction))
	staticInput.Counts[keyspace.FamilyTypeAsserts] = uint32(len(staticInput.Signatures.TypeAsserts))
	staticInput.Counts[keyspace.FamilyTypeOf] = uint32(len(staticInput.Operators.TypeOf))
	staticInput.Counts[keyspace.FamilyAnnotation] = uint32(len(staticInput.Operands.Annotation))
	if len(staticInput.Contracts.Function) == 0 && spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]static.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if len(staticInput.Contracts.Call) == 0 && spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]static.CallContract, spec.counts[keyspace.FamilyCall])
	}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticInput.Counts[keyspace.FamilyCall] = uint32(len(staticInput.Contracts.Call))
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()

	flowInput := spec.flow
	flowInput.Counts = spec.counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	ports, err := evaluation.SealPorts(preimage.Identity(), flowView, forest, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("evaluation.SealPorts: %v", err)
	}

	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, issuance, err := sourceFinalize.CommitWithSemanticPathIssuance(indexInput)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()

	graph, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	cellRoles := sourceView.CellRoles()
	if !cellRoles.Matches(sourceView) {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("source.CellRoles: unavailable")
	}
	certificate, certificateErr := semanticpath.Seal(issuance, cellRoles, sourceView, flowView, bodies, bindingResult, forest, outcomes, flowView.Cold().ContentID(), staticID, moduleID)
	if certificateErr != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("semanticpath.Seal: %v", certificateErr)
	}
	vertexPaths, pathsOK := certificate.VertexCatalog(sourceView.Identity().ContentID(), flowView.Cold().ContentID(), staticID, moduleID)
	vertexLease, vertexErr := graph.InstallVertexCatalogLease(bodies, vertexPaths)
	if !pathsOK || vertexErr != nil || vertexLease == nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("sourcecontrol.InstallVertexCatalog: no exact path view")
	}
	defer graph.ReleaseVertexCatalog(vertexLease)
	capturedBodyEntryPath, capturedBodyTailPath, capturedVertexPath := identity.ContentID{}, identity.ContentID{}, identity.ContentID{}
	if spec.captureBodyEntryPath != 0 {
		var pathOK bool
		capturedBodyEntryPath, pathOK = graph.BodyEntryPath(spec.captureBodyEntryPath)
		if !pathOK {
			closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
			t.Fatal("sourcecontrol.BodyEntryPath: unavailable while catalog lease is live")
		}
	}
	if spec.captureBodyTailPath != 0 {
		var pathOK bool
		capturedBodyTailPath, pathOK = graph.BodyTailPath(spec.captureBodyTailPath)
		if !pathOK {
			closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
			t.Fatal("sourcecontrol.BodyTailPath: unavailable while catalog lease is live")
		}
	}
	if spec.captureVertexPath != 0 {
		ref, refOK := graph.CoordinateRef(sourceView, spec.captureVertexPath)
		var pathOK bool
		if refOK {
			capturedVertexPath, pathOK = graph.VertexPath(ref)
		}
		if !pathOK {
			closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
			t.Fatal("sourcecontrol.VertexPath: unavailable while catalog lease is live")
		}
	}
	capturedArcs := make([]causalArcCapture, len(spec.captureArcs))
	if len(spec.captureArcs) != 0 {
		captureIndex := make(map[causalArcSelector]int, len(spec.captureArcs))
		for index, selector := range spec.captureArcs {
			if selector.Source == 0 || selector.Target == 0 {
				closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
				t.Fatal("causal fixture Arc capture selector is malformed")
			}
			if _, duplicate := captureIndex[selector]; duplicate {
				closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
				t.Fatal("causal fixture Arc capture selector is duplicated")
			}
			captureIndex[selector] = index
			capturedArcs[index].ordinal = -1
		}
		for ordinal := 0; ordinal < graph.ArcCount(); ordinal++ {
			arc, arcOK := graph.ArcAt(ordinal)
			if !arcOK {
				closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
				t.Fatal("causal fixture Arc denominator is unavailable while catalog lease is live")
			}
			selector := causalArcSelector{Source: arc.Source, Target: arc.Target, Decision: arc.Decision, Truth: arc.Truth}
			index, requested := captureIndex[selector]
			if !requested {
				continue
			}
			if capturedArcs[index].ordinal >= 0 {
				closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
				t.Fatal("causal fixture Arc capture selector is ambiguous")
			}
			capturedArcs[index] = causalArcCapture{arc: arc, ordinal: ordinal}
		}
		for index, capture := range capturedArcs {
			if capture.ordinal < 0 {
				closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
				t.Fatalf("causal fixture Arc capture %d is absent", index)
			}
		}
	}
	outcomePaths, outcomePathsOK := certificate.OutcomePhases(sourceView.Identity().ContentID(), flowView.Cold().ContentID(), staticID, moduleID)
	outcomePhases, outcomeErr := graph.BuildOutcomePhases(sourceView, flowView, bodies, outcomes, outcomePaths)
	if !outcomePathsOK || outcomeErr != nil || outcomePhases == nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("sourcecontrol.BuildOutcomePhases: unavailable")
	}
	execResult, err := executable.Seal(sourceView, flowView, forest, graph, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("executable.Seal: %v", err)
	}
	entries, err := runtimeentry.Seal(sourceView, flowView, graph, ports, execResult, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("runtimeentry.Seal: %v", err)
	}
	if spec.runtimeEntryProbe != nil {
		spec.runtimeEntryProbe(&runtimeEntryFixture{sourceView: sourceView, flow: flowView, outcomes: outcomes,
			control: graph, ports: ports, executable: execResult, entries: entries, staticID: staticID, moduleID: moduleID})
	}
	causalPaths, pathsOK := certificate.Causal(sourceView.Identity().ContentID(), flowView.Cold().ContentID(), staticID, moduleID)
	if !pathsOK {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("semanticpath.Causal: view unavailable")
	}
	preparation, err := PrepareRoutePlanWithStructuralPaths(sourceView, flowView, bodies, forest, outcomes, graph, ports, execResult, entries, causalPaths, outcomePhases, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("causal.PrepareRoutePlan: %v", err)
	}
	preparationCopy := *preparation
	result, err := preparation.Seal()
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("causal.Preparation.Seal: %v", err)
	}
	if _, err := preparationCopy.Seal(); err == nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("copied causal Preparation sealed a second transaction")
	}
	// The finished Causal authority retains no recurrence result. This fixture
	// separately seals recurrence only for diagnostic laws that inspect it.
	recur, err := recurrence.Seal(sourceView, flowView, bodies, forest, graph, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("recurrence.Seal: %v", err)
	}

	fixture := &causalFixture{
		sourceView: sourceView, flow: flowView, bodies: bodies, forest: forest,
		outcomes: outcomes, control: graph, recurrence: recur, ports: ports,
		executable: execResult, entries: entries, result: result,
		capturedBodyEntryPath: capturedBodyEntryPath, capturedBodyTailPath: capturedBodyTailPath,
		capturedVertexPath: capturedVertexPath, capturedArcs: capturedArcs,
		staticFinalize: staticFinalize, flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closeCausalFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeCausalFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize imports.Finalizer) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func causalSourceInput(spec causalSpec) source.Input {
	name := spec.name
	if name == "" {
		name = "causal-semantic.lua"
	}
	input := source.Input{Name: name}
	input.Keys = append([]source.KeyInput(nil), spec.keys...)
	input.ExactAtoms = append([]keyspace.LiteralValue(nil), spec.exactAtoms...)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, spec.counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(spec.rows))
	for index, terms := range spec.rows {
		input.Bodies[index] = source.BodySource{
			Body:  keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)),
			Terms: append([]keyspace.Term(nil), terms...),
		}
	}
	input.Binds = make([]source.BindCells, spec.counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		if index < len(spec.binds) {
			input.Binds[index].Cells = append([]keyspace.Term(nil), spec.binds[index].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, spec.counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		if index < len(spec.forms) {
			input.Functions[index].Formals = append([]keyspace.Term(nil), spec.forms[index].Formals...)
		}
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyNil]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.nilOwners) {
			owner = spec.nilOwners[ordinal-1]
		}
		input.Nil = append(input.Nil, source.NilLiteral{Owner: owner})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyBool]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.boolOwners) {
			owner = spec.boolOwners[ordinal-1]
		}
		input.Bool = append(input.Bool, source.BoolLiteral{Owner: owner, Value: ordinal&1 == 1})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyInteger]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.intOwners) {
			owner = spec.intOwners[ordinal-1]
		}
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: owner, Value: int64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyFloat]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.floatOwners) {
			owner = spec.floatOwners[ordinal-1]
		}
		bits := math.Float64bits(float64(ordinal))
		if int(ordinal) <= len(spec.floatBits) {
			bits = spec.floatBits[ordinal-1]
		}
		input.Float = append(input.Float, source.FloatLiteral{Owner: owner, Bits: bits})
	}
	return input
}

func causalTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	term := keyspace.MakeTerm(family, ordinal)
	if term == 0 {
		panic("causal fixture term outside family")
	}
	return term
}

type causalFamilyCount struct {
	family keyspace.Family
	count  uint32
}

func causalCounts(rows ...causalFamilyCount) (counts [keyspace.FamilyCount]uint32) {
	for _, row := range rows {
		counts[row.family] = row.count
	}
	return counts
}
