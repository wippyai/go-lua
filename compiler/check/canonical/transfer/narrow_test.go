package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNarrowAtPath_DynamicNestedScalarProofPinsNestedField(t *testing.T) {
	segs := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "retry"},
		{Kind: constraint.SegmentField, Name: "max_attempts"},
	}
	got, ok := narrowAtPath(product.FromType(typ.Any), segs, cfg.CheckTypeEqual, "number")
	if !ok {
		t.Fatal("expected nested scalar proof under any to refine the dynamic overlay")
	}
	root := got.ProjectValue()
	retry, ok := querycore.Field(root, "retry")
	if !ok {
		t.Fatalf("refined root %s has no retry field", typ.FormatShort(root))
	}
	attempts, ok := querycore.Field(retry, "max_attempts")
	if !ok || !typ.TypeEquals(attempts, typ.Number) {
		t.Fatalf("retry.max_attempts = %s, %v; want number,true", typ.FormatShort(attempts), ok)
	}
	other, ok := querycore.Field(root, "other")
	if !ok || !typ.IsAny(other) {
		t.Fatalf("unproven dynamic field = %s, %v; want any,true", typ.FormatShort(other), ok)
	}
}

func TestNarrowAtPath_DynamicPresenceProofDoesNotPinNestedField(t *testing.T) {
	segs := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "retry"},
		{Kind: constraint.SegmentField, Name: "max_attempts"},
	}
	if got, ok := narrowAtPath(product.FromType(typ.Any), segs, cfg.CheckNotNil, ""); ok {
		t.Fatalf("presence proof should not pin a dynamic scalar field, got %s", typ.FormatShort(got.ProjectValue()))
	}
}

func TestNarrowAtPath_DynamicTableProofDoesNotPinConcreteSlot(t *testing.T) {
	segs := []constraint.Segment{{Kind: constraint.SegmentField, Name: "retry"}}
	if got, ok := narrowAtPath(product.FromType(typ.Any), segs, cfg.CheckTypeEqual, "table"); ok {
		t.Fatalf("table proof should leave dynamic field unassignable, got %s", typ.FormatShort(got.ProjectValue()))
	}
}

func TestNarrowByCondCheckTruthyAnyMarksPresentDynamic(t *testing.T) {
	ident := &ast.IdentExpr{Value: "last_template_node_id"}
	sym := cfg.SymbolID(21)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		ident: sym,
	}), Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(sym): product.FromType(typ.Any),
	}}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition:  ident,
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	gotAV, ok := tr.symbolValue(&got, sym)
	if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.Any) || !gotAV.DefinitelyPresent() {
		t.Fatalf("truthy any edge value = %v/%v present=%v; want present any", gotAV.ProjectValue(), ok, gotAV.DefinitelyPresent())
	}
}

func TestNarrowByCondCheckTypeEqualAnyNarrowsToKind(t *testing.T) {
	ident := &ast.IdentExpr{Value: "x"}
	sym := cfg.SymbolID(22)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		ident: sym,
	}), Config{})
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTypeEqual, TypeName: "number"},
		Condition:  ident,
	}

	for name, base := range map[string]product.AbstractValue{
		"strict-any":  product.FromType(typ.Any),
		"gradual-any": product.GradualAny(),
	} {
		t.Run(name, func(t *testing.T) {
			out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
				flow.SymbolValueKey(sym): base,
			}}
			got := tr.narrowByCondCheck(out, info, true, false)
			gotAV, ok := tr.symbolValue(&got, sym)
			if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.Number) {
				t.Fatalf("type(x)==number edge value = %v/%v, want number,true", gotAV.ProjectValue(), ok)
			}
			if base.IsGradualTop() && !gotAV.IsGradualTop() {
				t.Fatal("type guard over gradual source lost gradual evidence")
			}
		})
	}
}

func TestNarrowByCondCheckDeclaredUnionIgnoresInitializerSingletonVeto(t *testing.T) {
	ident := &ast.IdentExpr{Value: "x"}
	sym := cfg.SymbolID(23)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		ident: sym,
	}), Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{
		sym: typ.NewUnion(typ.String, typ.Number),
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.LiteralInt(42)),
		},
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondVar:    "x",
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTypeEqual, TypeName: "string"},
		Condition:  ident,
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	gotAV, ok := tr.symbolValue(&got, sym)
	if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.String) {
		t.Fatalf("declared union type(x)==string edge = %v/%v, want string", gotAV.ProjectValue(), ok)
	}
}

func TestNarrowByCondCheckDeclaredUnionLetsPathConditionedCurrentSpecialize(t *testing.T) {
	ident := &ast.IdentExpr{Value: "x"}
	sym := cfg.SymbolID(24)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		ident: sym,
	}), Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{
		sym: typ.NewUnion(typ.String, typ.Number),
	}
	path := constraint.NewPath(sym, "x")
	out := flow.PointState{
		Cond: constraint.FromConstraints(constraint.HasType{
			Path: path,
			Type: typeKeyFor("number"),
		}),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.LiteralInt(42)),
		},
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondVar:    "x",
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTypeEqual, TypeName: "number"},
		Condition:  ident,
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	gotAV, ok := tr.symbolValue(&got, sym)
	if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.LiteralInt(42)) {
		t.Fatalf("condition-backed current value = %v/%v, want literal 42", gotAV.ProjectValue(), ok)
	}
}

func TestNarrowByCondCheckContradictoryTypeConditionBottomsEdge(t *testing.T) {
	ident := &ast.IdentExpr{Value: "x"}
	sym := cfg.SymbolID(25)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		ident: sym,
	}), Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{
		sym: typ.NewUnion(typ.String, typ.Number),
	}
	path := constraint.NewPath(sym, "x")
	out := flow.PointState{
		Cond: constraint.FromConstraints(constraint.HasType{
			Path: path,
			Type: typeKeyFor("number"),
		}),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Number),
		},
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondVar:    "x",
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTypeEqual, TypeName: "string"},
		Condition:  ident,
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	if !flow.PointStateDomain.Equal(got, flow.PointStateDomain.Bottom()) {
		t.Fatalf("contradictory typeof edge = %#v, want point-state bottom", got)
	}
}

