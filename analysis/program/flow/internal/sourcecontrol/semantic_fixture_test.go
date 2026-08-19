package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

// semanticFixture is assembled through every owner that contributes to
// source-control geometry. No fixture imports a helper from another package.
type semanticFixture struct {
	sourceView source.View
	flow       authored.View
	bodies     *body.Result
	forest     *containment.Result
	shape      *control.Shape
	result     *Result

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type semanticSpec struct {
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

func openSemanticFixture(t *testing.T, spec semanticSpec) *semanticFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 || len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatal("semantic fixture requires one Source row per Body")
	}

	sourceDraft, err := source.Build(semanticSourceInput(spec))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := spec.static
	// Static owns a distinct denominator from Flow. Only the static rows
	// authored by a case enter this plan; copying Flow's counts would falsely
	// claim every Bind/Loop/Body family as a Static relation.
	staticInput.Counts = [keyspace.FamilyCount]uint32{}
	staticInput.Counts[keyspace.FamilyBody] = spec.counts[keyspace.FamilyBody]
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	staticInput.Counts[keyspace.FamilyTypeAlias] = uint32(len(staticInput.Declarations.Alias))
	// Static carries one empty contract sidecar row for every Flow Function
	// and Call, even when the case has no static type arguments.
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
	staticView := staticFinalize.View()

	flowInput := spec.flow
	flowInput.Counts = spec.counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := term(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSemanticFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinalize.Commit(indexInput)
	if err != nil {
		closeSemanticFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()

	result, err := Seal(sourceView, flowView, bodies, forest, shape, entry,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeSemanticFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	fixture := &semanticFixture{
		sourceView:     sourceView,
		flow:           flowView,
		bodies:         bodies,
		forest:         forest,
		shape:          shape,
		result:         result,
		staticFinalize: staticFinalize,
		flowFinalize:   flowFinalize,
		moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closeSemanticFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeSemanticFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize imports.Finalizer) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func semanticSourceInput(spec semanticSpec) source.Input {
	name := spec.name
	if name == "" {
		name = "source-control-semantic.lua"
	}
	input := source.Input{Name: name}
	input.ExactAtoms = append([]keyspace.LiteralValue(nil), spec.exactAtoms...)
	input.Keys = append([]source.KeyInput(nil), spec.keys...)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, spec.counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(spec.rows))
	for index, terms := range spec.rows {
		input.Bodies[index] = source.BodySource{Body: term(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), terms...)}
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
	value := keyspace.MakeTerm(family, ordinal)
	if value == 0 {
		panic("semantic fixture term outside family")
	}
	return value
}

type familyCount struct {
	family keyspace.Family
	count  uint32
}

func countsWith(rows ...familyCount) (counts [keyspace.FamilyCount]uint32) {
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

func assertArc(t *testing.T, result *Result, from, to uint32, sourceTerm, targetTerm, decision keyspace.Term, truth bool) {
	t.Helper()
	want := Arc{From: from, To: to, Source: sourceTerm, Target: targetTerm, Decision: decision, Truth: truth}
	for index := 0; index < result.ArcCount(); index++ {
		got, ok := result.ArcAt(index)
		if !ok {
			t.Fatalf("ArcAt(%d) failed within ArcCount=%d", index, result.ArcCount())
		}
		if got == want {
			return
		}
	}
	t.Fatalf("missing canonical Arc %#v", want)
}

func assertNoTopologyArc(t *testing.T, result *Result, from, to uint32) {
	t.Helper()
	for index := 0; index < result.ArcCount(); index++ {
		arc, ok := result.ArcAt(index)
		if !ok {
			t.Fatalf("ArcAt(%d) failed within ArcCount=%d", index, result.ArcCount())
		}
		if arc.From == from && arc.To == to {
			t.Fatalf("non-structural topology pair appeared as Arc: %#v", arc)
		}
	}
}

func assertCanonicalArcRows(t *testing.T, result *Result) {
	t.Helper()
	for index := 0; index < result.ArcCount(); index++ {
		arc, ok := result.ArcAt(index)
		if !ok || arc.From >= result.NodeCount() || arc.To >= result.NodeCount() ||
			arc.Source == 0 || arc.Target == 0 {
			t.Fatalf("invalid canonical ArcAt(%d): %#v/%v", index, arc, ok)
		}
		if arc.Decision == 0 && arc.Truth {
			t.Fatalf("unguarded ArcAt(%d) carried Truth=true: %#v", index, arc)
		}
	}
}
