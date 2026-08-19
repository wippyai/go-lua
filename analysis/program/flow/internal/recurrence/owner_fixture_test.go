package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

// ownerFixture is deliberately assembled through the same sealed owners as
// production. Recurrence tests may not manufacture a graph or bypass Source
// positions: doing so would only qualify a second topology.
type ownerFixture struct {
	sourceView source.View
	flow       authored.View
	bodies     *body.Result
	forest     *containment.Result
	graph      *sourcecontrol.Result
	graphLease *sourcecontrol.VertexCatalogLease

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type ownerSpec struct {
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
}

func openOwnerFixture(t *testing.T, spec ownerSpec) *ownerFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 || len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatal("owner fixture requires one Source row per Body")
	}
	sourceDraft, err := source.Build(ownerSourceInput(spec))
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
	staticInput.Counts[keyspace.FamilyBody] = spec.counts[keyspace.FamilyBody]
	// TypeOf and Annotation are cross-owner rows.  Their scope, operand, and
	// Values references are still validated by Static.Build, so this fixture
	// must expose the corresponding Source/Flow denominators instead of
	// silently leaving those families at zero.
	for _, family := range []keyspace.Family{
		keyspace.FamilyCell, keyspace.FamilyValues, keyspace.FamilyFunction,
		keyspace.FamilyCall, keyspace.FamilyValueClaim,
	} {
		staticInput.Counts[family] = spec.counts[family]
	}
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	staticInput.Counts[keyspace.FamilyTypeAlias] = uint32(len(staticInput.Declarations.Alias))
	staticInput.Counts[keyspace.FamilyDeclaredType] = uint32(len(staticInput.Declarations.DeclaredType))
	staticInput.Counts[keyspace.FamilyAnnotation] = uint32(len(staticInput.Operands.Annotation))
	staticInput.Counts[keyspace.FamilyTypeOf] = uint32(len(staticInput.Operators.TypeOf))
	if len(staticInput.Contracts.Function) == 0 && spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if len(staticInput.Contracts.Call) == 0 && spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]staticcontracts.CallContract, spec.counts[keyspace.FamilyCall])
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

	flowInput := spec.flow
	flowInput.Counts = spec.counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := term(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticFinalize.View(), entry)
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	forest, _, err := containment.Prove(preimage, staticFinalize.View(), flowView, bodies, bindingResult, moduleFinalize.View(), entry)
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		closeOwnerFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, issuance, err := sourceFinalize.CommitWithSemanticPathIssuance(indexInput)
	if err != nil {
		closeOwnerFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	graph, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		closeOwnerFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	cellRoles := sourceView.CellRoles()
	if !cellRoles.Matches(sourceView) {
		closeOwnerFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("source.CellRoles: unavailable")
	}
	certificate, err := semanticpath.Seal(issuance, cellRoles, sourceView, flowView, bodies, bindingResult, forest, outcomes,
		flowView.Cold().ContentID(), staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		closeOwnerFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("semanticpath.Seal: %v", err)
	}
	vertexPaths, pathsOK := certificate.VertexCatalog(sourceView.Identity().ContentID(), flowView.Cold().ContentID(),
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	vertexLease, err := graph.InstallVertexCatalogLease(bodies, vertexPaths)
	if !pathsOK || err != nil || vertexLease == nil {
		closeOwnerFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatal("sourcecontrol.InstallVertexCatalog: no exact path view")
	}
	fixture := &ownerFixture{
		sourceView:     sourceView,
		flow:           flowView,
		bodies:         bodies,
		forest:         forest,
		graph:          graph,
		graphLease:     vertexLease,
		staticFinalize: staticFinalize,
		flowFinalize:   flowFinalize,
		moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		fixture.graph.ReleaseVertexCatalog(fixture.graphLease)
		closeOwnerFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeOwnerFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize imports.Finalizer) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func ownerSourceInput(spec ownerSpec) source.Input {
	name := spec.name
	if name == "" {
		name = "recurrence-owner.lua"
	}
	input := source.Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, spec.counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(spec.rows))
	for index, rows := range spec.rows {
		input.Bodies[index] = source.BodySource{Body: term(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), rows...)}
	}
	input.Binds = make([]source.BindCells, spec.counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = term(keyspace.FamilyBind, uint32(index+1))
		if index < len(spec.binds) {
			input.Binds[index].Cells = append([]keyspace.Term(nil), spec.binds[index].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, spec.counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = term(keyspace.FamilyFunction, uint32(index+1))
		if index < len(spec.forms) {
			input.Functions[index].Formals = append([]keyspace.Term(nil), spec.forms[index].Formals...)
		}
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyNil]; ordinal++ {
		owner := term(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.nilOwners) {
			owner = spec.nilOwners[ordinal-1]
		}
		input.Nil = append(input.Nil, source.NilLiteral{Owner: owner})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyBool]; ordinal++ {
		owner := term(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.boolOwners) {
			owner = spec.boolOwners[ordinal-1]
		}
		input.Bool = append(input.Bool, source.BoolLiteral{Owner: owner, Value: ordinal&1 == 1})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyInteger]; ordinal++ {
		owner := term(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.intOwners) {
			owner = spec.intOwners[ordinal-1]
		}
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: owner, Value: int64(ordinal)})
	}
	return input
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

type ownerFamilyCount struct {
	family keyspace.Family
	count  uint32
}

func countsWith(rows ...ownerFamilyCount) (counts [keyspace.FamilyCount]uint32) {
	for _, row := range rows {
		counts[row.family] = row.count
	}
	return counts
}

func loopTerms() (loops [4]keyspace.Term) {
	for index := range loops {
		loops[index] = term(keyspace.FamilyLoop, uint32(index+1))
	}
	return loops
}

func familyCount(family keyspace.Family, count uint32) ownerFamilyCount {
	return ownerFamilyCount{family: family, count: count}
}
