package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/control"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// outcomeFixture is deliberately assembled through every pre-Outcome owner.
// Outcome is a derived pass: its tests must not construct a body, binding,
// containment, or control proof by hand, and must not import the old core.
type outcomeFixture struct {
	preimage source.Preimage
	flow     authored.View
	bodies   *body.Result
	binding  binding.Result
	forest   *containment.Result
	shape    *control.Shape

	sourceFinalize source.Finalizer
	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize module.Finalizer
}

type outcomeSpec struct {
	counts    [keyspace.FamilyCount]uint32
	rows      [][]keyspace.Term
	flow      authored.Input
	binds     []source.BindCells
	forms     []source.FunctionFormals
	nilOwners []keyspace.Term
}

func openOutcomeFixture(t *testing.T, spec outcomeSpec) *outcomeFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 {
		t.Fatal("outcome fixture requires an Entry Body")
	}
	if len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatalf("Body rows = %d, want %d", len(spec.rows), spec.counts[keyspace.FamilyBody])
	}
	flowInput := spec.flow
	flowInput.Counts = spec.counts

	sourceInput := outcomeSourceInput(spec.counts, spec.rows, spec.binds, spec.forms, spec.nilOwners)
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := static.Input{Counts: spec.counts}
	if spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]static.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]static.CallContract, spec.counts[keyspace.FamilyCall])
	}
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

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("module.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		_ = moduleFinalize.Abort()
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		_ = moduleFinalize.Abort()
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("control.Seal: %v", err)
	}

	fixture := &outcomeFixture{
		preimage: preimage, flow: flowView, bodies: bodies, binding: bindingResult,
		forest: forest, shape: shape,
		sourceFinalize: sourceFinalize, staticFinalize: staticFinalize,
		flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		_ = fixture.moduleFinalize.Abort()
		_ = fixture.flowFinalize.Abort()
		_ = fixture.staticFinalize.Abort()
		_ = fixture.sourceFinalize.Abort()
	})
	return fixture
}

func (f *outcomeFixture) seal(t *testing.T) *Result {
	t.Helper()
	result, err := Seal(f.preimage.Identity(), f.flow, f.bodies, f.shape,
		f.staticFinalize.View().ContentID(), f.moduleFinalize.View().ContentID())

	if err != nil {
		t.Fatalf("outcome.Seal: %v", err)
	}
	return result
}

func outcomeSourceInput(
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	binds []source.BindCells,
	forms []source.FunctionFormals,
	nilOwners []keyspace.Term,
) source.Input {
	input := source.Input{Name: "flow-outcome.lua"}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{
				File: input.Name, StartLine: line, StartCol: 1,
				EndLine: line, EndCol: 1,
			}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, counts[keyspace.FamilyBody])
	for index := range input.Bodies {
		var terms []keyspace.Term
		if index < len(rows) {
			terms = append(terms, rows[index]...)
		}
		input.Bodies[index] = source.BodySource{
			Body:  keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)),
			Terms: terms,
		}
	}
	input.Binds = make([]source.BindCells, counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		if index < len(binds) {
			input.Binds[index].Cells = append([]keyspace.Term(nil), binds[index].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		if index < len(forms) {
			input.Functions[index].Formals = append([]keyspace.Term(nil), forms[index].Formals...)
		}
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyNil]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(nilOwners) {
			owner = nilOwners[ordinal-1]
		}
		input.Nil = append(input.Nil, source.NilLiteral{Owner: owner})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBool]; ordinal++ {
		input.Bool = append(input.Bool, source.BoolLiteral{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: ordinal&1 == 1,
		})
	}
	return input
}

func outcomeTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func outcomeCounts(body, values, nils, returns, breaks, labels, gotos, branches, loops, functions uint32) (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = body
	counts[keyspace.FamilyValues] = values
	counts[keyspace.FamilyNil] = nils
	counts[keyspace.FamilyReturn] = returns
	counts[keyspace.FamilyBreak] = breaks
	counts[keyspace.FamilyLabel] = labels
	counts[keyspace.FamilyGoto] = gotos
	counts[keyspace.FamilyBranch] = branches
	counts[keyspace.FamilyLoop] = loops
	counts[keyspace.FamilyFunction] = functions
	return counts
}
