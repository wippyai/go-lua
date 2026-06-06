package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

func TestInferReturnTypesUsesSummaryBeforeTypePipeline(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	calleeResolved := false

	got, ok := (ReturnInput{
		Call:  call,
		Ctx:   db.NewQueryContext(db.New()),
		Query: core.NewEngine(),
		SummaryReturns: func(gotCall *ast.FuncCallExpr, _ func(ast.Expr) typ.Type) []typ.Type {
			if gotCall != call {
				t.Fatal("summary callback saw wrong call")
			}
			return []typ.Type{typ.String}
		},
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				calleeResolved = true
				return typ.Func().Returns(typ.Boolean).Build()
			},
		},
	}).Types()
	if !ok || len(got) != 1 || got[0] != typ.String {
		t.Fatalf("InferReturnTypes summary = %#v, %v; want string, true", got, ok)
	}
	if calleeResolved {
		t.Fatal("callee pipeline ran before summary projection")
	}
}

func TestInferReturnTypesTopLikeSummaryYieldsToPipeline(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}

	got, ok := (ReturnInput{
		Call:  call,
		Ctx:   db.NewQueryContext(db.New()),
		Query: core.NewEngine(),
		SummaryReturns: func(*ast.FuncCallExpr, func(ast.Expr) typ.Type) []typ.Type {
			return []typ.Type{typ.Any}
		},
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				return typ.Func().Returns(typ.String).Build()
			},
		},
	}).Types()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("InferReturnTypes top-like summary fallback = %#v, %v; want string, true", got, ok)
	}
}

func TestInferReturnTypesPipelineReturns(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "f"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	fn := typ.Func().Param("x", typ.Number).Returns(typ.String, typ.Boolean).Build()

	got, ok := (ReturnInput{
		Call:     call,
		ArgTypes: []typ.Type{typ.Number},
		Ctx:      db.NewQueryContext(db.New()),
		Query:    core.NewEngine(),
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				return fn
			},
		},
	}).Types()
	if !ok || len(got) != 2 || got[0] != typ.String || got[1] != typ.Boolean {
		t.Fatalf("InferReturnTypes pipeline = %#v, %v; want string, boolean, true", got, ok)
	}
}

func TestInferReturnTypesClosesGenericIdentityFromArg(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "identity"},
		Args: []ast.Expr{&ast.StringExpr{Value: "test"}},
	}
	tp := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(tp).
		Param("x", tp).
		Returns(tp).
		Build()

	got, ok := (ReturnInput{
		Call:     call,
		ArgTypes: []typ.Type{typ.LiteralString("test")},
		Ctx:      db.NewQueryContext(db.New()),
		Query:    core.NewEngine(),
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr == call.Func {
					return fn
				}
				return typ.Unknown
			},
		},
	}).Types()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("InferReturnTypes generic identity = %#v/%v, want string", got, ok)
	}
}

func TestInferReturnTypesClosesNestedGenericRecordReturnFromArg(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "make_box"},
		Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
	}
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp},
		typ.NewRecord().
			Field("value", tp).
			Field("get", typ.Func().OptParam("self", typ.Self).Returns(tp).Build()).
			Build(),
	)
	fn := typ.Func().
		TypeParamRef(tp).
		Param("value", tp).
		Returns(typ.Instantiate(box, tp)).
		Build()

	got, ok := (ReturnInput{
		Call:     call,
		ArgTypes: []typ.Type{typ.LiteralString("hello")},
		Ctx:      db.NewQueryContext(db.New()),
		Query:    core.NewEngine(),
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr == call.Func {
					return fn
				}
				return typ.Unknown
			},
		},
	}).Types()
	if !ok || len(got) != 1 {
		t.Fatalf("InferReturnTypes generic box = %#v/%v, want one return", got, ok)
	}
	expanded := subst.ExpandInstantiated(got[0])
	rec, ok := expanded.(*typ.Record)
	if !ok {
		t.Fatalf("InferReturnTypes generic box = %v expanded %v, want record", got[0], expanded)
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	getFn, ok := get.Type.(*typ.Function)
	if !ok || len(getFn.Returns) != 1 || !typ.TypeEquals(getFn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestInferReturnTypesMethodReceiverInterfaceReturns(t *testing.T) {
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "db"},
		Method:   "execute",
		Args:     []ast.Expr{&ast.StringExpr{Value: "select 1"}},
	}
	result := typ.NewRecord().
		Field("rows_affected", typ.Integer).
		Build()
	receiver := typ.NewInterface("Transaction", []typ.Method{{
		Name: "execute",
		Type: typ.Func().
			Param("self", typ.Self).
			Param("sql", typ.String).
			Returns(result).
			Build(),
	}})

	got, ok := (ReturnInput{
		Call:               call,
		ArgTypes:           []typ.Type{typ.LiteralString("select 1")},
		Ctx:                db.NewQueryContext(db.New()),
		Query:              core.NewEngine(),
		MethodReceiverType: receiver,
		Resolver:           TypeResolver{},
	}).Types()
	if !ok || len(got) != 1 {
		t.Fatalf("InferReturnTypes method returns = %#v, %v; want one record", got, ok)
	}
	field, ok := core.Field(got[0], "rows_affected")
	if !ok || !typ.TypeEquals(field, typ.Integer) {
		t.Fatalf("rows_affected = %v/%v, want integer; return=%v", field, ok, got[0])
	}
}

