package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenType_Nil(t *testing.T) {
	result := WidenType(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestWidenType_BooleanLiteral(t *testing.T) {
	lit := typ.LiteralBool(true)
	result := WidenType(lit)
	if result != typ.Boolean {
		t.Errorf("expected Boolean, got %v", result)
	}
}

func TestWidenType_IntegerLiteral(t *testing.T) {
	lit := typ.LiteralInt(42)
	result := WidenType(lit)
	if result != typ.Integer {
		t.Errorf("expected Integer, got %v", result)
	}
}

func TestWidenType_NumberLiteral(t *testing.T) {
	lit := typ.LiteralNumber(3.14)
	result := WidenType(lit)
	if result != typ.Number {
		t.Errorf("expected Number, got %v", result)
	}
}

func TestWidenType_StringLiteral(t *testing.T) {
	lit := typ.LiteralString("hello")
	result := WidenType(lit)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestWidenType_NonLiteral(t *testing.T) {
	result := WidenType(typ.String)
	if result != typ.String {
		t.Errorf("expected String unchanged, got %v", result)
	}
}

func TestWidenType_Alias(t *testing.T) {
	alias := typ.NewAlias("NumAlias", typ.Number)
	result := WidenType(alias)
	if result != typ.Number {
		t.Errorf("expected alias to widen to Number, got %v", result)
	}
}

func TestWidenType_Optional(t *testing.T) {
	lit := typ.LiteralString("hello")
	opt := typ.NewOptional(lit)
	result := WidenType(opt)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	optResult, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if optResult.Inner != typ.String {
		t.Errorf("expected inner to be String, got %v", optResult.Inner)
	}
}

func TestWidenType_Union(t *testing.T) {
	lit1 := typ.LiteralString("a")
	lit2 := typ.LiteralNumber(1.0)
	union := typ.NewUnion(lit1, lit2)
	result := WidenType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestNormalizeType_TableTopAbsorbsPreciseTableMembers(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	preciseA := typ.NewRecord().
		Field("name", typ.String).
		Field("tools", typ.NewArray(typ.String)).
		Build()
	preciseB := typ.NewMap(typ.String, typ.Integer)
	evidence := typ.NewUnion(typ.NewOptional(tableTop), preciseA, preciseB, typ.String)

	got := NormalizeType(evidence)
	want := typ.NewUnion(typ.NewOptional(tableTop), typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected table top to absorb precise table members as %v, got %v", want, got)
	}
}

func TestWidenType_RecordPreservesClosedShape(t *testing.T) {
	rec := typ.NewRecord().
		Field("pid", typ.LiteralString("abc")).
		Field("topic", typ.LiteralString("test:update")).
		Build()

	result := WidenType(rec)
	widened, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", result)
	}
	if widened.Open {
		t.Fatalf("expected parameter evidence to preserve closed call-site shape, got open: %v", widened)
	}

	pid := widened.GetField("pid")
	if pid == nil || !typ.TypeEquals(pid.Type, typ.String) {
		t.Fatalf("expected pid field widened to string, got %v", pid)
	}
	topic := widened.GetField("topic")
	if topic == nil || !typ.TypeEquals(topic.Type, typ.String) {
		t.Fatalf("expected topic field widened to string, got %v", topic)
	}
}

func TestBuildSignatureMap_NilInputs(t *testing.T) {
	result := BuildSignatureMap(nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil inputs, got %v", result)
	}
}

func TestMergeIntoSignature_ImplicitSelfUsesEffectiveHintSlots(t *testing.T) {
	fn := functionWithParams("name")
	sig := typ.Func().
		Param("self", typ.Unknown).
		Param("name", typ.Unknown).
		Build()
	selfType := typ.NewRecord().Field("prefix", typ.String).Build()

	got := MergeIntoSignature(fn, []typ.Type{selfType, typ.String}, sig)
	if got == nil || len(got.Params) != 2 {
		t.Fatalf("unexpected merged signature: %v", got)
	}
	if !typ.TypeEquals(got.Params[0].Type, selfType) {
		t.Fatalf("self evidence should use effective slot 0, got %v", got.Params[0].Type)
	}
	if !typ.TypeEquals(got.Params[1].Type, typ.String) {
		t.Fatalf("source parameter evidence should use effective slot 1, got %v", got.Params[1].Type)
	}
}

func TestMergeIntoSignature_SoftArrayAnnotationPreservesContainerShape(t *testing.T) {
	fn := functionWithParams("responses")
	fn.ParList.Types = []ast.TypeExpr{&ast.ArrayTypeExpr{
		Element: &ast.PrimitiveTypeExpr{Name: "any"},
	}}
	sig := typ.Func().
		Param("responses", typ.NewArray(typ.Any)).
		Build()
	evidence := typ.NewTuple(
		typ.NewRecord().Field("ok", typ.Boolean).Build(),
		typ.NewRecord().Field("ok", typ.Boolean).Build(),
	)

	got := MergeIntoSignature(fn, []typ.Type{evidence}, sig)
	if got == nil || len(got.Params) != 1 {
		t.Fatalf("unexpected merged signature: %v", got)
	}
	want := typ.NewArray(typ.NewRecord().Field("ok", typ.Boolean).Build())
	if !typ.TypeEquals(got.Params[0].Type, want) {
		t.Fatalf("soft {any} parameter annotation must refine element domain without becoming a tuple, got %v", got.Params[0].Type)
	}
}

func TestMergeIntoSignature_HardAnyAnnotationRemainsAuthoritative(t *testing.T) {
	fn := functionWithParams("value")
	fn.ParList.Types = []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "any"}}
	sig := typ.Func().
		Param("value", typ.Any).
		Build()
	evidence := typ.NewRecord().Field("id", typ.String).Build()

	got := MergeIntoSignature(fn, []typ.Type{evidence}, sig)
	if got != sig {
		t.Fatalf("hard any annotation must stay authoritative, got %v", got)
	}
}

func TestMergeIntoSignature_PreservesExplicitNilabilityOnOptionalSlot(t *testing.T) {
	fn := functionWithParams("context")
	context := typ.NewRecord().
		MapComponent(typ.String, typ.Any).
		SetOpen(true).
		Build()
	sig := typ.Func().OptParam("context", typ.Any).Build()

	got := MergeIntoSignature(fn, []typ.Type{typ.NewOptional(context)}, sig)
	if got == nil || len(got.Params) != 1 {
		t.Fatalf("unexpected merged signature: %v", got)
	}
	if !got.Params[0].Optional {
		t.Fatalf("expected parameter slot to remain optional: %v", got)
	}
	want := typ.NewOptional(context)
	if !typ.TypeEquals(got.Params[0].Type, want) {
		t.Fatalf("expected nilability to remain in the value type, got %v", got.Params[0].Type)
	}
}

func TestProjectToParameterUse_KeepsDemandedRecordFields(t *testing.T) {
	fn := functionWithParams("client", "model_id")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "client"},
					Key:    &ast.StringExpr{Value: "invoke"},
				},
				Args: []ast.Expr{
					&ast.IdentExpr{Value: "model_id"},
					&ast.TableExpr{},
					&ast.TableExpr{},
				},
			},
		}},
	}
	graph := cfg.Build(fn)
	invoke := typ.Func().Param("model_id", typ.String).Returns(typ.Unknown).Build()
	client := typ.NewRecord().
		Field("invoke", invoke).
		Field("process_converse_stream", typ.Func().Returns(typ.String).Build()).
		Field("_credentials", typ.String).
		Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{client, typ.String})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected client evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("invoke") == nil {
		t.Fatalf("projected client evidence lost demanded invoke field: %v", rec)
	}
	for _, unused := range []string{"process_converse_stream", "_credentials"} {
		if rec.GetField(unused) != nil {
			t.Fatalf("projected client evidence kept unused field %q: %v", unused, rec)
		}
	}
	if !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("directly used scalar evidence should stay intact, got %v", got[1])
	}
}

