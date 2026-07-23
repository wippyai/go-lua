package transformer

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// This file contracts validatePredicateExpr, validatePredicateSource,
// validateExpressionRefinement, predicateSourceBoundDuringRow, and
// prepareCertifiedScalarExpressions in compiler_structural_predicate.go. Unlike
// samePredicateSource/predicateSourcePath/containsStructuralPoint, these
// functions read through a *Builder's Arena, so tests drive them via a hand
// built planCompileContext (as test_helpers_test.go does for CertifyPlan)
// rather than raw facts alone.

// newPredicateContext builds the smallest planCompileContext that can reach
// validatePredicateExpr/validatePredicateSource/validateExpressionRefinement/
// predicateSourceBoundDuringRow: a real Builder/Arena over an empty plan, plus
// the maps prepareCertifiedScalarExpressions would otherwise initialize itself.
// Callers that need real CFG points/facts (predicateSourceBoundDuringRow,
// prepareCertifiedScalarExpressions) pass their own plan and graph.
func newPredicateContext(t *testing.T, facts factflow.Facts, graph cfg.Graph, plan *operationplan.Plan) *planCompileContext {
	t.Helper()
	reg := standard.Registry()
	if plan == nil {
		graph = cfg.New()
		plan = operationplan.New(graph, factflow.FactsInput{})
	}
	return &planCompileContext{
		registry:              reg,
		graph:                 graph,
		plan:                  plan,
		facts:                 facts,
		builder:               NewBuilder(reg, Shape{}, nil, plan),
		locals:                make(map[symbol.ID]ValueTerm),
		resultRoots:           make(map[ResultRoot]ValueTerm),
		expressions:           make(map[factflow.ExprRef][]ValueTerm),
		predicateExpressions:  make(map[factflow.ExprRef]struct{}),
		expressionRefinements: make(map[factflow.ExprRef]struct{}),
		structuralPredicates:  make(map[factflow.ExprRef]factflow.StructuralExpressionRegion),
	}
}

func mustBinaryOp(t *testing.T, op string, left, right factflow.ValueSource) factflow.ExpressionOperation {
	t.Helper()
	out, ok := factflow.NewBinaryExpressionOperation(op, left, right)
	if !ok {
		t.Fatalf("binary operation %q rejected", op)
	}
	return out
}

func mustUnaryOp(t *testing.T, op string, operand factflow.ValueSource) factflow.ExpressionOperation {
	t.Helper()
	out, ok := factflow.NewUnaryExpressionOperation(op, operand)
	if !ok {
		t.Fatalf("unary operation %q rejected", op)
	}
	return out
}

func mustIntLiteral(t *testing.T, value int64) factflow.ValueSource {
	t.Helper()
	source, ok := factflow.NewIntegerLiteralValueSource(value, 0, 0, 0, mustScalarShape(t))
	if !ok {
		t.Fatalf("integer literal %d rejected", value)
	}
	return source
}

func mustExprSource(t *testing.T, ref factflow.ExprRef) factflow.ValueSource {
	t.Helper()
	source, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, mustScalarShape(t))
	if !ok {
		t.Fatalf("expression source %d rejected", ref)
	}
	return source
}

