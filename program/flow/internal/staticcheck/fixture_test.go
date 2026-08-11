package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/control"
	"github.com/wippyai/go-lua/program/flow/internal/directbinding"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/internal/position"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

type checkSpec struct {
	name         string
	counts       [keyspace.FamilyCount]uint32
	rows         [][]keyspace.Term
	literalOwner keyspace.Term
	flow         authored.Input
	static       static.Input
	module       module.Input
	binds        []source.BindCells
	formals      []source.FunctionFormals
	exacts       []keyspace.LiteralValue
	keys         []source.KeyInput
}

type checkFixture struct {
	sourceView source.View
	flowView   authored.View
	staticView static.View
	moduleView module.View
	preimage   source.Preimage
	bodies     *body.Result
	bindings   binding.Result
	forest     *containment.Result
	proof      *containment.StaticScopeProof
	direct     *directbinding.Result
	entry      keyspace.Term

	sourceFinal source.Finalizer
	flowFinal   authored.Finalizer
	staticFinal static.Finalizer
	moduleFinal module.Finalizer
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
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, module.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, module.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := module.Build(spec.module)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, module.Finalizer{})
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinal, err := moduleDraft.Finalizer()
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, module.Finalizer{})
		t.Fatalf("module.Finalizer: %v", err)
	}
	moduleView := moduleFinal.View()
	forest, proof, err := containment.Prove(preimage, staticView, flowView, bodies, bindings, moduleView, entry)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, moduleFinal)
		t.Fatalf("containment.Prove: %v", err)
	}
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()
	shape, err := control.Seal(preimage, flowView, bodies, bindings, forest, staticID, moduleID)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, moduleFinal)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, moduleFinal)
		t.Fatalf("outcome.Seal: %v", err)
	}
	index, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, moduleFinal)
		t.Fatalf("position.Seal: %v", err)
	}
	direct, err := directbinding.Seal(preimage, flowView, bodies, bindings, staticView, moduleView)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, moduleFinal)
		t.Fatalf("directbinding.Seal: %v", err)
	}
	sourceComponent, err := sourceFinal.Commit(index)
	if err != nil {
		cleanupCheckFixture(sourceFinal, flowFinal, staticFinal, moduleFinal)
		t.Fatalf("source.Commit: %v", err)
	}
	fixture := &checkFixture{
		sourceView: sourceComponent.View(), flowView: flowView, staticView: staticView, moduleView: moduleView,
		preimage: preimage, bodies: bodies, bindings: bindings, forest: forest, proof: proof, direct: direct, entry: entry,
		sourceFinal: sourceFinal, flowFinal: flowFinal, staticFinal: staticFinal, moduleFinal: moduleFinal,
	}
	t.Cleanup(func() {
		_ = fixture.flowFinal.Abort()
		_ = fixture.staticFinal.Abort()
		_ = fixture.moduleFinal.Abort()
	})
	return fixture
}

func cleanupCheckFixture(sourceFinal source.Finalizer, flowFinal authored.Finalizer, staticFinal static.Finalizer, moduleFinal module.Finalizer) {
	_ = sourceFinal.Abort()
	_ = flowFinal.Abort()
	_ = staticFinal.Abort()
	_ = moduleFinal.Abort()
}

func checkSourceInput(spec checkSpec) source.Input {
	input := source.Input{Name: spec.name, ExactAtoms: append([]keyspace.LiteralValue(nil), spec.exacts...), Keys: append([]source.KeyInput(nil), spec.keys...)}
	literalOwner := spec.literalOwner
	if literalOwner == 0 {
		literalOwner = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, spec.counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: spec.name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
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
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyNil]; ordinal++ {
		input.Nil = append(input.Nil, source.NilLiteral{Owner: literalOwner})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyBool]; ordinal++ {
		input.Bool = append(input.Bool, source.BoolLiteral{Owner: literalOwner, Value: ordinal%2 == 1})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyInteger]; ordinal++ {
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: literalOwner, Value: int64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyFloat]; ordinal++ {
		input.Float = append(input.Float, source.FloatLiteral{Owner: literalOwner, Bits: uint64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyString]; ordinal++ {
		input.String = append(input.String, source.StringLiteral{Owner: literalOwner, Value: "literal"})
	}
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
