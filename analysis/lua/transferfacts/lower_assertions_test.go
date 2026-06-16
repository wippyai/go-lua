package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerClaimsToSidecarsWithoutProofRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeRead := ident("x")
	anyRead := ident("x")
	nonNilRead := ident("x")
	typeCast := &ast.CastExpr{Expr: typeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
	anyCast := &ast.CastExpr{Expr: anyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxColonColon}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilRead}
	local := localAssign([]string{"a", "b", "c"}, typeCast, anyCast, nonNil)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, local, 3)
	typeSource := mustLocalSource(t, facts, points[0])
	anySource := mustLocalSource(t, facts, points[1])
	nonNilSource := mustLocalSource(t, facts, points[2])

	assertLoweredAssertion(t, facts, typeSource, assertion.Type(), factflow.ValueSourceExpression)
	assertLoweredAssertion(t, facts, anySource, assertion.Any(), factflow.ValueSourceExpression)
	assertLoweredAssertion(t, facts, nonNilSource, assertion.NonNil(), factflow.ValueSourceExpression)
	if len(facts.BranchRefinements(points[2])) != 0 {
		t.Fatalf("x! assignment produced branch/presence refinement")
	}
}

func TestLowerClaimsPreserveCastSyntaxVariantsWithoutProofRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	asTypeRead := ident("x")
	colonTypeRead := ident("x")
	asAnyRead := ident("x")
	colonAnyRead := ident("x")
	asTypeCast := &ast.CastExpr{Expr: asTypeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
	colonTypeCast := &ast.CastExpr{Expr: colonTypeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxColonColon}
	asAnyCast := &ast.CastExpr{Expr: asAnyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxAs}
	colonAnyCast := &ast.CastExpr{Expr: colonAnyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxColonColon}
	local := localAssign([]string{"a", "b", "c", "d"}, asTypeCast, colonTypeCast, asAnyCast, colonAnyCast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, local, 4)
	cases := []struct {
		name  string
		point cfg.Point
		want  assertion.Value
	}{
		{name: "as type", point: points[0], want: assertion.Type()},
		{name: "colon type", point: points[1], want: assertion.Type()},
		{name: "as any", point: points[2], want: assertion.Any()},
		{name: "colon any", point: points[3], want: assertion.Any()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mustLocalSource(t, facts, tc.point)
			assertLoweredAssertion(t, facts, source, tc.want, factflow.ValueSourceExpression)
		})
	}
}

func TestLowerParsedCastClaimsOnlyProduceClaimRefinements(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b, c, d = x as number, x :: number, x as any, x :: any
`)

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 4)
	cases := []struct {
		name  string
		point cfg.Point
		want  assertion.Value
	}{
		{name: "as number", point: points[0], want: assertion.Type()},
		{name: "colon number", point: points[1], want: assertion.Type()},
		{name: "as any", point: points[2], want: assertion.Any()},
		{name: "colon any", point: points[3], want: assertion.Any()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mustLocalSource(t, facts, tc.point)
			assertLoweredAssertion(t, facts, source, tc.want, factflow.ValueSourceExpression)
		})
	}
	for _, point := range built.Graph.RPO() {
		if len(facts.BranchRefinements(point)) != 0 {
			t.Fatalf("parsed source cast emitted branch refinement at point %d", point)
		}
	}
}

func TestLowerClaimConditionsDoNotCreateBranchRefinements(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
if x as number then end
if x :: number then end
`)

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	cases := []struct {
		name   string
		index  int
		syntax ast.CastSyntax
	}{
		{name: "as condition", index: 1, syntax: ast.CastSyntaxAs},
		{name: "colon condition", index: 2, syntax: ast.CastSyntaxColonColon},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt := mustIfStmt(t, stmts, tc.index)
			point := requireStmtPoints(t, built, stmt, 1)[0]
			if len(facts.BranchRefinements(point)) != 0 {
				t.Fatalf("%s emitted branch refinement at point %d", tc.name, point)
			}

			branch, ok := result.BranchCondition(point)
			if !ok {
				t.Fatalf("missing branch condition at point %d", point)
			}
			cast, ok := branch.Source.Expr.(*ast.CastExpr)
			if !ok {
				t.Fatalf("branch source expr = %T, want *ast.CastExpr", branch.Source.Expr)
			}
			if cast.Syntax != tc.syntax {
				t.Fatalf("cast syntax = %v, want %v", cast.Syntax, tc.syntax)
			}

			branchLowerer := lowerer{registry: standard.Registry(), exprs: make(map[any]factflow.ExprRef)}
			branchInput := factflow.FactsInput{ExpressionRefinements: make(map[factflow.ExprRef]factflow.ExpressionRefinement)}
			branchLowerer.addAssertionRefinementsForSource(&branchInput, branch.Source)
			branchFacts := factflow.NewFacts(branchInput)
			branchSource := branchLowerer.valueSource(branch.Source)
			assertLoweredAssertion(t, branchFacts, branchSource, assertion.Type(), factflow.ValueSourceExpression)
		})
	}
}