// TestValidatePredicateExprOperatorMatrix drives validatePredicateExpr
// directly across comparison/unary/intrinsic roots with literal, local, call,
// and nested-arithmetic operands, distinguishing accepted producers from
// genuinely uncertifiable ones.
func TestValidatePredicateExprOperatorMatrix(t *testing.T) {
	t.Run("comparison of two literals is accepted", func(t *testing.T) {
		root := factflow.ExprRef(1)
		op := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustIntLiteral(t, 2))
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("comparison of two literals rejected: %v", err)
		}
	})

	t.Run("comparison of a local and a literal is accepted", func(t *testing.T) {
		root := factflow.ExprRef(1)
		sym := symbol.ID(9)
		p := pathdom.NewPath(sym, "n")
		local, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("path source rejected")
		}
		op := mustBinaryOp(t, "<", local, mustIntLiteral(t, 2))
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}})
		ctx := newPredicateContext(t, facts, nil, nil)
		ctx.locals[sym] = ctx.builder.Arena().Constant(typevalue.LiteralInt(ctx.registry, 1))
		if err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("comparison of a bound local and a literal rejected: %v", err)
		}
	})

	t.Run("comparison of a call result and a literal is accepted", func(t *testing.T) {
		root := factflow.ExprRef(1)
		callPoint := cfg.Point(7)
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, callPoint, mustScalarShape(t))
		if !ok {
			t.Fatal("call source rejected")
		}
		op := mustBinaryOp(t, "<", callSource, mustIntLiteral(t, 2))
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}})
		ctx := newPredicateContext(t, facts, nil, nil)
		ctx.resultRoots[ResultRoot{Point: callPoint, Slot: 0}] = ctx.builder.Arena().Constant(typevalue.LiteralInt(ctx.registry, 7))
		if err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("comparison of a bound call result and a literal rejected: %v", err)
		}
	})

	t.Run("unary not applied to a literal is accepted", func(t *testing.T) {
		root := factflow.ExprRef(1)
		operand, ok := factflow.NewBoolLiteralValueSource(true, 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("bool literal rejected")
		}
		op := mustUnaryOp(t, "not", operand)
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("unary not over a literal rejected: %v", err)
		}
	})

	t.Run("lua_type intrinsic over a literal is accepted", func(t *testing.T) {
		root := factflow.ExprRef(1)
		op, ok := factflow.NewIntrinsicExpressionOperation(intrinsic.LuaType, mustIntLiteral(t, 1))
		if !ok {
			t.Fatal("lua_type intrinsic operation rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("lua_type intrinsic over a literal rejected: %v", err)
		}
	})

	t.Run("a bare arithmetic root is genuinely uncertifiable as a predicate producer", func(t *testing.T) {
		root := factflow.ExprRef(1)
		op := mustBinaryOp(t, "*", mustIntLiteral(t, 2), mustIntLiteral(t, 3))
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool))
		if err == nil || !strings.Contains(err.Error(), "unsupported predicate producer") {
			t.Fatalf("bare arithmetic root error = %v, want an unsupported predicate producer rejection", err)
		}
	})

	t.Run("an expression identity shared between an object literal and a scalar operation is rejected", func(t *testing.T) {
		root := factflow.ExprRef(1)
		op := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustIntLiteral(t, 2))
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op},
			ObjectLiterals:       map[factflow.ExprRef]factflow.ObjectLiteral{root: factflow.NewObjectLiteral(nil)},
		})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool))
		if err == nil || !strings.Contains(err.Error(), "object literal and scalar operation share an expression identity") {
			t.Fatalf("shared object-literal identity error = %v, want the identity-conflict rejection", err)
		}
	})

	// TestExternalCensusUnsupportedPredicateProducer mirror: `local n = 1;
	// return n < 2 * 3`. validatePredicateExpr recurses into every nested
	// operand that itself carries an ExpressionOperation fact through
	// validatePredicateSource -> validatePredicateExpr, and that recursive
	// call demands the nested operand independently pass the same
	// comparison/and/or/unary predicate-producer gate as a root. An arithmetic
	// expression used purely as a scalar operand of a comparison is not itself
	// a predicate root and should not need to satisfy that gate; the contract
	// is that this call still certifies, but it currently fails closed with
	// "unsupported predicate producer" reported for the nested "*" operand.
	t.Run("RED an arithmetic operand nested inside a comparison is accepted", func(t *testing.T) {
		root, nested := factflow.ExprRef(1), factflow.ExprRef(2)
		nestedOp := mustBinaryOp(t, "*", mustIntLiteral(t, 2), mustIntLiteral(t, 3))
		rootOp := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustExprSource(t, nested))
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: rootOp, nested: nestedOp},
		})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validatePredicateExpr(ctx, root, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("arithmetic operand of a comparison rejected: %v", err)
		}
	})
}

