package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/program/flow/internal/causal"
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
	staticID   keyspace.ContentID
	moduleID   keyspace.ContentID

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize module.Finalizer
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
		closeContinuationFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, module.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, module.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("module.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()
	staticID, moduleID := staticView.ContentID(), moduleView.ContentID()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	ports, err := evaluation.SealPorts(preimage.Identity(), flowView, forest, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("evaluation.SealPorts: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinalize.Commit(indexInput)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()

	controlResult, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	recurrenceResult, err := recurrence.Seal(sourceView, flowView, bodies, forest, controlResult, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("recurrence.Seal: %v", err)
	}
	executableResult, err := executable.Seal(sourceView, flowView, forest, controlResult, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("executable.Seal: %v", err)
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), flowView, executableResult, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("candidates.Seal: %v", err)
	}
	causalResult, err := causal.Seal(sourceView, flowView, bodies, forest, outcomes, controlResult, recurrenceResult, ports, executableResult, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("causal.Seal: %v", err)
	}
	result, err := Seal(sourceView, flowView, bodies, bindingResult, executableResult, candidateResult, causalResult, staticID, moduleID)
	if err != nil {
		closeContinuationFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("continuation.Seal: %v", err)
	}
	fixture := &continuationFixture{
		sourceView: sourceView, flow: flowView, bodies: bodies, binding: bindingResult,
		forest: forest, executable: executableResult, candidates: candidateResult,
		causal: causalResult, result: result, staticID: staticID, moduleID: moduleID,
		staticFinalize: staticFinalize, flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closeContinuationFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeContinuationFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize module.Finalizer) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func continuationSourceInput(spec continuationSpec) source.Input {
	name := spec.name
	if name == "" {
		name = "continuation-semantic.lua"
	}
	input := source.Input{Name: name, Keys: append([]source.KeyInput(nil), spec.keys...), ExactAtoms: append([]keyspace.LiteralValue(nil), spec.exactAtoms...)}
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

func termFamilyRange(family keyspace.Family, start, end uint32) []keyspace.Term {
	terms := make([]keyspace.Term, end-start+1)
	for index := range terms {
		terms[index] = continuationTerm(family, start+uint32(index))
	}
	return terms
}
