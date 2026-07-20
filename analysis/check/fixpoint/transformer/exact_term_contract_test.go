package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestExactExternalCallResultTermCompilesDependentSignatureEquation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	argument, ok := factflow.NewStringLiteralValueSource("preserved", 0, 0, 0, factflow.ValueSourceShape{})
	if !ok {
		t.Fatal("argument source rejected")
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource,
		Point:   point, HasPoint: true,
		Final: true, Adjusted: true,
		ArgumentSources: []factflow.ValueSource{argument},
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
	})
	sig := signature.Function{
		Type: typ.Func().Param("value", typ.Any).Returns(typ.Any).Build(),
		Effect: effect.Empty.With(returns.Return{
			ReturnIndex: 0,
			Transform:   returns.SameAs{Source: effect.ParamRef{Index: 0}},
		}),
	}
	op, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("signature operation rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: site}}).
		WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{point: op})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}
	source, ok := factflow.NewCallValueSource(0, 0, 0, 0, point, factflow.ValueSourceShape{Final: true, Adjusted: true})
	if !ok {
		t.Fatal("call result source rejected")
	}
	term, exact, err := exactExternalCallResultTerm(ctx, source)
	if err != nil || !exact || term == 0 {
		t.Fatalf("dependent result term = %d/%t/%v", term, exact, err)
	}
	value, evaluated := builder.Arena().evalValue(term, BindingCursor{}, SpecializationContext{})
	if !evaluated || !product.Equal(reg, value, typevalue.LiteralString(reg, "preserved")) {
		t.Fatalf("dependent result value = %#v/%t, want argument literal", value, evaluated)
	}
}

// These tests pin exactCompilerSourceTermRaw's and exactReturnCallResultTerm's
// resolution matrix directly against planCompileContext, without RunChunk.
// exactCompilerSourceTermRaw is the sole emit site of the RootAssignments/
// Returns "not a context-independent scalar" external-error family (~96
// errors); exactReturnCallResultTerm is the `return { f() }` call-authority
// family. Each case below isolates one branch of one function.

func TestExactCompilerSourceTermRawResolvesLiteralScalar(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	plan := operationplan.New(graph, factflow.FactsInput{})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}

	source, ok := factflow.NewIntegerLiteralValueSource(4, 0, 0, 0, factflow.ValueSourceShape{})
	if !ok {
		t.Fatal("fixture literal source is invalid")
	}
	got, err := exactCompilerSourceTermRaw(ctx, source, nil)
	if err != nil {
		t.Fatalf("literal scalar source rejected: %v", err)
	}
	want := builder.Arena().Constant(typevalue.LiteralInt(reg, 4))
	if got == 0 || got != want {
		t.Fatalf("literal scalar term = %d, want interned constant %d", got, want)
	}
}

func TestExactExternalCallResultTermSelectsImplicitOpenTailStaticResult(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	shape, ok := factflow.NewValueSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("open-tail call shape rejected")
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource,
		Point:   callPoint, HasPoint: true,
		Final: true, Expanded: true, OpenTail: true,
		MethodName: "synthetic",
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
	})
	sig := signature.Function{Type: typ.Func().Returns(typ.String, typ.Integer).Build()}
	op, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("static signature operation rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}}).
		WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{callPoint: op}).
		WithBoundaryReturns([]product.Value{product.Top(), product.Top()})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}
	source, ok := factflow.NewCallValueSource(0, 0, 1, 1, callPoint, shape)
	if !ok {
		t.Fatal("open-tail call source rejected")
	}

	term, exact, err := exactExternalCallResultTerm(ctx, source)
	if err != nil || !exact || term == 0 {
		t.Fatalf("open-tail static result term = %d, exact=%t, err=%v", term, exact, err)
	}
	value, evaluated := builder.Arena().evalValue(term, BindingCursor{}, SpecializationContext{})
	got, typed := typevalue.TypeOf(reg, value)
	if !evaluated || !typed || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("open-tail static result = %v, evaluated=%t, typed=%t, want integer", got, evaluated, typed)
	}
}