func TestValidatePredicateSourceShapeAndFallbacks(t *testing.T) {
	t.Run("an expanded operand is rejected as non-scalar", func(t *testing.T) {
		shape, ok := factflow.NewValueSourceShape(true, true, false, false)
		if !ok {
			t.Fatal("expanded shape rejected")
		}
		source, ok := factflow.NewIntegerLiteralValueSource(1, 0, 0, 0, shape)
		if !ok {
			t.Fatal("literal source rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validatePredicateSource(ctx, source, make(map[factflow.ExprRef]bool))
		if err == nil || !strings.Contains(err.Error(), "non-scalar operand") {
			t.Fatalf("expanded operand error = %v, want a non-scalar rejection", err)
		}
	})

	t.Run("an unbound symbol with no evidence anywhere is rejected", func(t *testing.T) {
		sym := symbol.ID(21)
		p := pathdom.NewPath(sym, "n")
		source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("path source rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validatePredicateSource(ctx, source, make(map[factflow.ExprRef]bool)); err == nil {
			t.Fatal("an unbound local with no plan evidence anywhere was accepted")
		}
	})

	t.Run("a symbol bound only through a root-assignment row fallback is accepted", func(t *testing.T) {
		sym := symbol.ID(22)
		graph := cfg.New()
		assign := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(graph.Entry(), assign, false)
		graph.AddEdge(assign, graph.Exit(), false)
		fact := factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, sym, pathdom.NewPath(sym, "n"), mustIntLiteral(t, 1))
		input := factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: fact}}
		plan := operationplan.New(graph, input)

		p := pathdom.NewPath(sym, "n")
		source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("path source rejected")
		}
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		if err := validatePredicateSource(ctx, source, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("a symbol with a row-local root assignment was rejected: %v", err)
		}
	})
}