func TestNarrowByPredicateDirectCallTrueEdgeNarrowsArgument(t *testing.T) {
	fnIdent := &ast.IdentExpr{Value: "is_positive_number"}
	argIdent := &ast.IdentExpr{Value: "value"}
	fnSym := cfg.SymbolID(21)
	argSym := cfg.SymbolID(22)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		fnIdent:  fnSym,
		argIdent: argSym,
	}), Config{
		PredicateFacts: []guard.PredicateFunction{
			{FuncSym: fnSym, ParamIndex: 0, Kind: "number"},
		},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(argSym): product.FromType(typ.Any),
	}}
	info := &cfg.BranchInfo{
		CondCheck: cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition: &ast.FuncCallExpr{
			Func: fnIdent,
			Args: []ast.Expr{argIdent},
		},
	}

	got := tr.narrowByPredicate(out, info, true)
	gotAV, ok := tr.symbolValue(&got, argSym)
	if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.Number) {
		t.Fatalf("true edge value = %v/%v, want number,true", gotAV.ProjectValue(), ok)
	}

	falseEdge := tr.narrowByPredicate(out, info, false)
	falseAV, ok := tr.symbolValue(&falseEdge, argSym)
	if !ok || !typ.TypeEquals(falseAV.ProjectValue(), typ.Any) {
		t.Fatalf("false edge value = %v/%v, want any,true", falseAV.ProjectValue(), ok)
	}
}

func TestNarrowByPredicateAssignedResultTrueEdgeNarrowsArgument(t *testing.T) {
	okIdent := &ast.IdentExpr{Value: "ok"}
	okSym := cfg.SymbolID(31)
	argSym := cfg.SymbolID(32)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		okIdent: okSym,
	}), Config{
		PredicateGuards: []guard.PredicateResult{
			{CondSym: okSym, NarrowSym: argSym, Kind: "number"},
		},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(argSym): product.FromType(typ.Any),
	}}
	info := &cfg.BranchInfo{
		CondSymbol: okSym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition:  okIdent,
	}

	got := tr.narrowByPredicate(out, info, true)
	gotAV, ok := tr.symbolValue(&got, argSym)
	if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.Number) {
		t.Fatalf("true edge value = %v/%v, want number,true", gotAV.ProjectValue(), ok)
	}

	falseEdge := tr.narrowByPredicate(out, info, false)
	falseAV, ok := tr.symbolValue(&falseEdge, argSym)
	if !ok || !typ.TypeEquals(falseAV.ProjectValue(), typ.Any) {
		t.Fatalf("false edge value = %v/%v, want any,true", falseAV.ProjectValue(), ok)
	}
}

func TestNarrowEdgePredicateDirectCallTrueSuccessorNarrowsArgument(t *testing.T) {
	fnIdent := &ast.IdentExpr{Value: "is_positive_number"}
	argIdent := &ast.IdentExpr{Value: "value"}
	guardExpr := &ast.FuncCallExpr{
		Func: fnIdent,
		Args: []ast.Expr{argIdent},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"value"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: guardExpr,
				Then: []ast.Stmt{
					&ast.LocalAssignStmt{Names: []string{"n"}, Exprs: []ast.Expr{argIdent}},
				},
			},
		},
	}
	in := input.BuildFromFunction(fn, nil, nil, "is_positive_number")
	if in.Graph == nil {
		t.Fatal("test graph not built")
	}
	fnSym, ok := in.Graph.Bindings().SymbolOf(fnIdent)
	if !ok || fnSym == 0 {
		t.Fatalf("predicate callee was not bound: %d/%v", fnSym, ok)
	}
	argSym, ok := in.Graph.Bindings().SymbolOf(argIdent)
	if !ok || argSym == 0 {
		t.Fatalf("predicate argument was not bound: %d/%v", argSym, ok)
	}
	var branch cfg.Point
	var trueSucc cfg.Point
	in.Graph.EachBranch(func(p cfg.Point, _ *cfg.BranchInfo) {
		if branch != 0 {
			return
		}
		branch = p
		for _, succ := range in.Graph.Successors(p) {
			taken, known := in.Graph.EdgeCond(p, succ)
			if known && taken {
				trueSucc = succ
				break
			}
		}
	})
	if trueSucc == 0 {
		t.Fatal("test CFG has no true branch edge")
	}
	tr := New(in, Config{
		PredicateFacts: []guard.PredicateFunction{
			{FuncSym: fnSym, ParamIndex: 0, Kind: "number"},
		},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(argSym): product.FromType(typ.Any),
	}}

	got := tr.NarrowEdge(in.Graph, branch, trueSucc, out)
	gotAV, ok := tr.symbolValue(&got, argSym)
	if !ok || !typ.TypeEquals(gotAV.ProjectValue(), typ.Number) {
		t.Fatalf("true successor value = %v/%v, want number,true", gotAV.ProjectValue(), ok)
	}
}

func TestNarrowEdgeTruthyAliasLocalTrueSuccessorNarrowsReceiver(t *testing.T) {
	store := &ast.IdentExpr{Value: "store"}
	callRecv := &ast.IdentExpr{Value: "store"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"store"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: store,
				Then: []ast.Stmt{
					&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Receiver: callRecv, Method: "go"}},
				},
			},
		},
	}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph not built")
	}
	storeSym := in.Scope.ParamSymbols[0]
	var branch cfg.Point
	var trueSucc cfg.Point
	in.Graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if branch != 0 || info == nil || info.CondSymbol != storeSym {
			return
		}
		branch = p
		for _, succ := range in.Graph.Successors(p) {
			taken, known := in.Graph.EdgeCond(p, succ)
			if known && taken {
				trueSucc = succ
				break
			}
		}
	})
	if trueSucc == 0 {
		t.Fatal("test CFG has no true branch edge")
	}
	svc := typ.NewAlias("Svc", typ.NewRecord().
		Field("go", typ.Func().Build()).
		Build())
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(storeSym): product.FromType(typ.NewOptional(svc)),
	}}

	got := tr.NarrowEdge(in.Graph, branch, trueSucc, out)
	gotAV, ok := tr.symbolValue(&got, storeSym)
	if !ok || !gotAV.DefinitelyPresent() {
		t.Fatalf("true successor value = %v/%v present=%v, want present alias receiver", gotAV.ProjectValue(), ok, gotAV.DefinitelyPresent())
	}
}

