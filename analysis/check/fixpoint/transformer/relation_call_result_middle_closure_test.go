package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCloseRelationCallResultMiddleSchemasOwnsCompleteLexicalWidth(t *testing.T) {
	reg := standard.Registry()
	callerGraph := cfg.New()
	callPoint := callerGraph.AddNode(cfg.NodeCall)
	definitionPoint := callerGraph.AddNode(cfg.NodeAssign)
	orphanMaterializePoint := callerGraph.AddNode(cfg.NodeCall)
	pathTarget := symbol.ID(9001)
	// Deliberately non-ordinal target order. Slot 1 is absent from caller syntax
	// but present in the callee outcome; slot 2 is path-only; slots 3 and 4 are
	// consumed only as expression/return values.
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextAssignmentSource, Point: callPoint, HasPoint: true, Final: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 4, 0, pathdom.Path{}),
			factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, 1, 2, pathTarget, pathdom.NewPath(pathTarget, "path-only")),
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 2, 3, 0, pathdom.Path{}),
		},
	})
	callerPlan := operationplan.New(callerGraph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callPoint: site},
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			orphanMaterializePoint: factflow.NewCallResultValueSet(factflow.NewCallResultValue(2, product.Top())),
		},
	})
	targetPlan := operationplan.New(cfg.New(), factflow.FactsInput{})
	callerBuilder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), callerPlan)
	targetBuilder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), targetPlan)
	frame := callerBuilder.arena.relationFrame(2, callPoint, 1, Shape{}, nil, nil, 0)
	if frame == 0 {
		t.Fatal("could not freeze lexical call frame")
	}
	// Slot 6 models a call result already referenced by closed expression syntax
	// but absent from CallSite target metadata. The canonical pass must retain
	// that finite environment use as part of the same width authority.
	if callerBuilder.arena.bindCallResult(callPoint, 6) == 0 {
		t.Fatal("could not bind pre-existing call-result environment use")
	}
	definitionFrame := callerBuilder.arena.relationFrame(2, definitionPoint, 1, Shape{}, nil, nil, 0)
	if definitionFrame == 0 || definitionFrame == frame {
		t.Fatal("could not freeze result-free definition frame")
	}
	targetOutcome := testReturnTransactionTerm(t, 1,
		targetBuilder.arena.Constant(product.Top()),
		targetBuilder.arena.Constant(product.Top()),
	)
	callerCode := &relationCode{terms: callerBuilder.arena, effects: callerBuilder.effects}
	targetCode := &relationCode{
		terms: targetBuilder.arena, effects: targetBuilder.effects,
		outcomes: []boundaryOutcomeTuple{{}, {returnTransaction: targetOutcome}},
	}
	prepared := []*PreparedPlanCompiler{
		{plan: callerPlan, builder: callerBuilder, codeBase: callerCode},
		{plan: targetPlan, builder: targetBuilder, codeBase: targetCode},
	}
	definitions := []relationProgramDefinition{{owner: 1, target: 2, point: definitionPoint, frame: definitionFrame}}
	if err := closeRelationCallResultMiddleSchemas(prepared, []*relationCode{callerCode, targetCode}, definitions); err != nil {
		t.Fatal(err)
	}
	if callerBuilder.arena.middle.sealed {
		t.Fatal("call-result pass sealed Middle instead of only closing its inventory")
	}
	if got := callerBuilder.arena.callFrames[frame].resultCount; got != 7 {
		t.Fatalf("closed call frame width = %d, want 7", got)
	}
	if got := callerBuilder.arena.callFrames[definitionFrame].resultCount; got != 0 {
		t.Fatalf("definition frame width = %d, want result-free", got)
	}
	if got := targetBuilder.shape.Results; got != 7 {
		t.Fatalf("closed target Output width = %d, want caller-demanded width 7", got)
	}
	if got := targetCode.shape.Results; got != 7 {
		t.Fatalf("closed target relation Output width = %d, want 7", got)
	}
	if got := callerBuilder.arena.callFrames[frame].shape.Results; got != 7 {
		t.Fatalf("closed frame target Output width = %d, want 7", got)
	}
	if _, present := callerBuilder.arena.environment[statekey.CallResult(uint32(definitionPoint), 0)]; present {
		t.Fatal("definition frame manufactured a call-result Middle register")
	}
	if _, present := callerBuilder.arena.environment[statekey.CallResult(uint32(orphanMaterializePoint), 2)]; !present {
		t.Fatal("unframed call-result materialization has no Middle register")
	}
	frameResults := make(map[uint32]bool)
	for term := ValueTerm(1); int(term) < len(callerBuilder.arena.values); term++ {
		node := callerBuilder.arena.values[term]
		if node.op == valueFrameResult && node.frame == frame {
			frameResults[uint32(node.resultIndex)] = true
		}
	}
	for slot := uint32(0); slot < 7; slot++ {
		if !frameResults[slot] {
			t.Fatalf("call result %d has no canonical FrameResult OUT selector", slot)
		}
		if _, present := callerBuilder.arena.environment[statekey.CallResult(uint32(callPoint), slot)]; !present {
			t.Fatalf("call result %d has no pre-close Middle inventory entry", slot)
		}
	}

	if _, err := newRelationTermClosure(callerBuilder.arena, Shape{}, callerPlan, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := newRelationTermClosure(targetBuilder.arena, Shape{}, targetPlan, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := len(callerBuilder.arena.middle.registers); got != 8 {
		t.Fatalf("sealed caller Middle width = %d, want 8", got)
	}

	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("call-result-middle-complete-width"))
	callerID, targetID := lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1)
	program := &RelationProgram{bodies: []relationProgramBody{
		{body: callerID, variable: 1, relation: Relation{shape: Shape{}, arena: callerBuilder.arena}},
		{body: targetID, variable: 2, relation: Relation{shape: targetBuilder.shape, arena: targetBuilder.arena}},
	}}
	slots, err := freezeSlotSpace(program)
	if err != nil {
		t.Fatal(err)
	}
	foundOrphanMaterialization := false
	for ordinal, register := range callerBuilder.arena.middle.registers {
		if register.point == orphanMaterializePoint {
			if register.kind != relationMiddleRegisterCallResult || register.ordinal != 2 {
				t.Fatalf("unframed materialization register %d = %#v", ordinal, register)
			}
			foundOrphanMaterialization = true
			continue
		}
		if register.kind != relationMiddleRegisterCallResult || register.point != callPoint || register.ordinal != uint32(ordinal) {
			t.Fatalf("Middle register %d = %#v; want deterministic call-result point/ordinal", ordinal, register)
		}
		root, ok := callerBuilder.arena.middleRoot(register.slot)
		if !ok || root != (Root{Kind: RootMiddle, Index: uint32(ordinal)}) {
			t.Fatalf("Middle register %d root = %#v/%t", ordinal, root, ok)
		}
		slot, ok := slots.Slot(callerID, root)
		formalRoot, rootOK := slot.Root()
		if !ok || !rootOK || formalRoot.Owner() != callerID || formalRoot.Vocabulary() != formal.Middle || formalRoot.Ordinal() != uint64(ordinal+1) {
			t.Fatalf("call-result FormalSlot %d = %#v/%t/%t", ordinal, formalRoot, ok, rootOK)
		}
	}
	if !foundOrphanMaterialization {
		t.Fatal("unframed materialization register was not sealed")
	}
	for ordinal := uint32(0); ordinal < 7; ordinal++ {
		if _, ok := slots.Slot(targetID, Root{Kind: RootResult, Index: ordinal}); !ok {
			t.Fatalf("caller-demanded target Output root %d is absent", ordinal)
		}
	}
}