func TestValidateExpressionRefinement(t *testing.T) {
	reg := standard.Registry()
	stringContract := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	t.Run("a runtime cast over an exact literal source is accepted", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		refinement := factflow.NewExpressionRuntimeValidation(mustIntLiteral(t, 1), stringContract)
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement}})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("runtime cast over an exact literal rejected: %v", err)
		}
	})

	t.Run("a meet refinement over an exact source is accepted", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		refinement := factflow.NewExpressionRefinement(mustIntLiteral(t, 1), stringContract)
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement}})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool))
		if err != nil {
			t.Fatalf("meet refinement rejected: %v", err)
		}
	})

	t.Run("an expression identity shared with a scalar operation is rejected", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		refinement := factflow.NewExpressionRuntimeValidation(mustIntLiteral(t, 1), stringContract)
		op := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustIntLiteral(t, 2))
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement},
			ExpressionOperations:  map[factflow.ExprRef]factflow.ExpressionOperation{ref: op},
		})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool))
		if err == nil || !strings.Contains(err.Error(), "shares identity with a scalar operation") {
			t.Fatalf("shared scalar-operation identity error = %v, want the identity-conflict rejection", err)
		}
	})

	t.Run("a zero expression ref is rejected as cyclic", func(t *testing.T) {
		facts := factflow.NewFacts(factflow.FactsInput{})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validateExpressionRefinement(ctx, 0, make(map[factflow.ExprRef]bool))
		if err == nil || !strings.Contains(err.Error(), "cyclic expression DAG") {
			t.Fatalf("zero expression ref error = %v, want a cyclic-DAG rejection", err)
		}
	})

	// TestExternalCensusRefinementOutsideCertifiedPredicate mirror: `local g:
	// any = nil; local pid = g() :: string`. The cast's source is the result of
	// a fully dynamic (any-typed) call: there is no static signature, no plan
	// CallSurface classification, and no CallSiteView evidence to consult, so
	// validatePredicateSource's exactCompilerSourceTermActive and
	// predicateSourceBoundDuringRow fallback both fail. The contract is that a
	// runtime cast is itself certifying evidence for its own call-result source.
	t.Run("a runtime cast over an any-typed call result is accepted", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		callPoint := cfg.Point(1)
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, callPoint, mustScalarShape(t))
		if !ok {
			t.Fatal("call source rejected")
		}
		refinement := factflow.NewExpressionRuntimeValidation(callSource, stringContract)
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement}})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("runtime cast over an any-typed call result rejected: %v", err)
		}
	})

	t.Run("a wrapper result path is independent from its call source", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, cfg.Point(1), mustScalarShape(t))
		if !ok {
			t.Fatal("call source rejected")
		}
		resultPath := pathdom.NewPath(symbol.ID(41), "validated")
		refinement := factflow.NewExpressionRuntimeValidation(callSource, stringContract).WithResultPath(resultPath)
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement},
			ExpressionPaths:       map[factflow.ExprRef]pathdom.Path{ref: resultPath},
		})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("wrapper-owned result path rejected: %v", err)
		}
	})

	for _, tc := range []struct {
		name       string
		refinement factflow.ExpressionRefinement
	}{
		{name: "missing result-path ownership", refinement: factflow.NewExpressionRuntimeValidation(mustIntLiteral(t, 1), stringContract)},
		{name: "mismatched result-path ownership", refinement: factflow.NewExpressionRuntimeValidation(mustIntLiteral(t, 1), stringContract).WithResultPath(pathdom.NewPath(symbol.ID(42), "other"))},
	} {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			ref := factflow.ExprRef(1)
			facts := factflow.NewFacts(factflow.FactsInput{
				ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: tc.refinement},
				ExpressionPaths:       map[factflow.ExprRef]pathdom.Path{ref: pathdom.NewPath(symbol.ID(41), "validated")},
			})
			ctx := newPredicateContext(t, facts, nil, nil)
			err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool))
			if err == nil || !strings.Contains(err.Error(), "shares identity with an unrelated path") {
				t.Fatalf("result-path ownership error = %v, want fail-closed path rejection", err)
			}
		})
	}

	t.Run("a closed indexed coordinate remains scalar when its value list expands", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		shape, ok := factflow.NewValueSourceShape(true, true, false, false)
		if !ok {
			t.Fatal("closed expanded call shape rejected")
		}
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, cfg.Point(1), shape)
		if !ok {
			t.Fatal("closed expanded call source rejected")
		}
		refinement := factflow.NewExpressionRuntimeValidation(callSource, stringContract)
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement}})
		ctx := newPredicateContext(t, facts, nil, nil)
		if err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool)); err != nil {
			t.Fatalf("closed indexed call coordinate rejected: %v", err)
		}
	})

	t.Run("an open multi-slot call result remains rejected", func(t *testing.T) {
		ref := factflow.ExprRef(1)
		shape, ok := factflow.NewValueSourceShape(true, true, false, true)
		if !ok {
			t.Fatal("open call shape rejected")
		}
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, cfg.Point(1), shape)
		if !ok {
			t.Fatal("open call source rejected")
		}
		refinement := factflow.NewExpressionRuntimeValidation(callSource, stringContract)
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: refinement}})
		ctx := newPredicateContext(t, facts, nil, nil)
		err := validateExpressionRefinement(ctx, ref, make(map[factflow.ExprRef]bool))
		if err == nil || !strings.Contains(err.Error(), "non-scalar operand") {
			t.Fatalf("open multi-slot runtime cast error = %v, want non-scalar rejection", err)
		}
	})
}

