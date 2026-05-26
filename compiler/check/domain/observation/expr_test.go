package observation

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type factsStub map[cfg.SymbolID]typ.Type

func (f factsStub) DeclaredAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return typedValueForTest(f[sym])
}

func (f factsStub) RefinedAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

func (f factsStub) EffectiveTypeAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return typedValueForTest(f[sym])
}

func (f factsStub) IsAnnotated(cfg.SymbolID) bool {
	return false
}

func typedValueForTest(t typ.Type) flow.TypedValue {
	if t == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}

func TestProjector_IdentUsesSolvedFacts(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 10
	bindings.Bind(ident, sym)

	observed := New(Config{
		Bindings: bindings,
		Facts:    factsStub{sym: typ.String},
	}).TypeOf(ident, 1)

	if !typ.TypeEquals(observed, typ.String) {
		t.Fatalf("TypeOf(ident) = %v, want string", observed)
	}
}

func TestProjector_NumberLiteralUsesCanonicalPrecision(t *testing.T) {
	observed := New(Config{}).TypeOf(&ast.NumberExpr{Value: "42"}, 1)
	lit, ok := observed.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("TypeOf(integer literal) = %T %[1]v, want integer literal", observed)
	}
}

func TestProjector_ExpectedNumberLiteralUsesContext(t *testing.T) {
	observed := New(Config{}).TypeOfWithExpected(&ast.NumberExpr{Value: "42"}, 1, typ.Integer)
	if !typ.TypeEquals(observed, typ.Integer) {
		t.Fatalf("TypeOfWithExpected(integer literal) = %v, want integer", observed)
	}
}

func TestProjector_DynamicIndexIdentifierDoesNotBecomeFieldName(t *testing.T) {
	objExpr := &ast.IdentExpr{Value: "obj"}
	keyExpr := &ast.IdentExpr{Value: "name"}
	indexExpr := &ast.AttrGetExpr{Object: objExpr, Key: keyExpr}
	bindings := bind.NewBindingTable()
	const objSym cfg.SymbolID = 20
	const keySym cfg.SymbolID = 21
	bindings.Bind(objExpr, objSym)
	bindings.Bind(keyExpr, keySym)

	objType := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	observed := New(Config{
		Bindings: bindings,
		Facts: factsStub{
			objSym: objType,
			keySym: typ.LiteralString("suite"),
		},
	}).TypeOf(indexExpr, 1)
	want := typ.NewOptional(typ.Number)

	if !typ.TypeEquals(observed, want) {
		t.Fatalf("TypeOf(obj[name]) = %v, want %v", observed, want)
	}
}

func TestProjector_CastExprUsesConfiguredTypeResolver(t *testing.T) {
	cast := &ast.CastExpr{
		Expr: &ast.TableExpr{Fields: []*ast.Field{
			{Value: &ast.StringExpr{Value: "admin"}},
		}},
		Type: &ast.ArrayTypeExpr{Element: &ast.PrimitiveTypeExpr{Name: "string"}},
	}
	expected := typ.NewArray(typ.String)

	observed := New(Config{
		ResolveType: func(expr ast.TypeExpr, _ *scope.State) typ.Type {
			if _, ok := expr.(*ast.ArrayTypeExpr); ok {
				return expected
			}
			return nil
		},
	}).TypeOf(cast, 1)

	if !typ.TypeEquals(observed, expected) {
		t.Fatalf("TypeOf(cast) = %v, want %v", observed, expected)
	}
}

func TestProjector_OperatorsUseCanonicalQueryAlgebra(t *testing.T) {
	p := New(Config{})
	intAdd := p.TypeOf(&ast.ArithmeticOpExpr{
		Operator: "+",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}, 1)
	if !typ.TypeEquals(intAdd, typ.Integer) {
		t.Fatalf("integer addition = %v, want integer", intAdd)
	}

	neg := p.TypeOf(&ast.UnaryMinusOpExpr{Expr: &ast.NumberExpr{Value: "1"}}, 1)
	if !typ.TypeEquals(neg, typ.Integer) {
		t.Fatalf("unary minus integer = %v, want integer", neg)
	}

	bnot := p.TypeOf(&ast.UnaryBNotOpExpr{Expr: &ast.NumberExpr{Value: "1"}}, 1)
	if !typ.TypeEquals(bnot, typ.Integer) {
		t.Fatalf("bitwise not integer = %v, want integer", bnot)
	}
}

func TestProjector_TableWithExpectedMapKeepsMapProduct(t *testing.T) {
	expected := typ.NewMap(typ.String, typ.Any)
	observed := New(Config{}).TypeOfWithExpected(&ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.IdentExpr{Value: "query"}, Value: &ast.StringExpr{Value: "term"}},
	}}, 1, expected)

	if !typ.TypeEquals(observed, expected) {
		t.Fatalf("TypeOfWithExpected(map literal) = %v, want %v", observed, expected)
	}
}

