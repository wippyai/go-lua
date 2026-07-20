package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCallResultReductionPublishesNonreturningStateThenNormalBottomAfterN0(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	transaction := factapply.PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, typevalue.LiteralString(reg, "ready"))),
		},
	}), point)
	terms := NewArena(reg)
	effects := NewEffectArena(terms)
	freezer, err := newWorldProgramFreezer(terms, effects, Shape{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	freezer.programs[1] = programNode{kind: programSequence, point: point, instructions: []instructionRef{
		freezer.appendInstruction(instructionNode{kind: instructionCallResults, result: transaction, resultPhase: factapply.ConcreteCallResultPhaseMaterialize}),
		freezer.appendInstruction(instructionNode{kind: instructionNoNormalReturn}),
	}}
	program, err := freezer.seal(1)
	if err != nil {
		t.Fatal(err)
	}
	code, root, err := reduceWorldProgram(program, DefaultDescriptorRegistry())
	if err != nil {
		t.Fatal(err)
	}
	node := code.nodes[root]
	if node.kind != relationNodeSequence || len(node.steps) != 1 || node.steps[0].kind != boundaryStepCallResults ||
		node.steps[0].resultPhase != factapply.ConcreteCallResultPhaseMaterialize || node.next == 0 || code.nodes[node.next].kind != relationNodeNonreturning {
		t.Fatalf("N0 -> Bottom reduction = %#v next=%#v", node, code.nodes[node.next])
	}
	if len(code.outcomes) != 1 {
		t.Fatalf("no-return relation published %d normal outcomes", len(code.outcomes)-1)
	}
}

func TestCallResultReductionKeepsN3BeforeN5TerminalPublication(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	transaction := factapply.PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
			point: factflow.NewPostconditionRefinementSet(factflow.NewPostconditionRefinement(
				pathdom.NewPath(symbol.ID(44), "result"),
				factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
			)),
		},
		ReturnPresenceRelations: map[cfg.Point]factflow.ReturnPresenceRelationSet{
			point: factflow.NewReturnPresenceRelationSet(factflow.NewReturnPresenceRelation(0, presence.Present(), 1, presence.Absent())),
		},
	}), point)
	terms := NewArena(reg)
	effects := NewEffectArena(terms)
	freezer, err := newWorldProgramFreezer(terms, effects, Shape{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ret := freezer.appendReturn(returnPayload{preserved: newBoundaryPreservationLedger(0, 0), resultPublication: transaction, returnTransaction: testReturnTransactionTerm(t, point)})
	freezer.programs[1] = programNode{kind: programSequence, point: point, instructions: []instructionRef{
		freezer.appendInstruction(instructionNode{kind: instructionCallResults, result: transaction, resultPhase: factapply.ConcreteCallResultPhasePostconditions}),
		freezer.appendInstruction(instructionNode{kind: instructionReturn, ret: ret}),
	}}
	program, err := freezer.seal(1)
	if err != nil {
		t.Fatal(err)
	}
	code, root, err := reduceWorldProgram(program, DefaultDescriptorRegistry())
	if err != nil {
		t.Fatal(err)
	}
	node := code.nodes[root]
	if node.kind != relationNodeSequence || len(node.steps) != 1 || node.steps[0].resultPhase != factapply.ConcreteCallResultPhasePostconditions ||
		node.next == 0 || code.nodes[node.next].kind != relationNodeOutcome {
		t.Fatalf("N3 -> N5 reduction = %#v", node)
	}
	outcome := code.outcomes[code.nodes[node.next].outcome]
	if !outcome.resultPublication.HasPublicationSteps() || outcome.resultPublication.Point() != point {
		t.Fatalf("terminal publication transaction = %#v", outcome.resultPublication)
	}
}

func TestWorldProgramValidatorAcceptsSharedDiamondContinuation(t *testing.T) {
	terms := NewArena(standard.Registry())
	effects := NewEffectArena(terms)
	freezer, err := newWorldProgramFreezer(terms, effects, Shape{}, 4)
	if err != nil {
		t.Fatal(err)
	}
	freezer.programs[1] = programNode{kind: programChoice, guard: terms.True(), whenTrue: 2, whenFalse: 3}
	freezer.programs[2] = programNode{kind: programSequence, next: 4}
	freezer.programs[3] = programNode{kind: programSequence, next: 4}
	ret := freezer.appendReturn(returnPayload{preserved: newBoundaryPreservationLedger(0, 0), returnTransaction: testReturnTransactionTerm(t, 1)})
	freezer.programs[4] = programNode{kind: programSequence, instructions: []instructionRef{
		freezer.appendInstruction(instructionNode{kind: instructionReturn, ret: ret}),
	}}
	program, err := freezer.seal(1)
	if err != nil || !program.valid(true) {
		t.Fatalf("shared diamond rejected as a cycle: program=%#v err=%v", program, err)
	}
	if terms.Sealed() || effects.Sealed() {
		t.Fatal("lexical IR seal prematurely closed the reducer term builder")
	}
	code, root, err := reduceWorldProgram(program, DefaultDescriptorRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if root == 0 || len(code.nodes) != 5 || code.nodes[2].kind != relationNodeSequence || code.nodes[3].kind != relationNodeSequence ||
		code.nodes[2].next == 0 || code.nodes[2].next != code.nodes[3].next || code.nodes[code.nodes[2].next].kind != relationNodeOutcome {
		t.Fatalf("diamond suffix was duplicated or disconnected: root=%d nodes=%#v", root, code.nodes)
	}
}

func TestRelationSealHasNoStructuralDepthLimit(t *testing.T) {
	const depth = 8192
	terms := NewArena(standard.Registry())
	effects := NewEffectArena(terms)
	nodes := make([]relationNode, depth+2)
	for index := 1; index <= depth; index++ {
		nodes[index] = relationNode{kind: relationNodeSequence, next: relationRootRef(index + 1)}
	}
	nodes[depth+1] = relationNode{kind: relationNodeOutcome, outcome: 1}
	code := &relationCode{
		terms:       terms,
		effects:     effects,
		descriptors: DefaultDescriptorRegistry(),
		shape:       Shape{},
		nodes:       nodes,
		outcomes: []boundaryOutcomeTuple{{}, {
			preserved: newBoundaryPreservationLedger(0, 0), returnTransaction: testReturnTransactionTerm(t, 1),
		}},
	}
	sealed, root, err := sealRelationCode(code, 1)
	if err != nil {
		t.Fatalf("exact %d-node relation rejected: %v", depth, err)
	}
	if root != 1 || len(sealed.nodes) != depth+2 || !sealed.valid(root) {
		t.Fatalf("deep relation was not preserved exactly: root=%d nodes=%d", root, len(sealed.nodes))
	}
}