func TestInferReturnTypesAnyCalleeReturnsGradualAnyType(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "dynamic"}}
	args := []typ.Type{nil}

	got, ok := (ReturnInput{
		Call:     call,
		ArgTypes: args,
		Ctx:      db.NewQueryContext(db.New()),
		Query:    core.NewEngine(),
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				return typ.Any
			},
		},
	}).Types()
	if !ok || len(got) != 1 || !typ.IsAny(got[0]) {
		t.Fatalf("InferReturnTypes any callee = %#v, %v; want any, true", got, ok)
	}
	if args[0] != nil {
		t.Fatal("InferReturnTypes mutated caller arg slice while normalizing nil")
	}
}

func TestInferReturnTypesVoidFunctionYieldsNoReturn(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "void"}}
	got, ok := (ReturnInput{
		Call:  call,
		Ctx:   db.NewQueryContext(db.New()),
		Query: core.NewEngine(),
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				return typ.Func().Build()
			},
		},
	}).Types()
	if ok || got != nil {
		t.Fatalf("InferReturnTypes void = %#v, %v; want nil, false", got, ok)
	}
}

func TestTypeCastTargetAndInterceptShareEnvironment(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "UserId"},
		Args: []ast.Expr{&ast.StringExpr{Value: "u1"}},
	}
	castFn := typ.Func().
		Param("value", typ.Any).
		Returns(typ.String).
		Effects(effect.WithCallableType()).
		Build()
	env := InterceptEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "UserId" {
				return castFn
			}
			return nil
		},
	}

	target, ok := InferTypeCastTarget(call, env)
	if !ok || target != typ.String {
		t.Fatalf("InferTypeCastTarget = %v, %v; want string, true", target, ok)
	}
	returns, ok := interceptReturnTypes(call, env)
	if !ok || len(returns) != 1 || returns[0] != typ.String {
		t.Fatalf("interceptReturnTypes = %#v, %v; want string, true", returns, ok)
	}
}

func TestGradualDynamicReturnValue(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "dynamic"}}
	got, ok := gradualDynamicReturnValue(call, func(expr ast.Expr) (product.AbstractValue, bool) {
		if ident, ok := expr.(*ast.IdentExpr); !ok || ident.Value != "dynamic" {
			t.Fatalf("unexpected gradual call expr: %#v", expr)
		}
		return product.GradualAny(), true
	})
	if !ok || !got.IsGradualTop() {
		t.Fatalf("gradualDynamicReturnValue = %v, %v; want gradual top, true", got, ok)
	}
}

func TestInferReturnValuesInterceptBeatsSummary(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "UserId"},
		Args: []ast.Expr{&ast.StringExpr{Value: "u1"}},
	}
	castFn := typ.Func().
		Param("value", typ.Any).
		Returns(typ.String).
		Effects(effect.WithCallableType()).
		Build()
	summaryUsed := false

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		Env: InterceptEnv{
			TypeLookup: func(name string) typ.Type {
				if name == "UserId" {
					return castFn
				}
				return nil
			},
		},
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			summaryUsed = true
			return []product.AbstractValue{product.FromType(typ.Boolean)}
		},
	}).Values()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), typ.String) {
		t.Fatalf("InferReturnValues intercept = %#v, %v; want string, true", got, ok)
	}
	if summaryUsed {
		t.Fatal("summary ran before intercept")
	}
}