func TestProjector_ArrayElementUsesDiscriminatedUnionContext(t *testing.T) {
	content := typ.NewRecord().
		Field("type", typ.LiteralString("content")).
		Field("data", typ.String).
		Build()
	toolCall := typ.NewRecord().
		Field("type", typ.LiteralString("tool_call")).
		Field("id", typ.String).
		Field("name", typ.String).
		Field("arguments", typ.NewMap(typ.String, typ.Any)).
		Build()
	expected := typ.NewArray(typ.NewUnion(content, toolCall))

	observed := New(Config{}).TypeOfWithExpected(&ast.TableExpr{Fields: []*ast.Field{
		{Value: &ast.TableExpr{Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "type"}, Value: &ast.StringExpr{Value: "content"}},
			{Key: &ast.IdentExpr{Value: "data"}, Value: &ast.StringExpr{Value: "hello"}},
		}}},
		{Value: &ast.TableExpr{Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "type"}, Value: &ast.StringExpr{Value: "tool_call"}},
			{Key: &ast.IdentExpr{Value: "id"}, Value: &ast.StringExpr{Value: "t1"}},
			{Key: &ast.IdentExpr{Value: "name"}, Value: &ast.StringExpr{Value: "search"}},
			{Key: &ast.IdentExpr{Value: "arguments"}, Value: &ast.TableExpr{Fields: []*ast.Field{
				{Key: &ast.IdentExpr{Value: "query"}, Value: &ast.StringExpr{Value: "term"}},
			}}},
		}}},
	}}, 1, expected)

	if !typ.TypeEquals(observed, expected) {
		t.Fatalf("TypeOfWithExpected(union array literal) = %v, want %v", observed, expected)
	}
}

func TestProjector_FunctionLiteralUsesActualBeforeExpected(t *testing.T) {
	fn := &ast.FunctionExpr{}
	actual := typ.Func().Param("n", typ.Number).Returns(typ.Number).Build()
	expected := typ.Func().Param("s", typ.String).Returns(typ.String).Build()

	observed := New(Config{
		LiteralSignatures: map[*ast.FunctionExpr]*typ.Function{fn: actual},
	}).TypeOfWithExpected(fn, 1, expected)

	if !typ.TypeEquals(observed, actual) {
		t.Fatalf("function literal observation = %v, want actual %v", observed, actual)
	}
}

func TestProjector_CallReturnsFunctionSignatureWithoutCallInference(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 11
	bindings.Bind(callee, sym)
	call := &ast.FuncCallExpr{Func: callee}

	observed := New(Config{
		Bindings: bindings,
		FunctionType: func(candidate cfg.SymbolID) typ.Type {
			if candidate == sym {
				return typ.Func().Returns(typ.String, typ.Integer).Build()
			}
			return nil
		},
	}).MultiTypeOf(call, 1)

	if len(observed) != 2 || !typ.TypeEquals(observed[0], typ.String) || !typ.TypeEquals(observed[1], typ.Integer) {
		t.Fatalf("MultiTypeOf(call) = %v, want string, integer", observed)
	}
}

func TestProjector_MethodReturnEffectsUseRuntimeReceiverSlot(t *testing.T) {
	receiverExpr := &ast.IdentExpr{Value: "context_query"}
	call := &ast.FuncCallExpr{
		Receiver: receiverExpr,
		Method:   "type",
		Args:     []ast.Expr{&ast.StringExpr{Value: "conversation_summary"}},
	}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 14
	bindings.Bind(receiverExpr, sym)

	method := typ.Func().
		Param("self", typ.Unknown).
		Param("kind", typ.String).
		Returns(typ.Unknown).
		Effects(effect.Row{Labels: []effect.Label{
			effect.FlowInto{ParamIndex: 0, ReturnIndex: 0},
		}}).
		Build()
	receiver := typ.NewRecord().
		Field("id", typ.String).
		Field("type", method).
		Build()

	observed := New(Config{
		Bindings: bindings,
		Facts:    factsStub{sym: receiver},
		Ctx:      db.NewQueryContext(db.New()),
		TypeOps:  querycore.NewEngine(),
	}).MultiTypeOf(call, 1)

	if len(observed) != 1 {
		t.Fatalf("MultiTypeOf(method call) = %v, want one return", observed)
	}
	rec, ok := observed[0].(*typ.Record)
	if !ok {
		t.Fatalf("method return = %T %v, want receiver record", observed[0], observed[0])
	}
	if field := rec.GetField("id"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("receiver id field = %#v, want string", field)
	}
}

func TestProjector_SetMetatableCallReturnsMetatabledRecord(t *testing.T) {
	closeFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "setmetatable"},
		Args: []ast.Expr{
			&ast.TableExpr{Fields: []*ast.Field{
				{Key: &ast.IdentExpr{Value: "id"}, Value: &ast.StringExpr{Value: "r1"}},
			}},
			&ast.TableExpr{Fields: []*ast.Field{
				{Key: &ast.IdentExpr{Value: "__index"}, Value: &ast.TableExpr{Fields: []*ast.Field{
					{Key: &ast.IdentExpr{Value: "close"}, Value: closeFn},
				}}},
			}},
		},
	}

	observed := New(Config{}).MultiTypeOf(call, 1)
	if len(observed) != 1 {
		t.Fatalf("MultiTypeOf(setmetatable) = %v, want one return", observed)
	}
	if _, ok := querycore.Method(observed[0], "close"); !ok {
		t.Fatalf("expected metatable method on observed return, got %s", typ.FormatShort(observed[0]))
	}
}

