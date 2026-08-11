package causal

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/control"
	"github.com/wippyai/go-lua/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/internal/position"
	"github.com/wippyai/go-lua/program/flow/internal/recurrence"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
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
	result     *Result

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize module.Finalizer
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
		closeCausalFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, module.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, module.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeCausalFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("module.Finalizer: %v", err)
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
	sourceComponent, err := sourceFinalize.Commit(indexInput)
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
	recur, err := recurrence.Seal(sourceView, flowView, bodies, forest, graph, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("recurrence.Seal: %v", err)
	}
	execResult, err := executable.Seal(sourceView, flowView, forest, graph, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("executable.Seal: %v", err)
	}
	result, err := Seal(sourceView, flowView, bodies, forest, outcomes, graph, recur, ports, execResult, staticID, moduleID)
	if err != nil {
		closeCausalFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("causal.Seal: %v", err)
	}

	fixture := &causalFixture{
		sourceView: sourceView, flow: flowView, bodies: bodies, forest: forest,
		outcomes: outcomes, control: graph, recurrence: recur, ports: ports,
		executable: execResult, result: result,
		staticFinalize: staticFinalize, flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closeCausalFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeCausalFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize module.Finalizer) {
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