func TestNarrowEdgeTruthyCopiedAliasLocalTrueSuccessorNarrowsReceiver(t *testing.T) {
	h := &ast.IdentExpr{Value: "h"}
	source := &ast.AttrGetExpr{Object: h, Key: &ast.StringExpr{Value: "store"}, KeySyntax: ast.AttrKeyDot}
	condStore := &ast.IdentExpr{Value: "store"}
	callStore := &ast.IdentExpr{Value: "store"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"h"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"store"}, Exprs: []ast.Expr{source}},
			&ast.IfStmt{
				Condition: condStore,
				Then: []ast.Stmt{
					&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Receiver: callStore, Method: "go"}},
				},
			},
		},
	}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph not built")
	}
	storeSym, ok := in.Graph.Bindings().SymbolOf(condStore)
	if !ok || storeSym == 0 {
		t.Fatalf("store condition was not bound: %d/%v", storeSym, ok)
	}
	var branch cfg.Point
	var trueSucc cfg.Point
	in.Graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if branch != 0 || info == nil || info.CondSymbol != storeSym {
			return
		}
		branch = p
		for _, succ := range in.Graph.Successors(p) {
			taken, known := in.Graph.EdgeCond(p, succ)
			if known && taken {
				trueSucc = succ
				break
			}
		}
	})
	if trueSucc == 0 {
		t.Fatal("test CFG has no true branch edge")
	}
	svc := typ.NewAlias("Svc", typ.NewRecord().
		Field("go", typ.Func().Build()).
		Build())
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(storeSym): product.FromType(typ.NewOptional(svc)),
	}}

	got := tr.NarrowEdge(in.Graph, branch, trueSucc, out)
	gotAV, ok := tr.symbolValue(&got, storeSym)
	if !ok || !gotAV.DefinitelyPresent() {
		t.Fatalf("true successor value = %v/%v present=%v, want present copied alias receiver", gotAV.ProjectValue(), ok, gotAV.DefinitelyPresent())
	}
}

func TestConditionRefinedCaptureValueNarrowsExistingCell(t *testing.T) {
	sym := cfg.SymbolID(10)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Cells: flow.CaptureCellsDomain.Bottom().With(sym, product.GradualAny()),
		Cond: constraint.FromConstraints(constraint.HasType{
			Path: constraint.NewPath(sym, "x"),
			Type: typeKeyFor("number"),
		}),
	}
	base, ok := out.Cells.Value(sym)
	if !ok {
		t.Fatal("test cell missing")
	}

	got, refined := tr.conditionRefinedCaptureValue(&out, sym, base, true)
	if !refined || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("conditionRefinedCaptureValue = %v/%v, want number refinement", got.ProjectValue(), refined)
	}
}

func TestPathSymbolUsesSharedStaticAccessProjection(t *testing.T) {
	sym := cfg.SymbolID(15)
	root := &ast.IdentExpr{Value: "messages"}
	dot := &ast.AttrGetExpr{
		Object:    root,
		Key:       &ast.StringExpr{Value: "root"},
		KeySyntax: ast.AttrKeyDot,
	}
	index := &ast.AttrGetExpr{
		Object:    root,
		Key:       &ast.StringExpr{Value: "root"},
		KeySyntax: ast.AttrKeyIndex,
	}
	nested := &ast.AttrGetExpr{
		Object:    index,
		Key:       &ast.NumberExpr{Value: "1"},
		KeySyntax: ast.AttrKeyIndex,
	}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{root: sym}), Config{})

	gotSym, gotDot, ok := tr.pathSymbol(dot)
	if !ok || gotSym != sym || len(gotDot) != 1 || gotDot[0].Kind != constraint.SegmentField || gotDot[0].Name != "root" {
		t.Fatalf("dot pathSymbol = %d %#v %v, want field segment", gotSym, gotDot, ok)
	}
	gotSym, gotIndex, ok := tr.pathSymbol(index)
	if !ok || gotSym != sym || len(gotIndex) != 1 || gotIndex[0].Kind != constraint.SegmentIndexString || gotIndex[0].Name != "root" {
		t.Fatalf("index pathSymbol = %d %#v %v, want string-index segment", gotSym, gotIndex, ok)
	}
	gotSym, gotNested, ok := tr.pathSymbol(nested)
	if !ok || gotSym != sym || len(gotNested) != 2 ||
		gotNested[0].Kind != constraint.SegmentIndexString || gotNested[0].Name != "root" ||
		gotNested[1].Kind != constraint.SegmentIndexInt || gotNested[1].Index != 1 {
		t.Fatalf("nested pathSymbol = %d %#v %v, want shared static-access projection", gotSym, gotNested, ok)
	}
}

func TestPathSymbolInStateAtNormalizesConstIndexKey(t *testing.T) {
	sym := cfg.SymbolID(16)
	keySym := cfg.SymbolID(17)
	p := cfg.Point(4)
	root := &ast.IdentExpr{Value: "obj"}
	key := &ast.IdentExpr{Value: "key"}
	index := &ast.AttrGetExpr{
		Object:    root,
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{root: sym, key: keySym})
	in.ConstValues = map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue{
		keySym: {p: {Kind: flow.ConstString, Str: "p-q"}},
	}
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(sym):    product.FromType(typ.NewRecord().Build()),
		flow.SymbolValueKey(keySym): product.FromType(typ.String),
	}}

	gotSym, gotSegs, ok := tr.pathSymbolInStateAt(&out, p, index, nil)
	if !ok || gotSym != sym || len(gotSegs) != 1 ||
		gotSegs[0].Kind != constraint.SegmentIndexString || gotSegs[0].Name != "p-q" {
		t.Fatalf("pathSymbolInStateAt const key = %d %#v %v, want obj[\"p-q\"]", gotSym, gotSegs, ok)
	}
}

