package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

type checkSpec struct {
	name         string
	counts       [keyspace.FamilyCount]uint32
	rows         [][]keyspace.Term
	literalOwner keyspace.Term
	flow         authored.Input
	static       static.Input
	module       imports.Input
	binds        []source.BindCells
	formals      []source.FunctionFormals
	exacts       []keyspace.LiteralValue
	keys         []source.KeyInput
}

type checkFixture struct {
	sourceView source.View
	flowView   authored.View
	staticView static.View
	moduleView imports.View
	preimage   source.Preimage
	bodies     *body.Result
	bindings   binding.Result
	forest     *containment.Result
	proof      *containment.StaticScopeProof
	access     *accessgeometry.Result
	entry      keyspace.Term

	sourceFinal source.Finalizer
	flowFinal   authored.Finalizer
	staticFinal static.Finalizer
	moduleFinal imports.Finalizer
}

func newCheckFixture(t *testing.T, spec checkSpec) *checkFixture {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if spec.name == "" {
		spec.name = "staticcheck-law.lua"
	}
	flowInput := spec.flow
	flowInput.Counts = spec.counts
	staticInput := spec.static
	staticInput.Counts = spec.counts
	sourceInput := checkSourceInput(spec)

	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinal, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}

	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinal.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinal, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinal.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = staticFinal.Abort()
		_ = sourceFinal.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinal, err := flowDraft.Finalizer()
	if err != nil {
		_ = staticFinal.Abort()
		_ = sourceFinal.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}

	flowView := flowFinal.View()
	staticView := staticFinal.View()
	preimage := sourceFinal.Preimage()
	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(spec.module)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinal, err := moduleDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinal.View()
	forest, proof, err := containment.Prove(preimage, staticView, flowView, bodies, bindings, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("containment.Prove: %v", err)
	}
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()
	shape, err := control.Seal(preimage, flowView, bodies, bindings, forest, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("outcome.Seal: %v", err)
	}
	index, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinal.Commit(index)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	access, err := accessgeometry.SealSelectors(sourceView, flowView, bodies, bindings, staticView, moduleView)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("accessgeometry.Seal: %v", err)
	}
	fixture := &checkFixture{
		sourceView: sourceView, flowView: flowView, staticView: staticView, moduleView: moduleView,
		preimage: preimage, bodies: bodies, bindings: bindings, forest: forest, proof: proof, access: access, entry: entry,
		sourceFinal: sourceFinal, flowFinal: flowFinal, staticFinal: staticFinal, moduleFinal: moduleFinal,
	}
	t.Cleanup(func() {
		_ = fixture.flowFinal.Abort()
		_ = fixture.staticFinal.Abort()
		_ = fixture.moduleFinal.Abort()
	})
	return fixture
}

func checkSourceInput(spec checkSpec) source.Input {
	input := source.Input{Name: spec.name, ExactAtoms: append([]keyspace.LiteralValue(nil), spec.exacts...), Keys: append([]source.KeyInput(nil), spec.keys...)}
	literalOwner := spec.literalOwner
	if literalOwner == 0 {
		literalOwner = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	}
	input.Families = flowtest.FamilySpans(spec.name, spec.counts)
	input.Bodies = make([]source.BodySource, spec.counts[keyspace.FamilyBody])
	for ordinal := range input.Bodies {
		if ordinal < len(spec.rows) {
			input.Bodies[ordinal].Terms = append([]keyspace.Term(nil), spec.rows[ordinal]...)
		}
		input.Bodies[ordinal].Body = keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal+1))
	}
	input.Binds = make([]source.BindCells, spec.counts[keyspace.FamilyBind])
	for ordinal := range input.Binds {
		input.Binds[ordinal].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(ordinal+1))
		if ordinal < len(spec.binds) {
			input.Binds[ordinal].Cells = append([]keyspace.Term(nil), spec.binds[ordinal].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, spec.counts[keyspace.FamilyFunction])
	for ordinal := range input.Functions {
		input.Functions[ordinal].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(ordinal+1))
		if ordinal < len(spec.formals) {
			input.Functions[ordinal].Formals = append([]keyspace.Term(nil), spec.formals[ordinal].Formals...)
		}
	}
	input.Nil = flowtest.LiteralRows(spec.counts[keyspace.FamilyNil], nil, literalOwner, func(owner keyspace.Term, _ uint32) source.NilLiteral {
		return source.NilLiteral{Owner: owner}
	})
	input.Bool = flowtest.LiteralRows(spec.counts[keyspace.FamilyBool], nil, literalOwner, func(owner keyspace.Term, ordinal uint32) source.BoolLiteral {
		return source.BoolLiteral{Owner: owner, Value: ordinal%2 == 1}
	})
	input.Integer = flowtest.LiteralRows(spec.counts[keyspace.FamilyInteger], nil, literalOwner, func(owner keyspace.Term, ordinal uint32) source.IntegerLiteral {
		return source.IntegerLiteral{Owner: owner, Value: int64(ordinal)}
	})
	input.Float = flowtest.LiteralRows(spec.counts[keyspace.FamilyFloat], nil, literalOwner, func(owner keyspace.Term, ordinal uint32) source.FloatLiteral {
		return source.FloatLiteral{Owner: owner, Bits: uint64(ordinal)}
	})
	input.String = flowtest.LiteralRows(spec.counts[keyspace.FamilyString], nil, literalOwner, func(owner keyspace.Term, _ uint32) source.StringLiteral {
		return source.StringLiteral{Owner: owner, Value: "literal"}
	})
	return input
}

func checkCounts(items ...struct {
	family keyspace.Family
	count  uint32
}) (counts [keyspace.FamilyCount]uint32) {
	for _, item := range items {
		counts[item.family] = item.count
	}
	return counts
}

func checkCount(family keyspace.Family, count uint32) struct {
	family keyspace.Family
	count  uint32
} {
	return struct {
		family keyspace.Family
		count  uint32
	}{family: family, count: count}
}