func TestExactExternalCallResultTermAcceptsExpressionProducerAcrossConsumerPositions(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextExpressionProducer,
		Point:   callPoint, HasPoint: true,
		Final: true, Expanded: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}
	if err := bindExternalCallSlotTerms(&ctx, callPoint, nil); err != nil {
		t.Fatal(err)
	}
	want, bound := builder.Arena().callResultValue(callPoint, 0)
	if !bound || want == 0 {
		t.Fatal("condition result slot was not sealed")
	}
	consumerShape, ok := factflow.NewValueSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("expression consumer shape rejected")
	}
	for _, consumerPosition := range []int{0, 1, 2, 7} {
		source, ok := factflow.NewCallValueSource(0, consumerPosition, consumerPosition, 0, callPoint, consumerShape)
		if !ok {
			t.Fatalf("expression consumer position %d call source rejected", consumerPosition)
		}
		got, exact, err := exactExternalCallResultTerm(ctx, source)
		if err != nil || !exact || got != want {
			t.Fatalf("expression consumer position %d call term = %d, exact=%t, err=%v, want producer slot %d", consumerPosition, got, exact, err, want)
		}
	}
}

func TestPreboundStaticCallStillDeclaresPointOwnedProducerCell(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	ref := factflow.ExprRef(771)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextExpressionProducer,
		Point:   point, HasPoint: true, ExprRef: ref, HasExpr: true,
		Final: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: site}})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	static := builder.Arena().Constant(typevalue.String(reg))
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		expressions: map[factflow.ExprRef][]ValueTerm{ref: {static}},
	}
	if err := bindExternalCallSlotTerms(&ctx, point, nil); err != nil {
		t.Fatal(err)
	}
	cell, bound := builder.Arena().callResultValue(point, 0)
	if !bound || cell == 0 || len(ctx.externalResults[point]) != 1 || ctx.externalResults[point][0] != cell {
		t.Fatal("prebound static result omitted the external producer's CallResult footprint")
	}
	if got := ctx.expressions[ref]; len(got) != 1 || got[0] != static {
		t.Fatal("producer-cell binding replaced the exact static consumer term")
	}
}

func TestExactExternalCallResultTermAcceptsConditionResultWithoutMaterializedTarget(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextCondition,
		Point:   callPoint, HasPoint: true,
		Final: true, Adjusted: true,
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	want := builder.Arena().bindCallResult(callPoint, 0)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}
	// Branch presentation has consumed the call's adjusted-one-result flag.
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("condition call shape rejected")
	}
	source, ok := factflow.NewCallValueSource(0, 0, factflow.NoValueSourceIndex, 0, callPoint, shape)
	if !ok {
		t.Fatal("condition call source rejected")
	}

	got, exact, err := exactExternalCallResultTerm(ctx, source)
	if err != nil || !exact || got != want {
		t.Fatalf("condition call term = %d, exact=%t, err=%v, want producer slot %d", got, exact, err, want)
	}
}

func TestBindExternalCallSlotTermsSealsImplicitOpenTailTuple(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource,
		Point:   callPoint, HasPoint: true,
		Final: true, Expanded: true, OpenTail: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}}).
		WithBoundaryReturns([]product.Value{product.Top(), product.Top()})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		expressions: make(map[factflow.ExprRef][]ValueTerm),
	}

	if err := bindExternalCallSlotTerms(&ctx, callPoint, nil); err != nil {
		t.Fatal(err)
	}
	for slot := 0; slot < 2; slot++ {
		if term, bound := builder.Arena().callResultValue(callPoint, slot); !bound || term == 0 {
			t.Fatalf("implicit open-tail result slot %d was not sealed", slot)
		}
	}
}

