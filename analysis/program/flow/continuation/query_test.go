package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/position"
	"github.com/wippyai/go-lua/analysis/program/flow/runtimeentry"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

func continuationIDs() (identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID) {
	return flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)
}

func emptyContinuationResult() *Result {
	call := keyspace.FamilyCall
	cellRoots := [keyspace.FamilyCount][]uint32{}
	guardRoots := [keyspace.FamilyCount][]uint32{}
	var counts [keyspace.FamilyCount]uint32
	counts[call] = 1
	cellRoots[call] = []uint32{absentRoot, 0}
	guardRoots[call] = []uint32{absentRoot, 0}
	cellRecords := [keyspace.FamilyCount][]cellRootRecord{}
	guardRecords := [keyspace.FamilyCount][]guardRootRecord{}
	cellRecords[call] = []cellRootRecord{{}, {root: 0, present: true}}
	guardRecords[call] = []guardRootRecord{{}, {root: 0, present: true}}
	sourceID, flowID, staticID, moduleID := continuationIDs()
	return &Result{
		sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID,
		cells:  cellProjection{roots: cellRoots, records: cellRecords, nodes: []scopeNode{{}}, terms: nil, counts: counts},
		guards: guardProjection{roots: guardRoots, records: guardRecords, nodes: []guardNode{{}}, counts: counts},
	}
}

func TestContinuationQueryLaws(t *testing.T) {
	result := emptyContinuationResult()
	sourceID, flowID, staticID, moduleID := continuationIDs()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("exact four-owner provenance did not match")
	}
	ids := []identity.ContentID{sourceID, flowID, staticID, moduleID}
	for index := range ids {
		foreign := ids[index]
		foreign[0]++
		candidate := []identity.ContentID{sourceID, flowID, staticID, moduleID}
		candidate[index] = foreign
		if Matches(result, candidate[0], candidate[1], candidate[2], candidate[3]) {
			t.Fatalf("foreign owner %d matched", index)
		}
	}
	if Matches(result, identity.ContentID{}, flowID, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) {
		t.Fatal("unavailable owner matched")
	}
	if count, ok := result.CellCount(call); !ok || count != 0 {
		t.Fatalf("empty Cell root = %d/%v", count, ok)
	}
	if count, ok := result.GuardCount(call); !ok || count != 0 {
		t.Fatalf("empty Guard root = %d/%v", count, ok)
	}
	if _, ok := result.CellAt(call, 0); ok {
		t.Fatal("empty Cell root exposed an element")
	}
	if _, ok := result.GuardAt(call, 0); ok {
		t.Fatal("empty Guard root exposed an element")
	}
	for _, term := range []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyCall, 2),
		0,
	} {
		if _, ok := result.CellCount(term); ok {
			t.Fatalf("non-subject Cell %08x matched", uint32(term))
		}
		if _, ok := result.GuardCount(term); ok {
			t.Fatalf("non-subject Guard %08x matched", uint32(term))
		}
	}
}

func TestContinuationQueriesRejectMalformedRoots(t *testing.T) {
	result := emptyContinuationResult()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	result.cells.roots[keyspace.FamilyCall] = []uint32{absentRoot, 0, 0}
	if _, ok := result.CellCount(call); ok {
		t.Fatal("CellCount accepted an overlong subject root plane")
	}

	result = emptyContinuationResult()
	result.guards.roots[keyspace.FamilyCall] = []uint32{absentRoot, 0, 0}
	if _, ok := result.GuardCount(call); ok {
		t.Fatal("GuardCount accepted an overlong subject root plane")
	}

	result = emptyContinuationResult()
	result.cells.nodes = []scopeNode{{total: 1}}
	if _, ok := result.CellCount(call); ok {
		t.Fatal("malformed Cell sentinel was accepted")
	}

	result = emptyContinuationResult()
	result.guards.nodes = []guardNode{{}, {count: 0, term: keyspace.MakeTerm(keyspace.FamilySelect, 1)}}
	result.guards.roots[keyspace.FamilyCall][1] = 1
	if _, ok := result.GuardCount(call); ok {
		t.Fatal("malformed Guard leaf was accepted")
	}

	result = emptyContinuationResult()
	result.guards.nodes = []guardNode{{}, {prev: 0, jump: 0, count: 1, term: keyspace.MakeTerm(keyspace.FamilySelect, 1)}, {prev: 0, jump: 0, count: 2, term: keyspace.MakeTerm(keyspace.FamilySelect, 2)}}
	result.guards.roots[keyspace.FamilyCall][1] = 2
	if _, ok := result.GuardCount(call); ok {
		t.Fatal("malformed Guard count was accepted")
	}
}