func TestProjectToParameterUse_KeepsDemandedAbsentRecordFieldsAsNil(t *testing.T) {
	fn := functionWithParams("options")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "stream"},
			},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{
				&ast.AttrGetExpr{
					Object: &ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "options"},
						Key:    &ast.StringExpr{Value: "headers"},
					},
					Key: &ast.StringExpr{Value: "Accept"},
				},
			},
			Rhs: []ast.Expr{&ast.StringExpr{Value: "application/json"}},
		},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().
		Field("headers", typ.NewRecord().Build()).
		Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected options evidence = %T, want record (%v)", got[0], got[0])
	}
	stream := rec.GetField("stream")
	if stream == nil || !typ.TypeEquals(stream.Type, typ.Nil) {
		t.Fatalf("demanded absent stream field should project as nil, got %v in %v", stream, rec)
	}
	headers := rec.GetField("headers")
	if headers == nil {
		t.Fatalf("projected options evidence lost demanded headers field: %v", rec)
	}
}

func TestProjectSignatureToParamUse_CompletesDemandedAbsentFields(t *testing.T) {
	fn := functionWithParams("info")
	fn.Stmts = []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "info"}},
			Rhs: []ast.Expr{&ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.IdentExpr{Value: "info"},
				Rhs:      &ast.TableExpr{},
			}},
		},
		&ast.LocalAssignStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "info"},
				Key:    &ast.StringExpr{Value: "message"},
			},
		}},
		&ast.LocalAssignStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "info"},
				Key:    &ast.StringExpr{Value: "status_code"},
			},
		}},
	}
	graph := cfg.Build(fn)
	info := typ.NewRecord().
		OptField("message", typ.String).
		Build()
	sig := typ.Func().
		Param("info", info).
		Returns(typ.String).
		Build()

	got := ProjectSignatureToParamUse(graph, fn, sig)
	rec, ok := got.Params[0].Type.(*typ.Record)
	if !ok {
		t.Fatalf("projected param = %T, want record (%v)", got.Params[0].Type, got.Params[0].Type)
	}
	if rec.GetField("message") == nil {
		t.Fatalf("projected signature lost existing demanded message field: %v", rec)
	}
	status := rec.GetField("status_code")
	if status == nil || !typ.TypeEquals(status.Type, typ.Nil) {
		t.Fatalf("projected signature should include demanded absent status_code as nil, got %v in %v", status, rec)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.String) {
		t.Fatalf("projected signature lost returns: %v", got)
	}
}