func TestLowerParsedAnyClaimCastsMarkUntrustedTop(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b = x as any, x :: any
`)

	reg := standard.Registry()
	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 2)
	base := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	inputValues := make(map[factflow.ExprRef]product.Value)
	for _, point := range points {
		source := mustLocalSource(t, facts, point)
		refinement, ok := facts.ExpressionRefinement(source.ExprRef)
		if !ok {
			t.Fatalf("missing any claim refinement for source ref %d", source.ExprRef)
		}
		if got := product.Get(reg, refinement.Refinement(), assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("refinement assertion = %s, want any", got)
		}
		if got := product.Get(reg, refinement.Refinement(), evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("refinement evidence = %s, want explicit-top", got)
		}
		inputValues[refinement.Source().ExprRef] = base
	}

	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry:         reg,
			ExpressionValues: inputValues,
		}),
	})
	for _, point := range points {
		out := factApply(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
		fact, ok := facts.LocalAssignment(point)
		if !ok {
			t.Fatalf("missing local assignment at point %d", point)
		}
		assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
		want := product.Set(reg, base, assertion.Key, assertion.Any())
		want = product.Set(reg, want, evidence.Key, evidence.ExplicitTop())
		if !product.Equal(reg, assigned, want) {
			t.Fatalf("assigned value changed axes other than assertion.Any and explicit-top at point %d", point)
		}
		if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("assigned assertion = %s, want any", got)
		}
		if got := product.PresenceOf(assigned); !presence.Equal(got, presence.Present()) {
			t.Fatalf("assigned presence = %s, want present", got)
		}
		if got := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.Table)) {
			t.Fatalf("assigned runtime kind = %s, want table", got)
		}
		if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("assigned evidence = %s, want explicit-top", got)
		}
	}
}

func TestExtractedCastValueSourcesPreserveParsedSyntax(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b = x as number, x :: any
`)

	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 2)
	cases := []struct {
		name   string
		point  cfg.Point
		syntax ast.CastSyntax
	}{
		{name: "as", point: points[0], syntax: ast.CastSyntaxAs},
		{name: "colon", point: points[1], syntax: ast.CastSyntaxColonColon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fact, ok := result.LocalAssignment(tc.point)
			if !ok {
				t.Fatalf("missing local assignment at point %d", tc.point)
			}
			cast, ok := fact.Source.Expr.(*ast.CastExpr)
			if !ok {
				t.Fatalf("semantic source expr = %T, want *ast.CastExpr", fact.Source.Expr)
			}
			if cast.Syntax != tc.syntax {
				t.Fatalf("cast syntax = %v, want %v", cast.Syntax, tc.syntax)
			}
		})
	}
}