func TestExactCompilerSourceTermRawResolvesLocalSymbol(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	local := symbol.ID(101)
	ref := factflow.ExprRef(1)
	facts := factflow.FactsInput{ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: pathdom.NewPath(local, "x")}}
	plan := operationplan.New(graph, facts)
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	localTerm := builder.Arena().Constant(typevalue.LiteralString(reg, "local-x"))
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		locals: map[symbol.ID]ValueTerm{local: localTerm},
	}

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: ref}
	got, err := exactCompilerSourceTermRaw(ctx, source, nil)
	if err != nil || got != localTerm {
		t.Fatalf("bare local symbol term = %d, err = %v, want bound local %d", got, err, localTerm)
	}
}

func TestExactCompilerSourceTermRawResolvesBoundaryParam(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	param := symbol.ID(201)
	ref := factflow.ExprRef(1)
	facts := factflow.FactsInput{ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: pathdom.NewPath(param, "p")}}
	plan := operationplan.New(graph, facts).WithBoundaryParams([]symbol.ID{param}).WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
	builder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder, locals: make(map[symbol.ID]ValueTerm)}
	if err := bindBoundaryParamTerms(&ctx, Shape{Params: 1}); err != nil {
		t.Fatal(err)
	}

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: ref}
	got, err := exactCompilerSourceTermRaw(ctx, source, nil)
	want := builder.Arena().Root(Root{Kind: RootParam, Index: 0})
	if err != nil || got != want {
		t.Fatalf("boundary param term = %d, err = %v, want interned param root %d", got, err, want)
	}
}

func TestExactCompilerSourceTermRawResolvesPureBinaryOverExactLocals(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	leftSym, rightSym := symbol.ID(301), symbol.ID(302)
	leftRef, rightRef, topRef := factflow.ExprRef(1), factflow.ExprRef(2), factflow.ExprRef(3)
	leftSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: leftRef}
	rightSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: rightRef}
	op, ok := factflow.NewBinaryExpressionOperation("+", leftSource, rightSource)
	if !ok {
		t.Fatal("fixture binary operation rejected")
	}
	facts := factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			leftRef: pathdom.NewPath(leftSym, "a"), rightRef: pathdom.NewPath(rightSym, "b"),
		},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{topRef: op},
	}
	plan := operationplan.New(graph, facts)
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	leftTerm := builder.Arena().Constant(typevalue.LiteralInt(reg, 1))
	rightTerm := builder.Arena().Constant(typevalue.LiteralInt(reg, 2))
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		locals: map[symbol.ID]ValueTerm{leftSym: leftTerm, rightSym: rightTerm},
	}

	topSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: topRef}
	got, err := exactCompilerSourceTermRaw(ctx, topSource, nil)
	want, exact := builder.Arena().ScalarBinaryValue("+", leftTerm, rightTerm)
	if !exact {
		t.Fatal("fixture binary term did not intern")
	}
	if err != nil || got != want {
		t.Fatalf("pure binary term = %d, err = %v, want interned binary %d", got, err, want)
	}
}

func TestExactCompilerSourceTermRawResolvesPureUnaryOutsidePredicatePosition(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	argSym := symbol.ID(401)
	argRef, topRef := factflow.ExprRef(1), factflow.ExprRef(2)
	argSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: argRef}
	op, ok := factflow.NewUnaryExpressionOperation("not", argSource)
	if !ok {
		t.Fatal("fixture unary operation rejected")
	}
	facts := factflow.FactsInput{
		ExpressionPaths:      map[factflow.ExprRef]pathdom.Path{argRef: pathdom.NewPath(argSym, "a")},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{topRef: op},
	}
	plan := operationplan.New(graph, facts)
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		locals: map[symbol.ID]ValueTerm{argSym: builder.Arena().Constant(typevalue.LiteralBool(reg, true))},
	}

	topSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: topRef}
	got, err := exactCompilerSourceTermRaw(ctx, topSource, nil)
	operand := ctx.locals[argSym]
	want, exact := builder.Arena().ScalarUnaryValue("not", operand)
	if !exact || err != nil || got != want {
		t.Fatalf("non-predicate unary term = %d, err=%v, want canonical term %d", got, err, want)
	}
}