func TestProjectToParameterUse_TypeGuardDoesNotKeepWholeRecord(t *testing.T) {
	fn := functionWithParams("params")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.RelationalOpExpr{
				Operator: "~=",
				Lhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "params"}},
				},
				Rhs: &ast.StringExpr{Value: "table"},
			},
		},
		&ast.IfStmt{
			Condition: &ast.UnaryNotOpExpr{
				Expr: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "params"},
					Key:    &ast.StringExpr{Value: "agent"},
				},
			},
		},
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "params"},
				Key:    &ast.StringExpr{Value: "kind"},
			},
		}},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().
		OptField("kind", typ.String).
		Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected params evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("kind") == nil {
		t.Fatalf("projected params evidence lost demanded kind field: %v", rec)
	}
	agent := rec.GetField("agent")
	if agent == nil || !typ.TypeEquals(agent.Type, typ.Nil) {
		t.Fatalf("type(params) should not force whole-record evidence; agent = %v in %v", agent, rec)
	}
}

func TestProjectToParameterUse_SelfDefaultDoesNotKeepWholeRecord(t *testing.T) {
	fn := functionWithParams("options")
	optionsIdent := &ast.IdentExpr{Value: "options"}
	fn.Stmts = []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{optionsIdent},
			Rhs: []ast.Expr{&ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.IdentExpr{Value: "options"},
				Rhs:      &ast.TableExpr{},
			}},
		},
		&ast.LocalAssignStmt{
			Names: []string{"method"},
			Exprs: []ast.Expr{&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "method"},
			}},
		},
		&ast.LocalAssignStmt{
			Names: []string{"timeout"},
			Exprs: []ast.Expr{&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "timeout"},
			}},
		},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().
		OptField("method", typ.String).
		Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected options evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("method") == nil {
		t.Fatalf("projected options evidence lost demanded method field: %v", rec)
	}
	timeout := rec.GetField("timeout")
	if timeout == nil || !typ.TypeEquals(timeout.Type, typ.Nil) {
		t.Fatalf("self-default assignment should not force whole-record evidence; timeout = %v in %v", timeout, rec)
	}
}

