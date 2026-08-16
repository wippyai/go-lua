package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func projectionCombine(left, right terminal.ID[uint8], joined terminal.ID[uint8]) (terminal.ID[uint8], bool) {
	if left == (terminal.ID[uint8]{}) {
		return right, true
	}
	if right == (terminal.ID[uint8]{}) || left == right {
		return left, true
	}
	return joined, true
}

func TestReindexProjectionRetainsExactValueNodeAcrossIssuedScopes(t *testing.T) {
	fixture := newDiagramFixture(t)
	source, ok := fixture.manager.SealScope([]guard.Atom{1, 2})
	if !ok {
		t.Fatal("source scope")
	}
	target, ok := fixture.manager.SealScope([]guard.Atom{1, 2})
	if !ok {
		t.Fatal("target scope")
	}
	if source.Same(target) {
		t.Fatal("test scopes unexpectedly share identity")
	}
	relationBuilder, ok := fixture.manager.NewReindex(source, target)
	if !ok || !relationBuilder.Identity(1) || !relationBuilder.Identity(2) {
		t.Fatal("coordinate identity construction")
	}
	relation, ok := relationBuilder.Seal()
	if !ok || !relation.PureProjection() {
		t.Fatal("coordinate identity did not prove pure projection")
	}

	builder := fixture.diagram.Begin()
	root, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 1, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("input write")
	}
	root, ok = builder.Seal(root)
	if !ok {
		t.Fatal("input seal")
	}
	value, present, valid := fixture.diagram.Get(root, factorFirst, 1)
	if !valid || !present {
		t.Fatal("input value")
	}

	transport := fixture.diagram.Begin()
	got, ok := transport.Reindex(value, relation, func(left, right terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		return projectionCombine(left, right, fixture.values[2])
	})
	if !ok || got.node != value.node {
		t.Fatal("coordinate identity did not retain the exact FDD node")
	}
}

func TestReindexProjectionOnlyCombinesAtForgottenFibers(t *testing.T) {
	fixture := newDiagramFixture(t)
	source, ok := fixture.manager.SealScope([]guard.Atom{1, 2})
	if !ok {
		t.Fatal("source scope")
	}
	target, ok := fixture.manager.SealScope([]guard.Atom{1, 2})
	if !ok {
		t.Fatal("target scope")
	}

	pureBuilder, ok := fixture.manager.NewReindex(source, target)
	if !ok || !pureBuilder.Identity(1) || !pureBuilder.Forget(2) {
		t.Fatal("pure projection construction")
	}
	pureRelation, ok := pureBuilder.Seal()
	if !ok || !pureRelation.PureProjection() {
		t.Fatal("pure projection seal")
	}

	setWork := fixture.manager.NewWork()
	literal, ok := setWork.Literal(1)
	if !ok {
		t.Fatal("target literal")
	}
	setWork.Seal()
	expression, ok := target.Expr(literal)
	if !ok {
		t.Fatal("target expression")
	}
	genericBuilder, ok := fixture.manager.NewReindex(source, target)
	if !ok || !genericBuilder.Set(1, expression) || !genericBuilder.Forget(2) {
		t.Fatal("reference relation construction")
	}
	genericRelation, ok := genericBuilder.Seal()
	if !ok || genericRelation.PureProjection() {
		t.Fatal("reference Set relation was accepted as pure projection")
	}

	inputBuilder := fixture.diagram.Begin()
	root, ok := inputBuilder.Set(fixture.diagram.Empty(), factorFirst, 1, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("first input write")
	}
	root, ok = inputBuilder.Set(root, factorFirst, 1, fixture.trueAtTwo, fixture.values[1])
	if !ok {
		t.Fatal("second input write")
	}
	root, ok = inputBuilder.Seal(root)
	if !ok {
		t.Fatal("input seal")
	}
	value, present, valid := fixture.diagram.Get(root, factorFirst, 1)
	if !valid || !present {
		t.Fatal("input value")
	}

	pureCalls := 0
	pureTransport := fixture.diagram.Begin()
	pure, ok := pureTransport.Reindex(value, pureRelation, func(left, right terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		pureCalls++
		return projectionCombine(left, right, fixture.values[2])
	})
	if !ok || pureCalls == 0 {
		t.Fatal("pure projection did not combine forgotten fibers")
	}

	genericCalls := 0
	referenceTransport := fixture.diagram.Begin()
	reference, ok := referenceTransport.Reindex(value, genericRelation, func(left, right terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		genericCalls++
		return projectionCombine(left, right, fixture.values[2])
	})
	if !ok || genericCalls == 0 {
		t.Fatal("reference reindex did not combine forgotten fibers")
	}
	if !fixture.diagram.equalValue(pure.node, reference.node) {
		t.Fatal("pure projection changed the reference semantic result")
	}
}