func TestTypedDiscriminantNarrowsInterfacePayloadChannels(t *testing.T) {
	resultSym := cfg.SymbolID(21)
	inboxSym := cfg.SymbolID(22)
	result := &ast.IdentExpr{Value: "result"}
	inbox := &ast.IdentExpr{Value: "inbox_ch"}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		result: resultSym,
		inbox:  inboxSym,
	}), Config{})

	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	channelParam := typ.NewTypeParam("T", nil)
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{channelParam}, typ.NewInterface("channel.Channel", nil))
	messageChannel := typ.Instantiate(channelGeneric, messageType)
	eventChannel := typ.Instantiate(channelGeneric, eventType)
	messageResult := typ.NewRecord().
		Field("channel", messageChannel).
		Field("value", messageType).
		Build()
	eventResult := typ.NewRecord().
		Field("channel", eventChannel).
		Field("value", eventType).
		Build()
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(resultSym): product.FromType(typ.NewUnion(messageResult, eventResult)),
		flow.SymbolValueKey(inboxSym):  product.FromType(messageChannel),
	}}
	guard := &ast.RelationalOpExpr{
		Lhs: &ast.AttrGetExpr{
			Object:    result,
			Key:       &ast.StringExpr{Value: "channel"},
			KeySyntax: ast.AttrKeyDot,
		},
		Operator: "==",
		Rhs:      inbox,
	}

	got, applied := tr.narrowByTypedDiscriminant(out, &cfg.BranchInfo{Condition: guard}, true)
	if !applied {
		t.Fatal("typed discriminant did not apply for interface payload channels")
	}
	av, ok := tr.symbolValue(&got, resultSym)
	if !ok {
		t.Fatal("narrowed result value missing")
	}
	if !typ.TypeEquals(av.ProjectValue(), messageResult) {
		t.Fatalf("result after result.channel == inbox_ch = %v, want %v", av.ProjectValue(), messageResult)
	}
}

func TestApplyParamInequalityNarrowsTypedDiscriminantPath(t *testing.T) {
	resultSym := cfg.SymbolID(31)
	chSym := cfg.SymbolID(32)
	result := &ast.IdentExpr{Value: "result"}
	ch := &ast.IdentExpr{Value: "ch1"}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		result: resultSym,
		ch:     chSym,
	}), Config{})

	ch1 := typ.NewRecord().Field("__tag", typ.LiteralString("one")).Build()
	ch2 := typ.NewRecord().Field("__tag", typ.LiteralString("two")).Build()
	case1 := typ.NewRecord().Field("channel", ch1).Field("value", typ.String).Build()
	case2 := typ.NewRecord().Field("channel", ch2).Field("value", typ.Number).Build()
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(resultSym): product.FromType(typ.NewUnion(case1, case2)),
		flow.SymbolValueKey(chSym):     product.FromType(ch1),
	}}
	call := &ast.FuncCallExpr{Args: []ast.Expr{
		&ast.AttrGetExpr{
			Object:    result,
			Key:       &ast.StringExpr{Value: "channel"},
			KeySyntax: ast.AttrKeyDot,
		},
		ch,
	}}

	tr.ApplyParamNarrows(&out, call, []ParamNarrow{{Param: 0, EqParam: 1, NotEqual: true}})
	av, ok := tr.symbolValue(&out, resultSym)
	if !ok {
		t.Fatal("narrowed result value missing")
	}
	if !typ.TypeEquals(av.ProjectValue(), case2) {
		t.Fatalf("result after asserted result.channel ~= ch1 = %v, want %v", av.ProjectValue(), case2)
	}
}

func TestApplyParamNarrowWritesConditionAxisForStaticPath(t *testing.T) {
	pageSym := cfg.SymbolID(41)
	page := &ast.IdentExpr{Value: "page"}
	proxy := &ast.AttrGetExpr{
		Object:    page,
		Key:       &ast.StringExpr{Value: "proxy"},
		KeySyntax: ast.AttrKeyDot,
	}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		page: pageSym,
	}), Config{})
	out := flow.PointState{}
	call := &ast.FuncCallExpr{Args: []ast.Expr{proxy}}

	if dead := tr.ApplyParamNarrows(&out, call, []ParamNarrow{{Param: 0, Check: cfg.CheckNotNil, EqParam: -1}}); dead {
		t.Fatal("not_nil(page.proxy) should not kill an unconstrained continuation")
	}

	wantPath := constraint.Path{
		Root:   "page",
		Symbol: pageSym,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "proxy"},
		},
	}
	want := constraint.FromConstraints(constraint.NotNil{Path: wantPath})
	if !constraint.Domain.Equal(out.Cond, want) {
		t.Fatalf("condition after not_nil(page.proxy) = %v, want %v", out.Cond, want)
	}
}

func TestApplyParamNarrowDeadWhenPostconditionImpossible(t *testing.T) {
	sym := cfg.SymbolID(42)
	x := &ast.IdentExpr{Value: "x"}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		x: sym,
	}), Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Nil),
		},
	}
	call := &ast.FuncCallExpr{Args: []ast.Expr{x}}

	if dead := tr.ApplyParamNarrows(&out, call, []ParamNarrow{{Param: 0, Check: cfg.CheckNotNil, EqParam: -1}}); !dead {
		t.Fatal("not_nil(x) over nil-only x should kill the continuation")
	}
}