func TestProjectToParameterUse_DedupsUnionAfterProjection(t *testing.T) {
	fn := functionWithParams("client")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "client"},
					Key:    &ast.StringExpr{Value: "invoke"},
				},
			},
		}},
	}
	graph := cfg.Build(fn)
	invoke := typ.Func().Returns(typ.Unknown).Build()
	broad := typ.NewRecord().
		Field("invoke", invoke).
		Field("stream", typ.Func().Returns(typ.String).Build()).
		Build()
	narrow := typ.NewRecord().
		Field("invoke", invoke).
		Field("stream", typ.Func().Returns(typ.LiteralString("invalid")).Build()).
		Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{typ.NewUnion(broad, narrow)})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected union evidence = %T, want coalesced record (%v)", got[0], got[0])
	}
	if rec.GetField("invoke") == nil || rec.GetField("stream") != nil {
		t.Fatalf("projected union should keep only invoke, got %v", rec)
	}
}

func TestProjectToParameterUse_WholeParameterUseKeepsEvidence(t *testing.T) {
	fn := functionWithParams("client")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "use_client"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "client"}},
			},
		}},
	}
	graph := cfg.Build(fn, "use_client")
	client := typ.NewRecord().Field("invoke", typ.Func().Returns(typ.Unknown).Build()).Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{client})
	if !typ.TypeEquals(got[0], client) {
		t.Fatalf("whole-parameter use should keep full evidence, got %v", got[0])
	}
}

func TestProjectToParameterUse_RecursiveForwardingDoesNotKeepWholeEvidence(t *testing.T) {
	recursiveIdent := &ast.IdentExpr{Value: "visit"}
	selfIdent := &ast.IdentExpr{Value: "self"}
	valueIdent := &ast.IdentExpr{Value: "value"}
	fn := functionWithParams("self", "value")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.AttrGetExpr{
				Object: valueIdent,
				Key:    &ast.StringExpr{Value: "next"},
			},
			Then: []ast.Stmt{
				&ast.ReturnStmt{Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: recursiveIdent,
						Args: []ast.Expr{
							selfIdent,
							&ast.AttrGetExpr{
								Object: &ast.IdentExpr{Value: "value"},
								Key:    &ast.StringExpr{Value: "next"},
							},
						},
					},
				}},
			},
		},
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "self"},
				Key:    &ast.StringExpr{Value: "id"},
			},
		}},
	}
	graph := cfg.Build(fn, "visit")
	if sym, ok := graph.Bindings().SymbolOf(recursiveIdent); ok {
		graph.Bindings().SetFuncLitSymbol(fn, sym)
	}
	selfEvidence := typ.NewRecord().
		Field("id", typ.String).
		Field("command", typ.Func().Returns(typ.Nil, typ.String).Build()).
		Build()

	got := ProjectToParameterUse(graph, fn, []typ.Type{selfEvidence, typ.NewRecord().Field("next", typ.Any).Build()})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected self evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("id") == nil {
		t.Fatalf("projected self evidence lost demanded id field: %v", rec)
	}
	if rec.GetField("command") != nil {
		t.Fatalf("recursive forwarding should not keep unused command field: %v", rec)
	}
}