func TestExactCompilerSourceTermRawResolvesPureUnaryWhenCertifiedPredicate(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	argSym := symbol.ID(401)
	argRef, topRef := factflow.ExprRef(1), factflow.ExprRef(2)
	argSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: argRef}
	op, ok := factflow.NewUnaryExpressionOperation("not", argSource)
	if !ok {
		t.Fatal("fixture unary operation rejected")
	}
	facts := factflow.FactsInput{
		ExpressionPaths:      map[factflow.ExprRef]pathdom.Path{argRef: pathdom.NewPath(argSym, "a")},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{topRef: op},
	}
	plan := operationplan.New(graph, facts)
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	argTerm := builder.Arena().Constant(typevalue.LiteralBool(reg, true))
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		locals:               map[symbol.ID]ValueTerm{argSym: argTerm},
		predicateExpressions: map[factflow.ExprRef]struct{}{topRef: {}},
	}

	topSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: topRef}
	got, err := exactCompilerSourceTermRaw(ctx, topSource, nil)
	want, exact := builder.Arena().ScalarUnaryValue("not", argTerm)
	if !exact {
		t.Fatal("fixture unary term did not intern")
	}
	if err != nil || got != want {
		t.Fatalf("certified unary predicate term = %d, err = %v, want interned unary %d", got, err, want)
	}
}

func TestExternalCallAccessTermsRetainsCanonicalLengthArgument(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	container := symbol.ID(402)
	containerRef, lengthRef := factflow.ExprRef(1), factflow.ExprRef(2)
	containerSource, ok := factflow.NewExpressionValueSource(containerRef, 0, 0, 0, mustScalarShape(t))
	if !ok {
		t.Fatal("container source rejected")
	}
	lengthOperation, ok := factflow.NewUnaryExpressionOperation("#", containerSource)
	if !ok {
		t.Fatal("length operation rejected")
	}
	lengthSource, ok := factflow.NewExpressionValueSource(lengthRef, 0, 1, 0, mustScalarShape(t))
	if !ok {
		t.Fatal("length argument source rejected")
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextStatement, Point: callPoint, HasPoint: true,
		ArgumentSources: []factflow.ValueSource{factflow.NewNilValueSource(0), lengthSource},
	})
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callPoint: site},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			containerRef: pathdom.NewPath(container, "items"),
		},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{lengthRef: lengthOperation},
	})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
		locals:                map[symbol.ID]ValueTerm{container: builder.Arena().bindEnvironmentSymbol(container)},
		structuralEnvironment: true, point: callPoint,
	}
	planAccess, err := externalCallAccessTerms(ctx, callPoint)
	if err != nil {
		t.Fatalf("external length argument: %v", err)
	}
	foundLength := false
	for _, item := range planAccess.access {
		node := builder.Arena().values[item.term]
		if node.op == valueUnaryOperation && node.operator == "#" {
			foundLength = true
		}
	}
	if !foundLength {
		t.Fatalf("external access = %#v, want canonical length term", planAccess.access)
	}
	if len(planAccess.operands.arguments) != 2 ||
		builder.Arena().values[planAccess.operands.arguments[1]].op != valueUnaryOperation ||
		builder.Arena().values[planAccess.operands.arguments[1]].operator != "#" {
		t.Fatalf("external operand roots = %#v, want ordered nil/length canonical roots", planAccess.operands.arguments)
	}
}

// TestExactCompilerSourceTermRawRejectsUnclassifiedCallResult mirrors the
// census family's minimal reproducers (`return math.sqrt(4)`, a
// registry-modeled channel.select result in root-assignment position): a call
// result with no bound frame root, no sealed external call-surface
// classification, and no matching local-assignment call-site target has no
// exact value in this scope. This is the sole emit site of the
// RootAssignments/Returns "not a context-independent scalar" family.
func TestExactCompilerSourceTermRawRejectsUnclassifiedCallResult(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	plan := operationplan.New(graph, factflow.FactsInput{})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, HasCallPoint: true, CallPoint: callPoint, ResultIndex: 0, Final: true}
	if !source.Valid() {
		t.Fatal("fixture call source is invalid")
	}
	_, err := exactCompilerSourceTermRaw(ctx, source, nil)
	if err == nil || !strings.Contains(err.Error(), "not a context-independent scalar") {
		t.Fatalf("unclassified call-result error = %v, want context-independent-scalar rejection", err)
	}
}