func TestContinuationQueriesDeepWideAndAllocationFree(t *testing.T) {
	result := emptyContinuationResult()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	terms := make([]keyspace.Term, 1024)
	nodes := make([]scopeNode, len(terms)+1)
	for index := range terms {
		terms[index] = keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
		nodes[index+1] = scopeNode{
			parent: uint32(index), start: uint32(index), count: 1, total: uint32(index + 1),
		}
	}
	result.cells.nodes = nodes
	result.cells.terms = terms
	result.cells.counts[keyspace.FamilyCell] = uint32(len(terms))
	result.cells.roots[keyspace.FamilyCall] = []uint32{absentRoot, uint32(len(nodes) - 1)}
	result.cells.records[keyspace.FamilyCall] = []cellRootRecord{{}, {root: uint32(len(nodes) - 1), count: uint32(len(terms)), present: true, node: nodes[len(nodes)-1]}}
	if count, ok := result.CellCount(call); !ok || count != len(terms) {
		t.Fatalf("deep CellCount = %d/%v, want %d/true", count, ok, len(terms))
	}
	for index, want := range []keyspace.Term{terms[len(terms)-1], terms[0]} {
		queryIndex := 0
		if index == 1 {
			queryIndex = len(terms) - 1
		}
		got, ok := result.CellAt(call, queryIndex)
		if !ok || got != want {
			t.Fatalf("deep CellAt(%d) = %08x/%v, want %08x/true", queryIndex, uint32(got), ok, uint32(want))
		}
	}
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = result.CellCount(call)
		_, _ = result.CellAt(call, len(terms)-1)
		_, _ = result.GuardCount(keyspace.MakeTerm(keyspace.FamilyCall, 1))
	})
	if allocs != 0 {
		t.Fatalf("continuation queries allocated %v objects per run", allocs)
	}
}

type continuationFixture struct {
	sourceView source.View
	flow       authored.View
	bodies     *body.Result
	binding    binding.Result
	forest     *containment.Result
	executable *executable.Result
	candidates *candidates.Result
	causal     *causal.Result
	result     *Result
	staticID   identity.ContentID
	moduleID   identity.ContentID

	flowFinalize authored.Finalizer
}

type continuationSpec struct {
	name       string
	counts     [keyspace.FamilyCount]uint32
	rows       [][]keyspace.Term
	flow       authored.Input
	static     static.Input
	binds      []source.BindCells
	forms      []source.FunctionFormals
	nilOwners  []keyspace.Term
	boolOwners []keyspace.Term
	intOwners  []keyspace.Term
	keys       []source.KeyInput
	exactAtoms []keyspace.LiteralValue
}

func openContinuationFixture(t *testing.T, spec continuationSpec) *continuationFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 || len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatal("continuation fixture requires one Source row per Body")
	}
	sourceDraft, err := source.Build(continuationSourceInput(spec))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := spec.static
	staticInput.Counts = [keyspace.FamilyCount]uint32{}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		staticInput.Counts[family] = spec.counts[family]
	}
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
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if len(staticInput.Contracts.Call) == 0 && spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]staticcontracts.CallContract, spec.counts[keyspace.FamilyCall])
	}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticInput.Counts[keyspace.FamilyCall] = uint32(len(staticInput.Contracts.Call))
	_, staticView, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}

	flowInput := spec.flow
	flowInput.Counts = spec.counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, authored.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, authored.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	moduleView := flowView.Imports()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("binding.Seal: %v", err)
	}

	staticID, moduleID := staticView.ContentID(), flowView.ModuleID()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	ports, err := evaluation.SealPorts(preimage.Identity(), flowView, forest, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("evaluation.SealPorts: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinalize.Commit(indexInput)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()

	controlResult, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	cellRoles := sourceView.CellRoles()
	if !cellRoles.Matches(sourceView) {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatal("source.CellRoles: unavailable")
	}
	certificate, certificateErr := semanticpath.Seal(cellRoles, sourceView, flowView, bodies, bindingResult, forest, outcomes, flowView.ContentID(), staticID, moduleID)
	if certificateErr != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("semanticpath.Seal: %v", certificateErr)
	}
	vertexPaths, pathsOK := certificate.VertexCatalog(sourceView.Identity().ContentID(), flowView.ContentID(), staticID, moduleID)
	vertexLease, vertexErr := controlResult.InstallVertexCatalogLease(bodies, vertexPaths)
	if !pathsOK || vertexErr != nil || vertexLease == nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatal("sourcecontrol.InstallVertexCatalog: no exact path view")
	}
	defer controlResult.ReleaseVertexCatalog(vertexLease)
	outcomePaths, outcomePathsOK := certificate.OutcomePhases(sourceView.Identity().ContentID(), flowView.ContentID(), staticID, moduleID)
	outcomePhases, outcomeErr := controlResult.BuildOutcomePhases(sourceView, flowView, bodies, outcomes, outcomePaths)
	if !outcomePathsOK || outcomeErr != nil || outcomePhases == nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatal("sourcecontrol.BuildOutcomePhases: unavailable")
	}
	executableResult, err := executable.Seal(sourceView, flowView, bodies, forest, controlResult, staticID, moduleID, certificate)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("executable.Seal: %v", err)
	}
	entries, err := runtimeentry.Seal(sourceView, flowView, controlResult, ports, executableResult, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("runtimeentry.Seal: %v", err)
	}
	causalPaths, pathsOK := certificate.Causal(sourceView.Identity().ContentID(), flowView.ContentID(), staticID, moduleID)
	if !pathsOK {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatal("semanticpath.Causal: view unavailable")
	}
	preparation, err := causal.PrepareRoutePlanWithStructuralPaths(sourceView, flowView, bodies, forest, outcomes, controlResult, ports, executableResult, entries, causalPaths, outcomePhases, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("causal.PrepareRoutePlan: %v", err)
	}
	causalResult, err := preparation.Seal()
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("causal.Preparation.Seal: %v", err)
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), flowView, executableResult, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("candidates.Seal: %v", err)
	}
	result, err := Seal(sourceView, flowView, bodies, bindingResult, executableResult, candidateResult, causalResult, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, flowFinalize)
		t.Fatalf("continuation.Seal: %v", err)
	}
	fixture := &continuationFixture{
		sourceView: sourceView, flow: flowView, bodies: bodies, binding: bindingResult,
		forest: forest, executable: executableResult, candidates: candidateResult,
		causal: causalResult, result: result, staticID: staticID, moduleID: moduleID,
		flowFinalize: flowFinalize,
	}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(source.Finalizer{}, fixture.flowFinalize)
	})
	return fixture
}