func TestNarrowLengthGuardRefinesContainerAndLiteralIndexRead(t *testing.T) {
	sym := cfg.SymbolID(18)
	rows := &ast.IdentExpr{Value: "rows"}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		rows: sym,
	}), Config{})
	row := typ.NewRecord().Field("text", typ.String).Build()
	rowsArray := typ.NewArray(row)
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.NewUnion(typ.NewRecord().Build(), rowsArray, typ.Nil)),
		},
	}
	guard := &ast.RelationalOpExpr{
		Lhs:      &ast.UnaryLenOpExpr{Expr: rows},
		Operator: ">",
		Rhs:      &ast.NumberExpr{Value: "0"},
	}
	info := &cfg.BranchInfo{
		CondCheck: cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition: guard,
	}

	got, applied := tr.narrowLengthGuard(out, guard, info, true)
	if !applied {
		t.Fatal("length guard did not apply")
	}
	lower, _, ok := got.Num.LenBoundsFor(constraint.PathKey(flow.SymbolValueKey(sym)))
	if !ok || lower != 1 {
		t.Fatalf("length lower bound = %d/%v, want 1,true", lower, ok)
	}
	av, ok := tr.symbolValue(&got, sym)
	if !ok {
		t.Fatalf("guarded rows missing, want %v", rowsArray)
	}
	if !typ.TypeEquals(av.ProjectValue(), rowsArray) {
		t.Fatalf("guarded rows = %v, want %v", av.ProjectValue(), rowsArray)
	}

	elem, ok := tr.evalAttrGet(&got, &ast.AttrGetExpr{
		Object:    rows,
		Key:       &ast.NumberExpr{Value: "1"},
		KeySyntax: ast.AttrKeyIndex,
	}, nil)
	if !ok || !typ.TypeEquals(elem.ProjectValue(), row) {
		t.Fatalf("rows[1] after #rows>0 = %v/%v, want %v,true", elem.ProjectValue(), ok, row)
	}
}

func TestReadFieldPathDispatchesBySegmentKind(t *testing.T) {
	oldResolver := fieldResolver
	fieldResolver = &querycore.FuncResolver{
		FieldFunc: func(_ typ.Type, name string) (typ.Type, bool) {
			if name == "foo" {
				return typ.String, true
			}
			return nil, false
		},
		IndexFunc: func(_ typ.Type, key typ.Type) (typ.Type, bool) {
			switch {
			case typ.TypeEquals(key, typ.LiteralString("foo")):
				return typ.Number, true
			case typ.TypeEquals(key, typ.LiteralString("")):
				return typ.Boolean, true
			case typ.TypeEquals(key, typ.LiteralInt(1)):
				return typ.Integer, true
			default:
				return nil, false
			}
		},
	}
	defer func() { fieldResolver = oldResolver }()

	base := typ.NewRecord().Build()
	cases := []struct {
		name string
		seg  constraint.Segment
		want typ.Type
	}{
		{
			name: "field",
			seg:  constraint.Segment{Kind: constraint.SegmentField, Name: "foo"},
			want: typ.String,
		},
		{
			name: "string index",
			seg:  constraint.Segment{Kind: constraint.SegmentIndexString, Name: "foo"},
			want: typ.Number,
		},
		{
			name: "empty string index",
			seg:  constraint.Segment{Kind: constraint.SegmentIndexString, Name: ""},
			want: typ.Boolean,
		},
		{
			name: "integer index",
			seg:  constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1},
			want: typ.Integer,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := readFieldPath(base, []constraint.Segment{tt.seg})
			if !ok || !typ.TypeEquals(got, tt.want) {
				t.Fatalf("readFieldPath(%v) = %v, %v; want %v,true", tt.seg, got, ok, tt.want)
			}
		})
	}
}

func TestRefineStaticMemberFactForPositiveGuard(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(9)
	segs := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}}
	base := product.FromType(typ.NewMap(typ.String, typ.String))
	out := flow.PointState{}

	tr.refineStaticMemberFactForCheck(&out, sym, segs, base, true, cfg.CheckNotNil, "")
	got, ok := testStaticMemberValue(t, out.StaticMembers, sym, segs)
	if !ok || !got.DefinitelyPresent() || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("guard fact = %v, %v; want present string", got.ProjectValue(), ok)
	}
}

func TestRefineStaticMemberFactRebasesExistingSentinelOnCurrentBase(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(9)
	segs := []constraint.Segment{{Kind: constraint.SegmentField, Name: "data_func"}}
	pathKey := flow.SymbolPathKey(sym, segs)
	base := product.FromType(typ.NewRecord().
		Field("data_func", typ.NewUnion(typ.String, typ.Nil, typ.False)).
		Build())
	out := flow.PointState{
		StaticMembers: flow.StaticMemberFacts{}.WithAddress(testStaticMemberAddressKey(t, pathKey), product.FromType(typ.True)),
	}

	tr.refineStaticMemberFactForCheck(&out, sym, segs, base, true, cfg.CheckTruthy, "")
	got, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, pathKey))
	if !ok || !got.DefinitelyPresent() || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("rebased guard fact = %v, %v; want present string", got.ProjectValue(), ok)
	}
}

func TestStaticMemberLiteralExclusionCollapsesExactCachedPath(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(9)
	segs := []constraint.Segment{{Kind: constraint.SegmentField, Name: "data_func"}}
	pathKey := flow.SymbolPathKey(sym, segs)
	out := flow.PointState{
		StaticMembers: flow.StaticMemberFacts{}.WithAddress(testStaticMemberAddressKey(t, pathKey), product.FromType(typ.LiteralString(""))),
	}

	if !tr.refineStaticMemberFactForLiteralComparison(&out, sym, segs, typ.LiteralString(""), false) {
		t.Fatal("literal exclusion reported no change")
	}
	if !flow.PointStateDomain.Equal(out, flow.PointStateDomain.Bottom()) {
		t.Fatalf("literal exclusion left exact cached path reachable: %s", out.StaticMembers.Format())
	}
}

func TestRefineStaticMemberFactKeepsFalsyOnlyFromExistingPresentFact(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(9)
	segs := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "flag"}}
	pathKey := flow.SymbolPathKey(sym, segs)
	out := flow.PointState{}

	tr.refineStaticMemberFactForCheck(&out, sym, segs, product.AbstractValue{}, false, cfg.CheckFalsy, "")
	if _, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, pathKey)); ok {
		t.Fatal("falsy guard without prior present fact must not invent a member fact")
	}

	out.StaticMembers = out.StaticMembers.WithAddress(testStaticMemberAddressKey(t, pathKey), product.FromType(typ.Boolean))
	tr.refineStaticMemberFactForCheck(&out, sym, segs, product.AbstractValue{}, false, cfg.CheckFalsy, "")
	got, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, pathKey))
	if !ok || !got.DefinitelyPresent() || !typ.TypeEquals(got.ProjectValue(), typ.False) {
		t.Fatalf("falsy existing fact = %v, %v; want present false", got.ProjectValue(), ok)
	}
}