func TestExactCompilerSourceTermRawRuntimeValidationOwnsUnresolvedScalarCall(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	plan := operationplan.New(graph, factflow.FactsInput{})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ref := factflow.ExprRef(1)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar call shape rejected")
	}
	callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, cfg.Point(1), shape)
	if !ok {
		t.Fatal("scalar call source rejected")
	}
	validated := typevalue.LiteralString(reg, "validated")
	refinement := factflow.NewExpressionRuntimeValidation(callSource, validated)
	facts := factflow.NewFacts(factflow.FactsInput{ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement}})
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: facts, builder: builder,
		expressionRefinements: map[factflow.ExprRef]struct{}{ref: {}},
	}
	expression, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
	if !ok {
		t.Fatal("runtime validation expression source rejected")
	}
	term, err := exactCompilerSourceTermRaw(ctx, expression, nil)
	if err != nil || term == 0 {
		t.Fatalf("runtime validation over unresolved scalar call = %d, err = %v", term, err)
	}
	node := builder.Arena().values[term]
	if node.op != valueExpressionRefinement || node.refinementMode != factflow.ExpressionRefinementRuntimeValidation || len(node.args) != 1 || !product.Equal(reg, node.value, validated) {
		t.Fatalf("runtime validation node = %#v, want one-source validated term", node)
	}
	inner := builder.Arena().values[node.args[0]]
	if inner.op != valueConstant || !product.Equal(reg, inner.value, product.Bottom(reg)) {
		t.Fatalf("unresolved runtime validation source = %#v, want constant Bottom", inner)
	}
}

func TestExactCompilerSourceTermRawCertifiedMeetRefinementUsesCanonicalExpressionMath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ref := factflow.ExprRef(1)
	inner := factflow.NewNilValueSource(0)
	claim := product.Set(reg, typevalue.FromType(reg, typ.Any), assertion.Key, assertion.Any())
	refinement := factflow.NewExpressionRefinement(inner, claim)
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement},
	})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
	}
	if err := prepareCertifiedScalarExpressions(&ctx); err != nil {
		t.Fatalf("certify meet refinement: %v", err)
	}
	expression, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, mustScalarShape(t))
	if !ok {
		t.Fatal("expression source rejected")
	}
	term, err := exactCompilerSourceTermRaw(ctx, expression, nil)
	if err != nil || term == 0 {
		t.Fatalf("certified meet refinement term = %d, err = %v", term, err)
	}
	node := builder.Arena().values[term]
	if node.op != valueExpressionRefinement || node.refinementMode != factflow.ExpressionRefinementMeet {
		t.Fatalf("refinement node = %#v, want canonical meet expression refinement", node)
	}
	got, exact := builder.Arena().evalValue(term, BindingCursor{}, SpecializationContext{})
	want := enginesourcevalue.ApplyExpressionRefinement(reg, typevalue.Nil(reg), refinement)
	if !exact || !product.Equal(reg, got, want) {
		t.Fatalf("refinement value exact=%t value=%v, want canonical %v", exact, got, want)
	}
}

func TestExactCompilerSourceTermRawRejectsNonScalarObjectLiteral(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ref := factflow.ExprRef(1)
	facts := factflow.FactsInput{
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{ref: factflow.NewObjectLiteral(nil)},
	}
	plan := operationplan.New(graph, facts)
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: ref, ResultIndex: 1}
	_, err := exactCompilerSourceTermRaw(ctx, source, nil)
	if err == nil || !strings.Contains(err.Error(), "is not a scalar value") {
		t.Fatalf("non-scalar object literal error = %v, want scalar-value rejection", err)
	}
}