func TestPredicateSourceBoundDuringRow(t *testing.T) {
	t.Run("a versioned path is never row-bound", func(t *testing.T) {
		p := pathdom.Path{Symbol: symbol.ID(31), Version: 1}
		source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("path source rejected")
		}
		ctx := newPredicateContext(t, factflow.NewFacts(factflow.FactsInput{}), nil, nil)
		if predicateSourceBoundDuringRow(*ctx, source) {
			t.Fatal("a versioned path was reported as row-bound")
		}
	})

	t.Run("a canonical symbol with a matching root assignment is row-bound", func(t *testing.T) {
		sym := symbol.ID(32)
		graph := cfg.New()
		assign := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(graph.Entry(), assign, false)
		graph.AddEdge(assign, graph.Exit(), false)
		fact := factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, sym, pathdom.NewPath(sym, "n"), mustIntLiteral(t, 1))
		plan := operationplan.New(graph, factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: fact}})
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)

		p := pathdom.NewPath(sym, "n")
		source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("path source rejected")
		}
		if !predicateSourceBoundDuringRow(*ctx, source) {
			t.Fatal("a canonical symbol with a matching root assignment was not row-bound")
		}
	})

	t.Run("a canonical symbol with no root assignment anywhere is not row-bound", func(t *testing.T) {
		graph := cfg.New()
		plan := operationplan.New(graph, factflow.FactsInput{})
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		p := pathdom.NewPath(symbol.ID(33), "n")
		source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("path source rejected")
		}
		if predicateSourceBoundDuringRow(*ctx, source) {
			t.Fatal("a symbol with no root assignment anywhere was reported as row-bound")
		}
	})

	t.Run("an expression source with no backing expr is not row-bound", func(t *testing.T) {
		ctx := newPredicateContext(t, factflow.NewFacts(factflow.FactsInput{}), nil, nil)
		source := factflow.ValueSource{Kind: factflow.ValueSourceExpression}
		if predicateSourceBoundDuringRow(*ctx, source) {
			t.Fatal("an expression source with no backing expr was reported as row-bound")
		}
	})

	t.Run("an expression path with a matching root assignment is row-bound", func(t *testing.T) {
		sym := symbol.ID(34)
		ref := factflow.ExprRef(1)
		graph := cfg.New()
		assign := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(graph.Entry(), assign, false)
		graph.AddEdge(assign, graph.Exit(), false)
		fact := factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, sym, pathdom.NewPath(sym, "n"), mustIntLiteral(t, 1))
		input := factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: fact},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: pathdom.NewPath(sym, "n")},
		}
		plan := operationplan.New(graph, input)
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		if !predicateSourceBoundDuringRow(*ctx, mustExprSource(t, ref)) {
			t.Fatal("an expression path with a matching root assignment was not row-bound")
		}
	})

	t.Run("a call source with no call-site evidence is not row-bound", func(t *testing.T) {
		ctx := newPredicateContext(t, factflow.NewFacts(factflow.FactsInput{}), nil, nil)
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, cfg.Point(9), mustScalarShape(t))
		if !ok {
			t.Fatal("call source rejected")
		}
		if predicateSourceBoundDuringRow(*ctx, callSource) {
			t.Fatal("a call with no call-site evidence was reported as row-bound")
		}
	})

	t.Run("a call source targeting a lexical local is row-bound", func(t *testing.T) {
		targetSym := symbol.ID(35)
		graph := cfg.New()
		call := graph.AddNode(cfg.NodeCall)
		ret := graph.AddNode(cfg.NodeReturn)
		graph.AddEdge(graph.Entry(), call, false)
		graph.AddEdge(call, ret, false)
		graph.AddEdge(ret, graph.Exit(), false)

		site := factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextStatement, Point: call, HasPoint: true,
			ResultTargets: []factflow.CallResultTarget{
				factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, targetSym, pathdom.NewPath(targetSym, "x")),
			},
		})
		input := factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{call: site}}
		bodyID := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
		plan := operationplan.New(graph, input)
		w := wir.NewBody(t.Name())
		emitStructuralPredicateCallPoint(w, call)
		w.AssignDebugPointOrdinals(graph)
		target, ok := operationplan.NewLexicalCallSurfaceTarget(bodyID)
		if !ok {
			t.Fatal("lexical call surface target rejected")
		}
		surface, err := operationplan.SealCallSurface(bodyID, graph.Size(), []cfg.Point{call}, []operationplan.CallSurfaceSite{{Point: call, Target: target}})
		if err != nil {
			t.Fatal(err)
		}
		plan = plan.WithCallSurface(surface)

		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		callSource, ok := factflow.NewCallValueSource(0, 0, 0, 0, call, mustScalarShape(t))
		if !ok {
			t.Fatal("call source rejected")
		}
		if !predicateSourceBoundDuringRow(*ctx, callSource) {
			t.Fatal("a call result bound to a lexical local was not row-bound")
		}
	})

	t.Run("a literal source is never row-bound", func(t *testing.T) {
		ctx := newPredicateContext(t, factflow.NewFacts(factflow.FactsInput{}), nil, nil)
		if predicateSourceBoundDuringRow(*ctx, mustIntLiteral(t, 1)) {
			t.Fatal("a literal source was reported as row-bound")
		}
	})
}