func TestProjector_EmptyTableUsesFreshBottom(t *testing.T) {
	observed := New(Config{}).TypeOf(&ast.TableExpr{}, 1)
	rec, ok := observed.(*typ.Record)
	if !ok {
		t.Fatalf("TypeOf(empty table) = %T %[1]v, want record", observed)
	}
	if rec.Open || len(rec.Fields) != 0 || rec.HasMapComponent() || rec.Metatable != nil {
		t.Fatalf("empty table observation should be closed fresh bottom, got %v", rec)
	}
}

func TestProjector_MissingRecordFieldReadsNilForLogicalDefault(t *testing.T) {
	const entrySym cfg.SymbolID = 42
	bindings := bind.NewBindingTable()
	entryForData := &ast.IdentExpr{Value: "entry"}
	entryForMax := &ast.IdentExpr{Value: "entry"}
	bindings.Bind(entryForData, entrySym)
	bindings.Bind(entryForMax, entrySym)

	dataForCondition := &ast.AttrGetExpr{
		Object: entryForData,
		Key:    &ast.StringExpr{Value: "data"},
	}
	dataForValue := &ast.AttrGetExpr{
		Object: entryForMax,
		Key:    &ast.StringExpr{Value: "data"},
	}
	value := &ast.AttrGetExpr{
		Object: dataForValue,
		Key:    &ast.StringExpr{Value: "max_tokens"},
	}
	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs: &ast.LogicalOpExpr{
			Operator: "and",
			Lhs:      dataForCondition,
			Rhs:      value,
		},
		Rhs: &ast.NumberExpr{Value: "0"},
	}

	entryType := typ.NewRecord().
		Field("data", typ.NewRecord().Build()).
		SetOpen(true).
		Build()
	got := New(Config{
		Bindings: bindings,
		Facts:    factsStub{entrySym: entryType},
	}).TypeOf(expr, 1)

	if !typ.TypeEquals(got, typ.LiteralInt(0)) {
		t.Fatalf("TypeOf(logical default) = %v, want 0", got)
	}
}

func TestFromFuncResultUsesModuleBindingsAndFunctionProjection(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 12
	bindings.Bind(callee, sym)
	call := &ast.FuncCallExpr{Func: callee}

	observed := FromFuncResult(&api.FuncResult{
		ModuleBindings: bindings,
	}, func(candidate cfg.SymbolID) typ.Type {
		if candidate == sym {
			return typ.Func().Returns(typ.Boolean).Build()
		}
		return nil
	}).MultiTypeOf(call, 1)

	if len(observed) != 1 || !typ.TypeEquals(observed[0], typ.Boolean) {
		t.Fatalf("MultiTypeOf(call) = %v, want boolean", observed)
	}
}

func TestProjector_FunctionLiteralUsesCanonicalProjection(t *testing.T) {
	fn := &ast.FunctionExpr{}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 13
	bindings.SetFuncLitSymbol(fn, sym)
	staleLiteral := typ.Func().Build()
	want := typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()

	observed := New(Config{
		Bindings:          bindings,
		LiteralSignatures: map[*ast.FunctionExpr]*typ.Function{fn: staleLiteral},
		FunctionType: func(candidate cfg.SymbolID) typ.Type {
			if candidate == sym {
				return want
			}
			return nil
		},
	}).TypeOf(fn, 1)

	if !typ.TypeEquals(observed, want) {
		t.Fatalf("TypeOf(function literal) = %v, want canonical projection %v", observed, want)
	}
}

func TestProjector_TableUsesDiscriminatedExpectedMember(t *testing.T) {
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.IdentExpr{Value: "kind"}, Value: &ast.StringExpr{Value: "ok"}},
		{Key: &ast.IdentExpr{Value: "value"}, Value: &ast.StringExpr{Value: "payload"}},
	}}
	expected := typ.NewUnion(
		typ.NewRecord().
			Field("kind", typ.LiteralString("ok")).
			Field("value", typ.String).
			Build(),
		typ.NewRecord().
			Field("kind", typ.LiteralString("err")).
			Field("message", typ.String).
			Build(),
	)

	observed := New(Config{}).TypeOfWithExpected(table, 1, expected)
	rec, ok := observed.(*typ.Record)
	if !ok {
		t.Fatalf("observed table = %T %v, want record", observed, observed)
	}
	if field := rec.GetField("kind"); field == nil || !typ.TypeEquals(field.Type, typ.LiteralString("ok")) {
		t.Fatalf("kind field = %#v, want literal ok", field)
	}
	if field := rec.GetField("value"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("value field = %#v, want string", field)
	}
}