func TestExactCompilerSourceTermRawRejectsCyclicExpressionSource(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ref := factflow.ExprRef(7)
	plan := operationplan.New(graph, factflow.FactsInput{})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, plan: plan, facts: plan.Facts(), builder: builder}

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, HasExpr: true, ExprRef: ref}
	active := map[factflow.ExprRef]bool{ref: true}
	_, err := exactCompilerSourceTermRaw(ctx, source, active)
	if err == nil || !strings.Contains(err.Error(), "cyclic expression source") {
		t.Fatalf("cyclic expression error = %v, want cyclic-source rejection", err)
	}
}

// exactReturnCallResultTerm consumes the frame-result root minted for one
// enumerated Return target of a lexical-body call: the `return { f() }`
// call-authority family.

func TestExactReturnCallResultTermResolvesExactValueList(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true, Final: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	frameResult := builder.Arena().Constant(typevalue.LiteralInt(reg, 4))
	ctx := planCompileContext{
		registry: reg, plan: plan, facts: plan.Facts(), builder: builder,
		resultRoots: map[ResultRoot]ValueTerm{{Point: callPoint, Slot: 0}: frameResult},
	}

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, HasCallPoint: true, CallPoint: callPoint, ResultIndex: 0, TargetIndex: 0, Final: true}
	if !source.Valid() {
		t.Fatal("fixture call-result source is invalid")
	}
	got, err := exactReturnCallResultTerm(ctx, source)
	if err != nil || got != frameResult {
		t.Fatalf("table-constructor call-result term = %d, err = %v, want frozen frame root %d", got, err, frameResult)
	}
}

func TestExactReturnCallResultTermRejectsNonScalarSource(t *testing.T) {
	_, err := exactReturnCallResultTerm(planCompileContext{}, factflow.ValueSource{})
	if err == nil || !strings.Contains(err.Error(), "non-scalar return source") {
		t.Fatalf("zero-value call-result source error = %v, want non-scalar rejection", err)
	}
}

func TestExactReturnCallResultTermRejectsMismatchedSiteAuthority(t *testing.T) {
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextAssignmentSource, Point: callPoint, HasPoint: true, Final: true,
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	ctx := planCompileContext{facts: plan.Facts()}

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, HasCallPoint: true, CallPoint: callPoint, ResultIndex: 0, Final: true}
	_, err := exactReturnCallResultTerm(ctx, source)
	if err == nil || !strings.Contains(err.Error(), "no exact value-list authority") {
		t.Fatalf("wrong-context call-result error = %v, want value-list-authority rejection", err)
	}
}

func TestExactReturnCallResultTermRejectsAdjustedNonZeroResult(t *testing.T) {
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true, Adjusted: true,
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	ctx := planCompileContext{facts: plan.Facts()}

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, HasCallPoint: true, CallPoint: callPoint, ResultIndex: 1, Adjusted: true}
	_, err := exactReturnCallResultTerm(ctx, source)
	if err == nil || !strings.Contains(err.Error(), "adjusted call return source selects result") {
		t.Fatalf("adjusted non-zero result error = %v, want adjusted-result rejection", err)
	}
}

func TestExactReturnCallResultTermRejectsUntargetedResult(t *testing.T) {
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true, Final: true,
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	ctx := planCompileContext{facts: plan.Facts()}

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, HasCallPoint: true, CallPoint: callPoint, ResultIndex: 0, TargetIndex: 0, Final: true}
	_, err := exactReturnCallResultTerm(ctx, source)
	if err == nil || !strings.Contains(err.Error(), "no exact return target") {
		t.Fatalf("untargeted call-result error = %v, want no-return-target rejection", err)
	}
}

func TestExactReturnCallResultTermRejectsMissingFrozenFrameRoot(t *testing.T) {
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true, Final: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
	})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}})
	ctx := planCompileContext{facts: plan.Facts()}

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, HasCallPoint: true, CallPoint: callPoint, ResultIndex: 0, TargetIndex: 0, Final: true}
	_, err := exactReturnCallResultTerm(ctx, source)
	if err == nil || !strings.Contains(err.Error(), "no frozen frame root") {
		t.Fatalf("unbound frame-root call-result error = %v, want frozen-frame-root rejection", err)
	}
}