func continuationSourceInput(spec continuationSpec) source.Input {
	name := spec.name
	if name == "" {
		name = "continuation-semantic.lua"
	}
	input := source.Input{Name: name, Keys: append([]source.KeyInput(nil), spec.keys...), ExactAtoms: append([]keyspace.LiteralValue(nil), spec.exactAtoms...)}
	input.Families = flowtest.FamilySpans(name, spec.counts)
	input.Bodies = make([]source.BodySource, len(spec.rows))
	for index, terms := range spec.rows {
		input.Bodies[index] = source.BodySource{Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), terms...)}
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
	input.Nil = flowtest.LiteralRows(spec.counts[keyspace.FamilyNil], spec.nilOwners, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, _ uint32) source.NilLiteral {
		return source.NilLiteral{Owner: owner}
	})
	input.Bool = flowtest.LiteralRows(spec.counts[keyspace.FamilyBool], spec.boolOwners, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, ordinal uint32) source.BoolLiteral {
		return source.BoolLiteral{Owner: owner, Value: ordinal&1 == 1}
	})
	input.Integer = flowtest.LiteralRows(spec.counts[keyspace.FamilyInteger], spec.intOwners, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, ordinal uint32) source.IntegerLiteral {
		return source.IntegerLiteral{Owner: owner, Value: int64(ordinal)}
	})
	return input
}

func testContinuationCounts(rows ...struct {
	family keyspace.Family
	count  uint32
}) (counts [keyspace.FamilyCount]uint32) {
	for _, row := range rows {
		counts[row.family] = row.count
	}
	return counts
}

func continuationTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	term := keyspace.MakeTerm(family, ordinal)
	if term == 0 {
		panic("continuation fixture term outside family")
	}
	return term
}

func familyCount(family keyspace.Family, count uint32) struct {
	family keyspace.Family
	count  uint32
} {
	return struct {
		family keyspace.Family
		count  uint32
	}{family: family, count: count}
}

func directContinuationSpec(name string) continuationSpec {
	body := continuationTerm(keyspace.FamilyBody, 1)
	call := continuationTerm(keyspace.FamilyCall, 1)
	values := continuationTerm(keyspace.FamilyValues, 1)
	nilValue := continuationTerm(keyspace.FamilyNil, 1)
	return continuationSpec{
		name: name,
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 1), familyCount(keyspace.FamilyCall, 1),
			familyCount(keyspace.FamilyValues, 1), familyCount(keyspace.FamilyNil, 1),
		),
		rows:      [][]keyspace.Term{{call}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Calls:  []authored.Call{{Owner: body, Callee: nilValue, Actuals: values}},
		},
	}
}

func continuationTerminalOnlySpec() continuationSpec {
	return continuationSpec{
		name:   "continuation-terminal-only.lua",
		counts: testContinuationCounts(familyCount(keyspace.FamilyBody, 1)),
		rows:   [][]keyspace.Term{nil},
	}
}