func functionWithParams(names ...string) *ast.FunctionExpr {
	return &ast.FunctionExpr{ParList: &ast.ParList{Names: names}}
}

func TestIsInformative(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "any", in: typ.Any, want: false},
		{name: "unknown", in: typ.Unknown, want: false},
		{name: "never", in: typ.Never, want: false},
		{name: "nil type", in: typ.Nil, want: false},
		{name: "empty record", in: typ.NewRecord().Build(), want: false},
		{name: "map with string key", in: typ.NewMap(typ.String, typ.NewArray(typ.Any)), want: true},
		{name: "record map component", in: typ.NewRecord().MapComponent(typ.String, typ.Any).Build(), want: true},
		{name: "string", in: typ.String, want: true},
		{name: "literal", in: typ.LiteralString("x"), want: true},
		{name: "type param", in: typ.NewTypeParam("T", nil), want: false},
		{name: "ref", in: typ.NewRef("", "Foo"), want: false},
		{name: "optional unknown", in: typ.NewOptional(typ.Unknown), want: false},
		{name: "optional string", in: typ.NewOptional(typ.String), want: true},
		{name: "union placeholders", in: typ.NewUnion(typ.Unknown, typ.Nil), want: false},
		{name: "union with informative member", in: typ.NewUnion(typ.Unknown, typ.String), want: true},
	}

	for _, tt := range tests {
		if got := IsInformative(tt.in); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestEnsureCapacity(t *testing.T) {
	base := []typ.Type{typ.String}
	got := EnsureCapacity(base, 3)
	if len(got) != 3 {
		t.Fatalf("EnsureCapacity len = %d, want 3", len(got))
	}
	if got[0] != typ.String {
		t.Fatalf("EnsureCapacity preserved value = %v, want string", got[0])
	}
}

func TestMergeAt(t *testing.T) {
	join := func(prev, next typ.Type) typ.Type { return typ.JoinPreferNonSoft(prev, next) }

	t.Run("filters non-informative", func(t *testing.T) {
		evidence := []typ.Type{typ.String}
		got, changed := MergeAt(evidence, 1, typ.Unknown, join)
		if changed {
			t.Fatal("expected no change for unknown evidence")
		}
		if len(got) != 1 {
			t.Fatalf("expected unchanged slice len 1, got %d", len(got))
		}
	})

	t.Run("normalizes literal and merges", func(t *testing.T) {
		got, changed := MergeAt(nil, 0, typ.LiteralString("x"), join)
		if !changed {
			t.Fatal("expected merge change for informative literal")
		}
		if len(got) != 1 {
			t.Fatalf("expected one evidence, got %d", len(got))
		}
		if !typ.TypeEquals(got[0], typ.String) {
			t.Fatalf("expected normalized string evidence, got %v", got[0])
		}
	})
}

func TestMergeCallArgAt_PreservesExplicitNilArgument(t *testing.T) {
	got, changed := MergeCallArgAt(nil, 0, typ.Nil, typ.JoinPreferNonSoft, true)
	if !changed {
		t.Fatal("expected nil argument to be recorded")
	}
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Nil) {
		t.Fatalf("expected nil evidence, got %v", got)
	}

	rec := typ.NewRecord().Field("id", typ.String).Build()
	got, changed = MergeCallArgAt(got, 0, rec, typ.JoinPreferNonSoft, true)
	if !changed {
		t.Fatal("expected record call to merge with nil evidence")
	}
	if !typ.TypeEquals(got[0], typ.NewOptional(rec)) {
		t.Fatalf("expected nil plus record calls to produce optional record evidence, got %v", got[0])
	}
}