func TestInferReturnValuesSummaryBeatsGradualAndTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "dynamic"}}
	typeFallbackUsed := false

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(typ.String)}
		},
		ExprValue: func(ast.Expr) (product.AbstractValue, bool) {
			return product.GradualAny(), true
		},
		TypeFallback: func() ([]typ.Type, bool) {
			typeFallbackUsed = true
			return []typ.Type{typ.Boolean}, true
		},
	}).Values()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), typ.String) {
		t.Fatalf("InferReturnValues summary = %#v, %v; want string, true", got, ok)
	}
	if got[0].IsGradualTop() {
		t.Fatal("gradual fallback overrode summary return")
	}
	if typeFallbackUsed {
		t.Fatal("type fallback ran after summary return")
	}
}

func TestInferReturnValuesSkipsRecursiveFamilyFallbackScan(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "build_graph"}}
	node := typ.NewRecursivePlaceholder("Node")
	var tower typ.Type = typ.NewAlias("NodeAlias", typ.NewRecord().
		Field("next", node).
		Build())
	for i := 0; i < 512; i++ {
		tower = typ.NewAlias("TowerAlias", typ.NewMap(typ.String, typ.NewOptional(typ.NewUnion(tower, typ.Nil))))
	}
	node.SetBody(typ.NewRecord().
		Field("next", tower).
		Field("hole", typ.Unknown).
		Build())
	typeFallbackUsed := false

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(tower)}
		},
		TypeFallback: func() ([]typ.Type, bool) {
			typeFallbackUsed = true
			return []typ.Type{typ.String}, true
		},
	}).Values()

	if !ok || len(got) != 1 || got[0].IsZero() {
		t.Fatalf("InferReturnValues recursive summary = %#v, %v; want summary value", got, ok)
	}
	if typeFallbackUsed {
		t.Fatal("recursive family return forced type fallback")
	}
}

func TestInferReturnValuesSkipsOversizedStructuralFallbackScan(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "build"}}
	var tower typ.Type = typ.String
	for i := 0; i < summaryReturnFallbackScanLimit+16; i++ {
		tower = typ.NewMap(typ.String, typ.NewOptional(tower))
	}
	typeFallbackUsed := false

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(tower)}
		},
		TypeFallback: func() ([]typ.Type, bool) {
			typeFallbackUsed = true
			return []typ.Type{typ.String}, true
		},
	}).Values()

	if !ok || len(got) != 1 || got[0].IsZero() {
		t.Fatalf("InferReturnValues oversized summary = %#v, %v; want summary value", got, ok)
	}
	if typeFallbackUsed {
		t.Fatal("oversized structural summary forced type fallback")
	}
}

func TestInferReturnValuesRefinesStructuralSummaryWithTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "select"}}
	summary := typ.NewRecord().
		Field(effect.SelectResultChannelField, typ.Any).
		Field(effect.SelectResultValueField, typ.Unknown).
		Field("ok", typ.Boolean).
		Build()
	stringChannel := typ.NewInterface("Channel<string>", nil)
	numberChannel := typ.NewInterface("Channel<number>", nil)
	refined := typ.NewUnion(
		typ.NewRecord().
			Field(effect.SelectResultChannelField, stringChannel).
			Field(effect.SelectResultValueField, typ.String).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field(effect.SelectResultChannelField, numberChannel).
			Field(effect.SelectResultValueField, typ.Number).
			Field("ok", typ.Boolean).
			Build(),
	)

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(summary)}
		},
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{refined}, true
		},
	}).Values()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), refined) {
		t.Fatalf("InferReturnValues refined structural summary = %#v, %v; want %v, true", got, ok, refined)
	}
}