func TestPreparePredicateExpressions(t *testing.T) {
	t.Run("an effectful RHS call remains CFG-owned while its result is a predicate leaf", func(t *testing.T) {
		root := factflow.ExprRef(1)
		left, ok := factflow.NewBoolLiteralValueSource(true, 0, 0, 0, mustScalarShape(t))
		if !ok {
			t.Fatal("bool literal rejected")
		}
		graph := cfg.New()
		branch := graph.AddBranch()
		call := graph.AddNode(cfg.NodeCall)
		join := graph.AddNode(cfg.NodeJoin)
		ret := graph.AddNode(cfg.NodeReturn)
		graph.AddEdge(graph.Entry(), branch, false)
		graph.AddEdge(branch, call, true)
		graph.AddEdge(branch, join, false)
		graph.AddEdge(call, join, false)
		graph.AddEdge(join, ret, false)
		graph.AddEdge(ret, graph.Exit(), false)

		targetSym := symbol.ID(9)
		site := factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextStatement, Point: call, HasPoint: true,
			ResultTargets: []factflow.CallResultTarget{
				factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, targetSym, pathdom.NewPath(targetSym, "result")),
			},
		})
		callResult, ok := factflow.NewCallValueSource(0, 0, 0, 0, call, mustScalarShape(t))
		if !ok {
			t.Fatal("call result source rejected")
		}
		logical := mustBinaryOp(t, "and", left, callResult)
		condition, ok := factflow.NewBranchCondition(left, true)
		if !ok {
			t.Fatal("branch condition rejected")
		}
		region, ok := factflow.NewStructuralExpressionRegion(branch, call, join, join, true, []cfg.Point{call})
		if !ok {
			t.Fatal("structural region rejected")
		}
		input := factflow.FactsInput{
			CallSites:              map[cfg.Point]factflow.CallSite{call: site},
			BranchConditionSources: map[cfg.Point]factflow.BranchCondition{branch: condition},
			ExpressionOperations:   map[factflow.ExprRef]factflow.ExpressionOperation{root: logical},
			Returns:                map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{mustExprSource(t, root)})},
		}
		plan := operationplan.NewWithStructuralExpressionRegions(graph, input, map[factflow.ExprRef]factflow.StructuralExpressionRegion{root: region})
		bodyID := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
		w := wir.NewBody(t.Name())
		emitStructuralPredicateCallPoint(w, call)
		w.AssignDebugPointOrdinals(graph)
		callTarget, ok := operationplan.NewLexicalCallSurfaceTarget(bodyID)
		if !ok {
			t.Fatal("lexical call target rejected")
		}
		surface, err := operationplan.SealCallSurface(bodyID, graph.Size(), []cfg.Point{call}, []operationplan.CallSurfaceSite{{Point: call, Target: callTarget}})
		if err != nil {
			t.Fatal(err)
		}
		plan = plan.WithCallSurface(surface)

		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		if err := prepareCertifiedScalarExpressions(ctx); err != nil {
			t.Fatalf("CFG-owned RHS call result was rejected as a predicate leaf: %v", err)
		}
		if _, certified := ctx.predicateExpressions[root]; !certified {
			t.Fatal("logical result was not certified")
		}
		if _, structural := ctx.structuralPredicates[root]; !structural {
			t.Fatal("logical result lost its exact structural region")
		}
		callCells := 0
		cursor := plan.Cursor(call)
		for cell, more := cursor.Next(); more; cell, more = cursor.Next() {
			if cell.Kind() == operationplan.CallSite {
				callCells++
			}
		}
		if callCells != 1 {
			t.Fatalf("RHS call has %d executable call cells, want exactly one", callCells)
		}
	})

	t.Run("a supported comparison root over literals is certified", func(t *testing.T) {
		root := factflow.ExprRef(1)
		op := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustIntLiteral(t, 2))
		graph := cfg.New()
		input := factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op}}
		plan := operationplan.New(graph, input)
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		if err := prepareCertifiedScalarExpressions(ctx); err != nil {
			t.Fatalf("a supported comparison root was rejected: %v", err)
		}
		if _, ok := ctx.predicateExpressions[root]; !ok {
			t.Fatal("a supported comparison root was not recorded as a certified predicate expression")
		}
	})

	t.Run("the same root reached from two return points is certified once", func(t *testing.T) {
		root := factflow.ExprRef(1)
		op := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustIntLiteral(t, 2))
		rootSource := mustExprSource(t, root)
		graph := cfg.New()
		retA := graph.AddNode(cfg.NodeReturn)
		retB := graph.AddNode(cfg.NodeReturn)
		graph.AddEdge(graph.Entry(), retA, false)
		graph.AddEdge(retA, retB, false)
		graph.AddEdge(retB, graph.Exit(), false)
		input := factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: op},
			Returns: map[cfg.Point]factflow.Return{
				retA: factflow.NewReturn([]factflow.ValueSource{rootSource}),
				retB: factflow.NewReturn([]factflow.ValueSource{rootSource}),
			},
		}
		plan := operationplan.New(graph, input)
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		if err := prepareCertifiedScalarExpressions(ctx); err != nil {
			t.Fatalf("a root shared by two return points was rejected: %v", err)
		}
		if _, ok := ctx.predicateExpressions[root]; !ok {
			t.Fatal("a root shared by two return points was not recorded as certified")
		}
	})

	// TestExternalCensusUnsupportedPredicateProducer mirror, driven through the
	// public preparation entry point rather than validatePredicateExpr
	// directly: `local n = 1; return n < 2 * 3`. See the matching RED case in
	// TestValidatePredicateExprOperatorMatrix for the exact mechanism.
	t.Run("RED an arithmetic operand nested inside a returned comparison is accepted", func(t *testing.T) {
		root, nested := factflow.ExprRef(1), factflow.ExprRef(2)
		nestedOp := mustBinaryOp(t, "*", mustIntLiteral(t, 2), mustIntLiteral(t, 3))
		rootOp := mustBinaryOp(t, "<", mustIntLiteral(t, 1), mustExprSource(t, nested))
		graph := cfg.New()
		input := factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{root: rootOp, nested: nestedOp},
		}
		plan := operationplan.New(graph, input)
		ctx := newPredicateContext(t, plan.Facts(), graph, plan)
		if err := prepareCertifiedScalarExpressions(ctx); err != nil {
			t.Fatalf("arithmetic operand of a returned comparison rejected: %v", err)
		}
	})
}

func emitStructuralPredicateCallPoint(body *wir.Body, point cfg.Point) {
	start := body.Len()
	body.Emit(wir.Instruction{Op: wir.OpCall, Point: point})
	body.SetPointRange(point, start, body.Len())
}