func TestNarrowByCondCheckInstallsFactOnFalseEdgeOfNotIndexGuard(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(12)
	base := &ast.IdentExpr{Value: "messages"}
	guard := &ast.AttrGetExpr{
		Object:    base,
		Key:       &ast.StringExpr{Value: "root"},
		KeySyntax: ast.AttrKeyIndex,
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckFalsy},
		Condition:  &ast.UnaryNotOpExpr{Expr: guard},
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.NewMap(typ.String, typ.String)),
		},
	}

	got := tr.narrowByCondCheck(out, info, false, false)
	pathKey := flow.SymbolPathKey(sym, []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "root"}})
	fact, ok := got.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, pathKey))
	if !ok || !fact.DefinitelyPresent() || !typ.TypeEquals(fact.ProjectValue(), typ.String) {
		t.Fatalf("false edge fact = %v, %v; want present string", fact.ProjectValue(), ok)
	}
}

func TestNarrowByCondCheckMissingStaticMemberPositiveGuardKeepsDynamicEdge(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(14)
	base := &ast.IdentExpr{Value: "prev"}
	config := &ast.AttrGetExpr{
		Object:    base,
		Key:       &ast.StringExpr{Value: "config"},
		KeySyntax: ast.AttrKeyDot,
	}
	guard := &ast.AttrGetExpr{
		Object:    config,
		Key:       &ast.StringExpr{Value: "data_targets"},
		KeySyntax: ast.AttrKeyDot,
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckFalsy},
		Condition:  &ast.UnaryNotOpExpr{Expr: guard},
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.NewRecord().
				Field("config", typ.NewRecord().
					Field("kind", typ.String).
					Build()).
				Build()),
		},
	}

	got := tr.narrowByCondCheck(out, info, false, false)
	root, ok := got.Env[flow.SymbolValueKey(sym)]
	if !ok || valueIsBottom(root) {
		t.Fatalf("positive guard on dynamic table member killed reachable root: %v, %v", root.ProjectValue(), ok)
	}
	path := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "config"},
		{Kind: constraint.SegmentField, Name: "data_targets"},
	}
	fact, ok := testStaticMemberValue(t, got.StaticMembers, sym, path)
	if !ok || !fact.DefinitelyPresent() {
		t.Fatalf("dynamic missing-member guard fact = %v, %v; want present dynamic", fact.ProjectValue(), ok)
	}
}

func TestNarrowByCondCheckTruthyFieldDropsArrayAlternative(t *testing.T) {
	entryIdent := &ast.IdentExpr{Value: "entry"}
	sym := cfg.SymbolID(24)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		entryIdent: sym,
	}), Config{})
	entryRecord := typ.NewRecord().Field("id", typ.String).Build()
	entry := typ.NewAlias("Entry", typ.NewUnion(typ.String, entryRecord))
	entryArray := typ.NewArray(entry)
	declared := typ.NewUnion(entry, entryArray)
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: declared}
	guard := &ast.AttrGetExpr{
		Object:    entryIdent,
		Key:       &ast.StringExpr{Value: "id"},
		KeySyntax: ast.AttrKeyDot,
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition:  guard,
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(declared),
		},
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	root, ok := tr.symbolValue(&got, sym)
	if !ok {
		t.Fatal("narrowed root missing")
	}
	gotType := root.ProjectValue()
	if typ.IsNever(gotType) || !typ.TypeEquals(gotType, entryRecord) {
		t.Fatalf("truthy entry.id root = %v, want %v", gotType, entryRecord)
	}
}

func TestNarrowByCondCheckDoesNotWidenCurrentEdgeWithDeclaredBase(t *testing.T) {
	entryIdent := &ast.IdentExpr{Value: "entry"}
	sym := cfg.SymbolID(25)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		entryIdent: sym,
	}), Config{})
	entryRecord := typ.NewRecord().Field("id", typ.String).Build()
	entry := typ.NewAlias("Entry", typ.NewUnion(typ.String, entryRecord))
	entryArray := typ.NewArray(entry)
	declared := typ.NewUnion(entry, entryArray)
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: declared}
	guard := &ast.AttrGetExpr{
		Object:    entryIdent,
		Key:       &ast.StringExpr{Value: "id"},
		KeySyntax: ast.AttrKeyDot,
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		CondVar:    "entry.id",
		Condition:  guard,
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(entryRecord),
		},
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	root, ok := tr.symbolValue(&got, sym)
	if !ok {
		t.Fatal("narrowed root missing")
	}
	gotType := root.ProjectValue()
	if !typ.TypeEquals(gotType, entryRecord) {
		t.Fatalf("truthy entry.id widened current edge value to %v, want %v", gotType, entryRecord)
	}
}

func TestNarrowByCondCheckFieldPresenceFalseEdgeDropsPresentOnlyVariant(t *testing.T) {
	outcomeIdent := &ast.IdentExpr{Value: "outcome"}
	sym := cfg.SymbolID(26)
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		outcomeIdent: sym,
	}), Config{})
	accepted := typ.NewAlias("Accepted", typ.NewRecord().
		Field("id", typ.String).
		Field("attempt", typ.Number).
		Build())
	rejected := typ.NewAlias("Rejected", typ.NewRecord().
		Field("id", typ.String).
		Field("reason", typ.String).
		Build())
	decision := typ.NewAlias("Decision", typ.NewUnion(accepted, rejected))
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: decision}
	guard := &ast.AttrGetExpr{
		Object:    outcomeIdent,
		Key:       &ast.StringExpr{Value: "reason"},
		KeySyntax: ast.AttrKeyDot,
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		CondVar:    "outcome.reason",
		Condition:  guard,
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(decision),
		},
	}

	got := tr.narrowByCondCheck(out, info, false, false)
	root, ok := tr.symbolValue(&got, sym)
	if !ok {
		t.Fatal("narrowed root missing")
	}
	gotType := root.ProjectValue()
	if !typ.TypeEquals(gotType, accepted) {
		t.Fatalf("falsy outcome.reason root = %v, want %v", gotType, accepted)
	}
}

