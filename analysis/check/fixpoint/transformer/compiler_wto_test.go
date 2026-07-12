package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPreparedPlanCompilerExactWTOLoopComposesAliasedStaticDirectCall(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	iteratorCall := graph.AddNode(cfg.NodeCall)
	head := graph.AddNode(cfg.NodeBranch)
	genericPoint := graph.AddNode(cfg.NodeAssign)
	aliasPoint := graph.AddNode(cfg.NodeAssign)
	directPoint := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), iteratorCall, false)
	graph.AddEdge(iteratorCall, head, false)
	graph.AddEdge(head, genericPoint, true)
	graph.AddEdge(head, graph.Exit(), false)
	graph.AddEdge(genericPoint, aliasPoint, false)
	graph.AddEdge(aliasPoint, directPoint, false)
	graph.AddEdge(directPoint, head, false)

	containerSymbol := symbol.ID(10)
	projectionSymbol := symbol.ID(20)
	aliasSymbol := symbol.ID(21)
	resultSymbol := symbol.ID(22)
	containerRef, memberRef := factflow.ExprRef(1), factflow.ExprRef(2)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	adjusted, _ := factflow.NewValueSourceShape(true, false, true, false)
	containerSource, _ := factflow.NewPathValueSource(pathdom.NewPath(containerSymbol, "routes").Key(), 0, 0, 0, scalar)
	memberSource, _ := factflow.NewExpressionValueSource(memberRef, 0, 0, 0, adjusted)
	aliasSource, _ := factflow.NewPathValueSource(pathdom.NewPath(projectionSymbol, "route").Key(), 0, 0, 0, scalar)
	iteratorSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: iteratorCall, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{containerSource},
	})
	directSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: directPoint, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{memberSource},
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, resultSymbol, pathdom.NewPath(resultSymbol, "resolved"),
		)},
	})
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	generic, ok := operationplan.NewGenericForOperation(1, projectionSymbol, projectionSymbol-1, operationplan.GenericForSource{
		Kind: operationplan.GenericForSourceCall, CallPoint: iteratorCall, HasCallPoint: true,
	}, []typ.Type{typ.NewArray(record)})
	if !ok {
		t.Fatal("generic-for operation rejected")
	}
	generic = generic.WithIterator(iter)
	sigOp, ok := operationplan.NewSignatureCallOperation(signature.Function{
		Type: typ.Func().Param("source", typ.Any).Returns(typ.Any).Build(), Effect: effect.Row{Labels: []effect.Label{iter}},
	})
	if !ok {
		t.Fatal("iterator signature rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{iteratorCall: iteratorSite, directPoint: directSite},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{aliasPoint: factflow.NewRootAssignment(
			factflow.RootAssignmentLocalDeclaration, aliasSymbol, pathdom.NewPath(aliasSymbol, "route_entry"), aliasSource,
		)},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			containerRef: pathdom.NewPath(containerSymbol, "routes"),
			memberRef:    pathdom.NewPath(aliasSymbol, "route_entry").Field("target_name"),
		},
	}).WithBoundaryParams([]symbol.ID{containerSymbol}).
		WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{iteratorCall: sigOp}).
		WithExtensions([]operationplan.ExtensionInput{{Point: genericPoint, Kind: operationplan.BodyGenericFor, GenericFor: generic}})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.cyclic || prepared.wtoTape == nil || len(prepared.wtoTape.components) != 1 {
		t.Fatalf("prepared topology cyclic/tape/components = %v/%p/%d", prepared.cyclic, prepared.wtoTape, len(prepared.wtoTape.components))
	}

	calleePlan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(calleePlan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	calleeBuilder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), calleePlan)
	calleeValue := calleeBuilder.Arena().Constant(typevalue.LiteralString(reg, "resolved"))
	callee, err := calleeBuilder.Build(certificate, []Row{{Guard: calleeBuilder.Arena().True(), Ops: []Operation{{
		Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: calleeValue,
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	calleeRef := CellRef{Function: 701}
	catalog, err := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{directPoint: {Cell: calleeRef, Shape: Shape{Params: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	view := RelationView{values: map[CellRef]Relation{calleeRef: callee}, allowed: map[CellRef]struct{}{calleeRef: {}}}
	tape := prepared.wtoTape
	digest := symbolicWTOTapeDigest(prepared.wtoTape)
	first := prepared.EvaluateDirect(view, catalog)
	second := prepared.EvaluateDirect(view, catalog)
	if first.ContextualReason() != "" || first.Widened() || !EqualRelation(first, second) {
		t.Fatalf("cyclic direct relation contextual/widened/equal = %q/%v/%v", first.ContextualReason(), first.Widened(), EqualRelation(first, second))
	}
	if prepared.wtoTape != tape || symbolicWTOTapeDigest(prepared.wtoTape) != digest {
		t.Fatal("repeated cyclic evaluation rebuilt or mutated prepared WTO topology")
	}

	allocationSignature, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, _ := effectlowering.StaticSignatureAllocationTemplate(allocationSignature)
	allocationOp, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 701, Template: template.Root, Ordinal: 1,
	}, template)
	allocation := calleeBuilder.Arena().AllocationTemplate(allocationOp)
	allocationEffect, err := calleeBuilder.EffectArena().AllocationTemplate(allocation)
	if err != nil {
		t.Fatal(err)
	}
	effectful := callee
	effectful.rows = append([]Row(nil), callee.rows...)
	effectful.rows[0].Effects = []EffectTerm{allocationEffect}
	effectView := RelationView{values: map[CellRef]Relation{calleeRef: effectful}, allowed: map[CellRef]struct{}{calleeRef: {}}}
	rejected := prepared.EvaluateDirect(effectView, catalog)
	if rejected.ContextualReason() == "" || !rejected.Widened() || rejected.Rows() != 0 {
		t.Fatalf("recurrent effect relation = reason %q widened %v rows %d", rejected.ContextualReason(), rejected.Widened(), rejected.Rows())
	}
}

func TestPreparedPlanCompilerUncertifiedWTOLoopFailsAsContextualRelation(t *testing.T) {
	graph := cfg.New()
	head := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(graph.Entry(), head, false)
	graph.AddEdge(head, head, true)
	graph.AddEdge(head, graph.Exit(), false)
	prepared, err := NewPlanCompiler().Prepare(standard.Registry(), graph, operationplan.New(graph, factflow.FactsInput{}), Shape{})
	if err != nil {
		t.Fatal(err)
	}
	relation := prepared.Evaluate()
	if relation.ContextualReason() == "" || !relation.Widened() || relation.Rows() != 0 {
		t.Fatalf("uncertified cyclic relation = reason %q widened %v rows %d", relation.ContextualReason(), relation.Widened(), relation.Rows())
	}
}