func TestInferReturnValuesRepairsOpenGenericLeafWithInstantiatedFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "make_box"}}
	open := typ.NewTypeParam("T", nil)
	summary := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(open).Build()).
		Build()
	boxParam := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typ.NewRecord().
			Field("value", boxParam).
			Field("get", typ.Func().OptParam("self", typ.Self).Returns(boxParam).Build()).
			Build(),
	)
	instantiated := typ.Instantiate(box, typ.String)
	if expanded := subst.ExpandInstantiated(instantiated); expanded == instantiated {
		t.Fatalf("ExpandInstantiated(%v) did not expand", instantiated)
	}

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(summary)}
		},
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{instantiated}, true
		},
	}).Values()
	if !ok || len(got) != 1 {
		t.Fatalf("InferReturnValues generic fallback = %#v, %v; want one value", got, ok)
	}
	rec, ok := got[0].ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("return value = %v, want record", got[0].ProjectValue())
	}
	value := rec.GetField("value")
	if value == nil || !typ.TypeEquals(value.Type, typ.LiteralString("hello")) {
		t.Fatalf("value field = %#v, want literal hello", value)
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	fn, ok := get.Type.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestInferReturnValuesRepairsOpenGenericLeafWithExpandedFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "make_box"}}
	open := typ.NewTypeParam("T", nil)
	summary := typ.NewRecord().
		Field("value", typ.LiteralString("hello")).
		Field("get", typ.Func().OptParam("self", typ.Any).Returns(open).Build()).
		Build()
	fallback := typ.NewRecord().
		Field("value", typ.String).
		Field("get", typ.Func().OptParam("self", typ.Self).Returns(typ.String).Build()).
		Build()

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(summary)}
		},
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{fallback}, true
		},
	}).Values()
	if !ok || len(got) != 1 {
		t.Fatalf("InferReturnValues expanded fallback = %#v, %v; want one value", got, ok)
	}
	rec, ok := got[0].ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("return value = %v, want record", got[0].ProjectValue())
	}
	value := rec.GetField("value")
	if value == nil || !typ.TypeEquals(value.Type, typ.LiteralString("hello")) {
		t.Fatalf("value field = %#v, want literal hello", value)
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	fn, ok := get.Type.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestInferReturnValuesTopLikeSummaryYieldsToTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		SummaryReturnValues: func(*ast.FuncCallExpr) []product.AbstractValue {
			return []product.AbstractValue{product.FromType(typ.Any)}
		},
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{typ.String}, true
		},
	}).Values()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), typ.String) {
		t.Fatalf("InferReturnValues top-like summary fallback = %#v, %v; want string, true", got, ok)
	}
}

func TestInferReturnValuesGradualBeatsTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "dynamic"}}
	typeFallbackUsed := false

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		ExprValue: func(ast.Expr) (product.AbstractValue, bool) {
			return product.GradualAny(), true
		},
		TypeFallback: func() ([]typ.Type, bool) {
			typeFallbackUsed = true
			return []typ.Type{typ.String}, true
		},
	}).Values()
	if !ok || len(got) != 1 || !got[0].IsGradualTop() {
		t.Fatalf("InferReturnValues gradual = %#v, %v; want gradual top, true", got, ok)
	}
	if typeFallbackUsed {
		t.Fatal("type fallback ran before gradual dynamic return")
	}
}

func TestInferReturnValuesTypeFallbackProjectsTypes(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{typ.String, typ.Boolean}, true
		},
	}).Values()
	if !ok || len(got) != 2 ||
		!typ.TypeEquals(got[0].ProjectValue(), typ.String) ||
		!typ.TypeEquals(got[1].ProjectValue(), typ.Boolean) {
		t.Fatalf("InferReturnValues type fallback = %#v, %v; want string, boolean, true", got, ok)
	}
}

func TestInferReturnValuesPendingInputAllowsInformativeTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	gradualUsed := false

	got, ok := (ReturnValueInput{
		Call:                call,
		TypePolicyAvailable: true,
		PendingInput:        true,
		ExprValue: func(ast.Expr) (product.AbstractValue, bool) {
			gradualUsed = true
			return product.GradualAny(), true
		},
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{typ.String, typ.NewOptional(typ.Boolean)}, true
		},
	}).Values()
	if !ok || len(got) != 2 ||
		!typ.TypeEquals(got[0].ProjectValue(), typ.String) ||
		!typ.TypeEquals(got[1].ProjectValue(), typ.NewOptional(typ.Boolean)) {
		t.Fatalf("pending informative fallback = %#v, %v; want string, boolean?, true", got, ok)
	}
	if gradualUsed {
		t.Fatal("pending input used gradual dynamic fallback before stable type fallback")
	}
}