func TestLowerNestedClaimsPreserveOuterIdentityAndInnerFlow(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	read := ident("x")
	nonNil := &ast.NonNilAssertExpr{Expr: read}
	cast := &ast.CastExpr{Expr: nonNil, Type: primitiveType("number")}
	local := localAssign([]string{"a"}, cast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	source := mustLocalSource(t, facts, requireStmtPoints(t, built, local, 1)[0])
	outer, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing outer assertion for source %#v", source)
	}
	if got := refinementAssertion(t, outer); !assertion.Equal(got, assertion.Type()) {
		t.Fatalf("outer assertion = %s, want type", got)
	}
	innerSource := outer.Source()
	if innerSource.ExprRef == source.ExprRef || innerSource.ExprRef == 0 {
		t.Fatalf("outer assertion did not point at distinct inner expr ref: outer=%#v inner=%#v", source, innerSource)
	}
	inner, ok := facts.ExpressionRefinement(innerSource.ExprRef)
	if !ok {
		t.Fatalf("missing inner non-nil claim for source %#v", innerSource)
	}
	if got := refinementAssertion(t, inner); !assertion.Equal(got, assertion.NonNil()) {
		t.Fatalf("inner assertion = %s, want non-nil", got)
	}
}

func TestLowerClaimRefinementsApplyIndicatorsWithoutMutatingBaseValues(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeRead := ident("x")
	anyRead := ident("x")
	nonNilRead := ident("x")
	typeCast := &ast.CastExpr{Expr: typeRead, Type: primitiveType("number")}
	anyCast := &ast.CastExpr{Expr: anyRead, Type: primitiveType("any")}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilRead}
	local := localAssign([]string{"a", "b", "c"}, typeCast, anyCast, nonNil)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	reg, err := standard.RegistryWithAxes(testLowerSparseAxisSpec().Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes error = %v", err)
	}
	facts := lowerFacts(t, result, built.Graph, reg)
	points := requireStmtPoints(t, built, local, 3)
	inputValues := make(map[factflow.ExprRef]product.Value)
	type sourceCase struct {
		name              string
		point             cfg.Point
		base              product.Value
		wantClaim         assertion.Value
		wantPresence      presence.Value
		wantRuntimeKind   runtimekind.Value
		checkRuntimeKind  bool
		checkNoRefinement bool
		checkEvidence     bool
		wantEvidence      evidence.Value
	}
	cases := []sourceCase{
		{
			name:         "type",
			point:        points[0],
			base:         product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), testLowerSparseAxisKey, testLowerSparseAxisLow),
			wantClaim:    assertion.Type(),
			wantPresence: presence.Present(),
		},
		{
			name:              "any",
			point:             points[1],
			base:              product.Set(reg, product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), testLowerSparseAxisKey, testLowerSparseAxisLow), runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
			wantClaim:         assertion.Any(),
			wantPresence:      presence.Present(),
			wantRuntimeKind:   runtimekind.Singleton(runtimekind.Table),
			checkRuntimeKind:  true,
			checkNoRefinement: true,
			checkEvidence:     true,
			wantEvidence:      evidence.ExplicitTop(),
		},
		{
			name:         "non-nil",
			point:        points[2],
			base:         product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Absent()), testLowerSparseAxisKey, testLowerSparseAxisLow),
			wantClaim:    assertion.NonNil(),
			wantPresence: presence.Absent(),
		},
	}
	for i := range cases {
		source := mustLocalSource(t, facts, cases[i].point)
		refinement, ok := facts.ExpressionRefinement(source.ExprRef)
		if !ok {
			t.Fatalf("%s refinement missing", cases[i].name)
		}
		inputValues[refinement.Source().ExprRef] = cases[i].base
	}

	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry:         reg,
			ExpressionValues: inputValues,
		}),
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.checkRuntimeKind {
				if got := product.PresenceOf(tc.base); !presence.Equal(got, tc.wantPresence) {
					t.Fatalf("base input presence = %s, want %s", got, tc.wantPresence)
				}
				if got := product.Get(reg, tc.base, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("base input runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			out := factApply(transfer.NodeContext{Registry: reg, Point: tc.point}, state.State{})
			fact, ok := facts.LocalAssignment(tc.point)
			if !ok {
				t.Fatalf("missing local assignment at point %d", tc.point)
			}
			assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
			if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, tc.wantClaim) {
				t.Fatalf("assigned assertion = %s, want %s", got, tc.wantClaim)
			}
			if got := product.PresenceOf(assigned); !presence.Equal(got, tc.wantPresence) {
				t.Fatalf("assigned presence = %s, want %s", got, tc.wantPresence)
			}
			if tc.checkRuntimeKind {
				if got := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("assigned runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			if got := product.Get(reg, assigned, testLowerSparseAxisKey); got != testLowerSparseAxisLow {
				t.Fatalf("assigned sparse axis = %v, want %v", got, testLowerSparseAxisLow)
			}
			if tc.checkEvidence {
				if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, tc.wantEvidence) {
					t.Fatalf("assigned evidence = %s, want %s", got, tc.wantEvidence)
				}
			}
			if tc.checkNoRefinement {
				if len(facts.BranchRefinements(tc.point)) != 0 {
					t.Fatalf("%s assignment produced branch refinement", tc.name)
				}
			}
			if got := product.Get(reg, tc.base, assertion.Key); !assertion.Equal(got, assertion.Top()) {
				t.Fatalf("base value mutated with assertion = %s", got)
			}
			if tc.checkRuntimeKind {
				if got := product.Get(reg, tc.base, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("base runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			if got := product.Get(reg, tc.base, testLowerSparseAxisKey); got != testLowerSparseAxisLow {
				t.Fatalf("base sparse axis = %v, want %v", got, testLowerSparseAxisLow)
			}
		})
	}
}