func TestApplyParamCondNarrowCarriesFullNarrowedProductState(t *testing.T) {
	sym := cfg.SymbolID(13)
	base := &ast.IdentExpr{Value: "messages"}
	arg := &ast.AttrGetExpr{
		Object:    base,
		Key:       &ast.StringExpr{Value: "root"},
		KeySyntax: ast.AttrKeyIndex,
	}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		base: sym,
	}), Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.NewMap(typ.String, typ.String)),
		},
	}

	tr.applyParamCondNarrow(&out, arg, cfg.CheckTruthy)

	pathKey := flow.SymbolPathKey(sym, []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "root"}})
	fact, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, pathKey))
	if !ok || !fact.DefinitelyPresent() || !typ.TypeEquals(fact.ProjectValue(), typ.String) {
		t.Fatalf("forwarded condition fact = %v, %v; want present string", fact.ProjectValue(), ok)
	}
}

func TestNarrowByCondCheckKeepsClosedEntryValueForOpenGenericDeclaredBase(t *testing.T) {
	const sym = cfg.SymbolID(19)
	resultIdent := &ast.IdentExpr{Value: "result"}
	guard := &ast.AttrGetExpr{
		Object:    resultIdent,
		Key:       &ast.StringExpr{Value: "ok"},
		KeySyntax: ast.AttrKeyDot,
	}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		resultIdent: sym,
	}), Config{})

	tp := typ.NewTypeParam("T", nil)
	openSuccess := typ.NewRecord().
		Field("ok", typ.True).
		Field("value", tp).
		Build()
	openFailure := typ.NewRecord().
		Field("ok", typ.False).
		Field("error", typ.String).
		Build()
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{
		sym: typ.NewUnion(openSuccess, openFailure),
	}

	envelope := typ.NewRecord().Field("id", typ.String).Build()
	closedSuccess := typ.NewRecord().
		Field("ok", typ.True).
		Field("value", envelope).
		Build()
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(closedSuccess),
		},
	}
	info := &cfg.BranchInfo{
		CondSymbol: sym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition:  guard,
	}

	got := tr.narrowByCondCheck(out, info, true, false)
	av, ok := tr.symbolValue(&got, sym)
	if !ok {
		t.Fatalf("narrowed state lost result symbol: env=%v cells=%s", got.Env, got.Cells.Format())
	}
	value, ok := querycore.Field(av.ProjectValue(), "value")
	if !ok || !typ.TypeEquals(value, envelope) {
		t.Fatalf("result.value after truthy ok guard = %v, %v; want Envelope", value, ok)
	}
}

func TestNarrowAndFalseEdgeUnionsSameSymbolLiteralRefinement(t *testing.T) {
	sym := cfg.SymbolID(21)
	lhs := &ast.IdentExpr{Value: "v"}
	rhsIdent := &ast.IdentExpr{Value: "v"}
	guard := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      lhs,
		Rhs: &ast.RelationalOpExpr{
			Lhs:      rhsIdent,
			Operator: "~=",
			Rhs:      &ast.StringExpr{Value: ""},
		},
	}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		lhs:      sym,
		rhsIdent: sym,
	}), Config{})
	setTransferParamSlotsForTest(tr, sym)
	out := flow.PointStateDomain.Bottom()
	out.Env = map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(sym): product.FromType(typ.NewUnion(typ.String, typ.Boolean, typ.Nil)),
	}

	got, applied := tr.narrowAndUnionFalseEdge(out, guard)
	if !applied {
		t.Fatal("same-symbol `A and B` false edge did not apply a union refinement")
	}
	av, ok := got.Env[flow.SymbolValueKey(sym)]
	if !ok {
		t.Fatalf("narrowed state lost symbol %d: env=%v cells=%s", sym, got.Env, got.Cells.Format())
	}
	want := typ.NewUnion(typ.Nil, typ.False, typ.LiteralString(""))
	if projected := av.ProjectValue(); !typ.TypeEquals(projected, want) {
		t.Fatalf("false-edge value = %v, want %v", projected, want)
	}
}

func TestNarrowByTypedDiscriminantUsesDeclaredComparedValue(t *testing.T) {
	resultSym := cfg.SymbolID(31)
	channelSym := cfg.SymbolID(32)
	resultIdent := &ast.IdentExpr{Value: "result"}
	channelIdent := &ast.IdentExpr{Value: "ch"}
	tr := New(keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		resultIdent:  resultSym,
		channelIdent: channelSym,
	}), Config{})

	chInt := typ.NewRecord().Field("__tag", typ.LiteralString("int")).Build()
	chStr := typ.NewRecord().Field("__tag", typ.LiteralString("str")).Build()
	intCase := typ.NewRecord().Field("channel", chInt).Field("value", typ.Number).Build()
	strCase := typ.NewRecord().Field("channel", chStr).Field("value", typ.String).Build()
	resultUnion := typ.NewUnion(intCase, strCase)
	tr.declaredTypes[channelSym] = chInt
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(resultSym):  product.FromType(resultUnion),
			flow.SymbolValueKey(channelSym): product.FromType(typ.NewRecord().Field("__tag", typ.String).Build()),
		},
	}
	info := &cfg.BranchInfo{
		CondCheck: cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition: &ast.RelationalOpExpr{
			Lhs: &ast.AttrGetExpr{
				Object:    resultIdent,
				Key:       &ast.StringExpr{Value: "channel"},
				KeySyntax: ast.AttrKeyDot,
			},
			Operator: "==",
			Rhs:      channelIdent,
		},
	}

	got, applied := tr.narrowByTypedDiscriminant(out, info, true)
	if !applied {
		t.Fatal("typed discriminant did not apply using declared compared value")
	}
	av, ok := tr.symbolValue(&got, resultSym)
	if !ok {
		t.Fatalf("narrowed state lost result symbol: env=%v cells=%s", got.Env, got.Cells.Format())
	}
	if projected := av.ProjectValue(); !typ.TypeEquals(projected, intCase) {
		t.Fatalf("typed discriminant result = %v, want %v", projected, intCase)
	}
}