func TestInferReturnValuesPendingInputRejectsTopLikeTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}

	for name, returns := range map[string][]typ.Type{
		"any":        {typ.Any},
		"unknown":    {typ.Unknown},
		"type-param": {typ.NewTypeParam("T", nil)},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := (ReturnValueInput{
				Call:                call,
				TypePolicyAvailable: true,
				PendingInput:        true,
				ExprValue: func(ast.Expr) (product.AbstractValue, bool) {
					return product.GradualAny(), true
				},
				TypeFallback: func() ([]typ.Type, bool) {
					return returns, true
				},
			}).Values()
			if ok || got != nil {
				t.Fatalf("pending top-like fallback = %#v, %v; want nil, false", got, ok)
			}
		})
	}
}

func TestInformativeReturnValueTypeUsesTypeParamFlags(t *testing.T) {
	closed := typ.NewRecord().
		Field("graph", typ.NewRecord().
			Field("nodes", typ.NewMap(typ.String, typ.Number)).
			Field("edges", typ.NewMap(typ.String, typ.Boolean)).
			Build()).
		Build()
	if !informativeReturnValueType(closed) {
		t.Fatalf("closed structural return reported uninformative: %v", closed)
	}

	tp := typ.NewTypeParam("T", nil)
	open := typ.NewRecord().Field("value", tp).Build()
	if informativeReturnValueType(open) {
		t.Fatalf("open type-param return reported informative: %v", open)
	}
	if informativeReturnValueType(typ.NewRef("", "Later")) {
		t.Fatal("deferred ref return reported informative")
	}
}

func TestInferReturnValuesSelectedTargetBlocksGradualAnySeed(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	gradualUsed := false

	got, ok := (ReturnValueInput{
		Call:                 call,
		TypePolicyAvailable:  true,
		BlockDynamicFallback: true,
		ExprValue: func(ast.Expr) (product.AbstractValue, bool) {
			gradualUsed = true
			return product.GradualAny(), true
		},
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{typ.Any}, true
		},
	}).Values()
	if !ok || len(got) != 1 || !product.Domain.Equal(got[0], product.Bottom()) {
		t.Fatalf("selected target top-like fallback = %#v, %v; want bottom, true", got, ok)
	}
	if gradualUsed {
		t.Fatal("selected local target used gradual dynamic fallback before summary convergence")
	}
}

func TestInferReturnValuesSelectedTargetAllowsInformativeTypeFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}

	got, ok := (ReturnValueInput{
		Call:                 call,
		TypePolicyAvailable:  true,
		BlockDynamicFallback: true,
		TypeFallback: func() ([]typ.Type, bool) {
			return []typ.Type{typ.Number}, true
		},
	}).Values()
	if !ok || len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), typ.Number) {
		t.Fatalf("selected target informative fallback = %#v, %v; want number, true", got, ok)
	}
}

func TestInferReturnValuesTypePolicyUnavailableSkipsInterceptAndFallback(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "UserId"}}
	usedFallback := false
	got, ok := (ReturnValueInput{
		Call: call,
		Env: InterceptEnv{
			TypeLookup: func(string) typ.Type {
				return typ.Func().Returns(typ.String).Effects(effect.WithCallableType()).Build()
			},
		},
		TypeFallback: func() ([]typ.Type, bool) {
			usedFallback = true
			return []typ.Type{typ.String}, true
		},
	}).Values()
	if ok || got != nil {
		t.Fatalf("InferReturnValues without type policy = %#v, %v; want nil, false", got, ok)
	}
	if usedFallback {
		t.Fatal("type fallback ran while type policy unavailable")
	}
}

func TestApplySpecReturnOverrideUsesTypeCases(t *testing.T) {
	fn := typ.Func().Returns(typ.Boolean).Build()
	fn.Spec = &contract.Spec{
		Return: &contract.ReturnSpec{
			Cases: []contract.ReturnCase{
				{When: constraint.TrueCondition(), Type: typ.String},
			},
		},
	}
	got := applySpecReturnOverride(SpecReturnInput{
		Call:    &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}},
		Callee:  fn,
		Returns: []typ.Type{typ.Boolean},
	})
	if len(got) != 1 || got[0] != typ.String {
		t.Fatalf("applySpecReturnOverride = %#v; want string", got)
	}
}
