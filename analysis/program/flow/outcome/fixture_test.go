package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
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
	staticView     staticquery.View
	flowFinalize   authored.Finalizer
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
	_, staticView, err := static.Build(staticInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, authored.Finalizer{})
		t.Fatalf("static.Build: %v", err)
	}

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

	moduleView := flowView.Imports()
	moduleID := flowView.ModuleID()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, flowFinalize)
		t.Fatalf("control.Seal: %v", err)
	}

	fixture := &outcomeFixture{
		preimage: preimage, flow: flowView, bodies: bodies, binding: bindingResult,
		forest: forest, shape: shape,
		sourceFinalize: sourceFinalize, staticView: staticView,
		flowFinalize: flowFinalize,
	}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(fixture.sourceFinalize, fixture.flowFinalize)
	})
	return fixture
}

func (f *outcomeFixture) seal(t *testing.T) *Result {
	t.Helper()
	result, err := Seal(f.preimage.Identity(), f.flow, f.bodies, f.shape,
		f.staticView.ContentID(), f.flow.ModuleID())

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