func TestLowerNestedClaimRefinementsApplyCombinedIndicators(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	read := ident("x")
	nonNil := &ast.NonNilAssertExpr{Expr: read}
	cast := &ast.CastExpr{Expr: nonNil, Type: primitiveType("number")}
	local := localAssign([]string{"a"}, cast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	reg := standard.Registry()
	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	point := requireStmtPoints(t, built, local, 1)[0]
	source := mustLocalSource(t, facts, point)
	outer, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing outer assertion refinement")
	}
	inner, ok := facts.ExpressionRefinement(outer.Source().ExprRef)
	if !ok {
		t.Fatalf("missing inner assertion refinement")
	}
	base := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				inner.Source().ExprRef: base,
			},
		}),
	})

	out := factApply(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
	fact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment at point %d", point)
	}
	assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
	got := product.Get(reg, assigned, assertion.Key)
	if !got.Has(assertion.TypeClaim) || !got.Has(assertion.NonNilClaim) {
		t.Fatalf("nested assertion = %s, want type and non-nil indicators", got)
	}
	if got := product.Get(reg, base, assertion.Key); !assertion.Equal(got, assertion.Top()) {
		t.Fatalf("base value mutated with assertion = %s", got)
	}
}

func TestLowerNestedAnyClaimRefinementsKeepUntrustedTop(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b = (x as any) as number, (x :: any) :: number
`)

	reg := standard.Registry()
	facts := lowerFacts(t, result, built.Graph, reg)
	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 2)
	inputValues := make(map[factflow.ExprRef]product.Value)
	for _, point := range points {
		source := mustLocalSource(t, facts, point)
		outer, ok := facts.ExpressionRefinement(source.ExprRef)
		if !ok {
			t.Fatalf("missing outer assertion refinement for source ref %d", source.ExprRef)
		}
		assertClaimRefinementProduct(t, outer.Refinement(), assertion.Type())
		inner := outer.Source()
		innerRefinement, ok := facts.ExpressionRefinement(inner.ExprRef)
		if !ok {
			t.Fatalf("missing inner any assertion refinement for source ref %d", inner.ExprRef)
		}
		if got := product.Get(reg, innerRefinement.Refinement(), assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("inner assertion = %s, want any", got)
		}
		if got := product.Get(reg, innerRefinement.Refinement(), evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("inner evidence = %s, want explicit-top", got)
		}
		inputValues[innerRefinement.Source().ExprRef] = product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	}

	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry:         reg,
			ExpressionValues: inputValues,
		}),
	})
	for _, point := range points {
		out := factApply(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
		fact, ok := facts.LocalAssignment(point)
		if !ok {
			t.Fatalf("missing local assignment at point %d", point)
		}
		assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
		want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), assertion.Key, assertion.Of(assertion.TypeClaim, assertion.AnyClaim))
		want = product.Set(reg, want, evidence.Key, evidence.ExplicitTop())
		if !product.Equal(reg, assigned, want) {
			t.Fatalf("assigned value = %v, want nested type+any claim with explicit-top evidence", assigned)
		}
		if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("assigned evidence = %v, want explicit-top", got)
		}
	}
}

func TestLowerClaimWrappedCallPreservesProducerAndClaim(t *testing.T) {
	fooIdent := ident("foo")
	fooCall := &ast.FuncCallExpr{Func: fooIdent}
	fooCast := &ast.CastExpr{Expr: fooCall, Type: primitiveType("number")}
	local := localAssign([]string{"x"}, fooCast)
	barCall := &ast.FuncCallExpr{Func: ident("bar")}
	barCast := &ast.CastExpr{Expr: barCall, Type: primitiveType("string")}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{barCast}}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	readyCast := &ast.CastExpr{Expr: readyCall, Type: primitiveType("boolean")}
	ifStmt := &ast.IfStmt{Condition: readyCast}
	stmts := []ast.Stmt{local, ifStmt, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"foo", "bar", "ready"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatal("BuildChunk returned nil")
	}
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	localPoints := requireStmtPoints(t, built, local, 2)
	site, ok := facts.CallSite(localPoints[0])
	if !ok {
		t.Fatal("missing assertion-wrapped assignment call site")
	}
	innerRef, ok := site.Expr()
	if !ok || innerRef == 0 {
		t.Fatalf("inner call-site expr ref = %d/%v", innerRef, ok)
	}
	localSource := mustLocalSource(t, facts, localPoints[1])
	if localSource.Kind != factflow.ValueSourceCall || localSource.ExprRef == innerRef || localSource.CallPoint != localPoints[0] || !localSource.HasCallPoint {
		t.Fatalf("local wrapped call source = %#v, inner ref %d", localSource, innerRef)
	}
	claim, ok := facts.ExpressionRefinement(localSource.ExprRef)
	if !ok {
		t.Fatalf("missing assertion sidecar for outer ref %d", localSource.ExprRef)
	}
	if got := refinementAssertion(t, claim); !assertion.Equal(got, assertion.Type()) {
		t.Fatalf("outer assertion = %s, want type", got)
	}
	innerSource := claim.Source()
	if innerSource.Kind != factflow.ValueSourceCall || innerSource.ExprRef != innerRef || innerSource.CallPoint != localPoints[0] || !innerSource.HasCallPoint {
		t.Fatalf("assertion inner source = %#v, want call source ref %d at point %d", innerSource, innerRef, localPoints[0])
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnFact, ok := facts.Return(returnPoints[1])
	if !ok {
		t.Fatal("missing wrapped return fact")
	}
	returnSources := returnFact.Sources()
	if len(returnSources) != 1 || returnSources[0].Kind != factflow.ValueSourceCall || returnSources[0].CallPoint != returnPoints[0] || !returnSources[0].HasCallPoint {
		t.Fatalf("wrapped return source = %#v", returnSources)
	}
	assertLoweredAssertion(t, facts, returnSources[0], assertion.Type(), factflow.ValueSourceCall)

	ifPoints := requireStmtPoints(t, built, ifStmt, 2)
	branch, ok := result.BranchCondition(ifPoints[1])
	if !ok {
		t.Fatal("missing wrapped condition branch fact")
	}
	branchLowerer := lowerer{registry: standard.Registry(), exprs: make(map[any]factflow.ExprRef)}
	branchInput := factflow.FactsInput{ExpressionRefinements: make(map[factflow.ExprRef]factflow.ExpressionRefinement)}
	branchLowerer.addAssertionRefinementsForSource(&branchInput, branch.Source)
	branchFacts := factflow.NewFacts(branchInput)
	branchSource := branchLowerer.valueSource(branch.Source)
	if branchSource.Kind != factflow.ValueSourceCall || branchSource.CallPoint != ifPoints[0] || !branchSource.HasCallPoint {
		t.Fatalf("wrapped condition source = %#v", branchSource)
	}
	assertLoweredAssertion(t, branchFacts, branchSource, assertion.Type(), factflow.ValueSourceCall)
}

func TestLowerExpandedClaimWrappedCallKeepsPerResultSlotRefinements(t *testing.T) {
	cases := []struct {
		name string
		wrap func(*ast.FuncCallExpr) ast.Expr
		want assertion.Value
	}{
		{
			name: "cast",
			wrap: func(call *ast.FuncCallExpr) ast.Expr {
				return &ast.CastExpr{Expr: call, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
			},
			want: assertion.Type(),
		},
		{
			name: "non-nil",
			wrap: func(call *ast.FuncCallExpr) ast.Expr {
				return &ast.NonNilAssertExpr{Expr: call}
			},
			want: assertion.NonNil(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			makeCall := &ast.FuncCallExpr{Func: ident("make")}
			local := localAssign([]string{"a", "b"}, tc.wrap(makeCall))
			stmts := []ast.Stmt{local}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
			built := cfgbuild.BuildChunk(stmts, bindings)
			result, err := semantics.ExtractChunk(stmts, bindings, built)
			if err != nil {
				t.Fatalf("ExtractChunk: %v", err)
			}

			reg := standard.Registry()
			facts := lowerFacts(t, result, built.Graph, reg)
			points := requireStmtPoints(t, built, local, 3)
			site, ok := facts.CallSite(points[0])
			if !ok {
				t.Fatal("missing wrapped call site")
			}
			innerRef, ok := site.Expr()
			if !ok || innerRef == 0 {
				t.Fatalf("call-site expr ref = %d/%v", innerRef, ok)
			}

			firstSource := mustLocalSource(t, facts, points[1])
			secondSource := mustLocalSource(t, facts, points[2])
			if firstSource.ExprRef == secondSource.ExprRef {
				t.Fatalf("expanded wrapped call reused one outer source ref for both result slots: %#v %#v", firstSource, secondSource)
			}

			assertSlotRefinement := func(source factflow.ValueSource, resultIndex int) {
				t.Helper()
				refinement, ok := facts.ExpressionRefinement(source.ExprRef)
				if !ok {
					t.Fatalf("missing refinement for source ref %d", source.ExprRef)
				}
				assertClaimRefinementProduct(t, refinement.Refinement(), tc.want)
				inner := refinement.Source()
				if inner.Kind != factflow.ValueSourceCall || inner.ExprRef != innerRef || inner.ResultIndex != resultIndex || inner.CallPoint != points[0] || !inner.HasCallPoint {
					t.Fatalf("refinement source = %#v, want call ref %d result %d at point %d", inner, innerRef, resultIndex, points[0])
				}
			}
			assertSlotRefinement(firstSource, 0)
			assertSlotRefinement(secondSource, 1)

			firstValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
			secondValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
			transferFn := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
				Facts: facts,
				Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
					Registry: reg,
				}),
				CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
					if ctx.Point != points[0] {
						t.Fatalf("call result requested at point %d, want %d", ctx.Point, points[0])
					}
					return factapply.CallOutcome{
						Results: []factapply.CallResult{
							{Index: 0, Value: firstValue},
							{Index: 1, Value: secondValue},
						},
					}
				},
			})

			out := transferFn(transfer.NodeContext{Registry: reg, Point: points[1]}, state.State{})
			out = transferFn(transfer.NodeContext{Registry: reg, Point: points[2]}, out)
			firstFact, ok := facts.LocalAssignment(points[1])
			if !ok {
				t.Fatalf("missing first local assignment")
			}
			secondFact, ok := facts.LocalAssignment(points[2])
			if !ok {
				t.Fatalf("missing second local assignment")
			}
			firstAssigned := out.ReadValue(reg, key.SymbolValue(firstFact.TargetSymbol()))
			secondAssigned := out.ReadValue(reg, key.SymbolValue(secondFact.TargetSymbol()))
			if want := product.Set(reg, firstValue, assertion.Key, tc.want); !product.Equal(reg, firstAssigned, want) {
				t.Fatalf("first assigned value = %v, want first call result with claim", firstAssigned)
			}
			if want := product.Set(reg, secondValue, assertion.Key, tc.want); !product.Equal(reg, secondAssigned, want) {
				t.Fatalf("second assigned value = %v, want second call result with claim", secondAssigned)
			}
		})
	}
}