func TestNarrowEdgeTypedDiscriminantUsesRealCFGEdge(t *testing.T) {
	resultUse := &ast.IdentExpr{Value: "result"}
	channelUse := &ast.IdentExpr{Value: "ch"}
	guard := &ast.RelationalOpExpr{
		Lhs: &ast.AttrGetExpr{
			Object:    resultUse,
			Key:       &ast.StringExpr{Value: "channel"},
			KeySyntax: ast.AttrKeyDot,
		},
		Operator: "==",
		Rhs:      channelUse,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"result", "ch"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: guard,
				Then: []ast.Stmt{
					&ast.LocalAssignStmt{Names: []string{"hit"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
				},
				Else: []ast.Stmt{
					&ast.LocalAssignStmt{Names: []string{"miss"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}}},
				},
			},
		},
	}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph not built")
	}
	resultSym, ok := in.Graph.Bindings().SymbolOf(resultUse)
	if !ok {
		t.Fatal("result use was not bound")
	}
	channelSym, ok := in.Graph.Bindings().SymbolOf(channelUse)
	if !ok {
		t.Fatal("channel use was not bound")
	}
	var branch cfg.Point
	var trueSucc cfg.Point
	in.Graph.EachBranch(func(p cfg.Point, _ *cfg.BranchInfo) {
		if branch != 0 {
			return
		}
		branch = p
		for _, succ := range in.Graph.Successors(p) {
			taken, known := in.Graph.EdgeCond(p, succ)
			if known && taken {
				trueSucc = succ
				break
			}
		}
	})
	if trueSucc == 0 {
		t.Fatalf("test CFG has no true branch edge")
	}

	chInt := typ.NewRecord().Field("__tag", typ.LiteralString("int")).Build()
	chStr := typ.NewRecord().Field("__tag", typ.LiteralString("str")).Build()
	intCase := typ.NewRecord().Field("channel", chInt).Field("value", typ.NewRecord().Field("error", typ.String).Build()).Build()
	strCase := typ.NewRecord().Field("channel", chStr).Field("value", typ.NewRecord().Field("data", typ.Number).Build()).Build()
	resultUnion := typ.NewUnion(intCase, strCase)
	in.Scope.DeclaredTypes = map[cfg.SymbolID]typ.Type{
		resultSym:  resultUnion,
		channelSym: chInt,
	}
	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(resultSym): product.FromType(resultUnion),
		},
	}

	got := tr.NarrowEdge(in.Graph, branch, trueSucc, out)
	av, ok := tr.symbolValue(&got, resultSym)
	if !ok {
		t.Fatalf("narrowed state lost result symbol: env=%v cells=%s", got.Env, got.Cells.Format())
	}
	if projected := av.ProjectValue(); !typ.TypeEquals(projected, intCase) {
		t.Fatalf("real CFG edge typed discriminant result = %v, want %v", projected, intCase)
	}
}

func TestNarrowEdgePathEqualityUsesVariantOriginConditionAxis(t *testing.T) {
	resultUse := &ast.IdentExpr{Value: "result"}
	timeoutUse := &ast.IdentExpr{Value: "timeout"}
	guard := &ast.RelationalOpExpr{
		Lhs: &ast.AttrGetExpr{
			Object:    resultUse,
			Key:       &ast.StringExpr{Value: effect.SelectResultChannelField},
			KeySyntax: ast.AttrKeyDot,
		},
		Operator: "==",
		Rhs:      timeoutUse,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"result", "timeout"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: guard,
				Then: []ast.Stmt{
					&ast.LocalAssignStmt{Names: []string{"hit"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
				},
			},
		},
	}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph not built")
	}
	resultSym, ok := in.Graph.Bindings().SymbolOf(resultUse)
	if !ok {
		t.Fatal("result use was not bound")
	}
	timeoutSym, ok := in.Graph.Bindings().SymbolOf(timeoutUse)
	if !ok {
		t.Fatal("timeout use was not bound")
	}
	var branch cfg.Point
	var trueSucc cfg.Point
	in.Graph.EachBranch(func(p cfg.Point, _ *cfg.BranchInfo) {
		if branch != 0 {
			return
		}
		branch = p
		for _, succ := range in.Graph.Successors(p) {
			taken, known := in.Graph.EdgeCond(p, succ)
			if known && taken {
				trueSucc = succ
				break
			}
		}
	})
	if trueSucc == 0 {
		t.Fatalf("test CFG has no true branch edge")
	}
	originFamily := uint64(777)
	in.VariantFieldOrigins = []flow.VariantFieldOrigin{{
		Target:       constraint.Path{Root: "result", Symbol: resultSym},
		Field:        effect.SelectResultChannelField,
		Source:       constraint.Path{Root: "timeout", Symbol: timeoutSym},
		OriginFamily: originFamily,
		CaseIndex:    1,
	}}
	tr := New(in, Config{})

	base := product.FromType(typ.String)
	got := tr.NarrowEdge(in.Graph, branch, trueSucc, flow.PointState{
		Cond: constraint.TrueCondition(),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(resultSym): product.WithVariantOrigin(base, originFamily, []int{0, 1}),
		},
	})
	if !conditionHasVariantCaseEquals(got.Cond, resultSym, originFamily, 1) {
		t.Fatalf("narrowed condition = %#v, want variant-origin case proof", got.Cond)
	}
	av, ok := tr.symbolValue(&got, resultSym)
	if !ok {
		t.Fatalf("narrowed state lost result symbol: env=%v cells=%s", got.Env, got.Cells.Format())
	}
	want := product.WithVariantOrigin(base, originFamily, []int{1})
	if !product.Equal(av, want) {
		t.Fatalf("variant-origin product reduction = %#v, want %#v", av, want)
	}
}

func conditionHasVariantCaseEquals(cond constraint.Condition, targetSym cfg.SymbolID, family uint64, caseIndex int) bool {
	for _, c := range cond.AllConstraints() {
		eq, ok := c.(constraint.VariantCaseEquals)
		if !ok {
			continue
		}
		if eq.Target.Symbol == targetSym && eq.OriginFamily == family && eq.CaseIndex == caseIndex {
			return true
		}
	}
	return false
}
