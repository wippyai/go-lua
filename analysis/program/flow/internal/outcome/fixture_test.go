package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
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
	moduleFinalize imports.Finalizer
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
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]staticcontracts.CallContract, spec.counts[keyspace.FamilyCall])
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, static.Finalizer{}, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, static.Finalizer{}, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}

	fixture := &outcomeFixture{
		preimage: preimage, flow: flowView, bodies: bodies, binding: bindingResult,
		forest: forest, shape: shape,
		sourceFinalize: sourceFinalize, staticFinalize: staticFinalize,
		flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(fixture.sourceFinalize, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
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
	input.Families = flowtest.FamilySpans(input.Name, counts)
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
	input.Nil = flowtest.LiteralRows(counts[keyspace.FamilyNil], nilOwners, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, _ uint32) source.NilLiteral {
		return source.NilLiteral{Owner: owner}
	})
	input.Bool = flowtest.LiteralRows(counts[keyspace.FamilyBool], nil, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, ordinal uint32) source.BoolLiteral {
		return source.BoolLiteral{Owner: owner, Value: ordinal&1 == 1}
	})
	return input
}

func outcomeTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	return flowtest.Term(family, ordinal)
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
