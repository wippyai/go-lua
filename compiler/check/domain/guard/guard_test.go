package guard_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTruthyPathKey_Equality(t *testing.T) {
	key1 := truthyKey(t, 1, constraint.Segment{Kind: constraint.SegmentField, Name: "foo"})
	key2 := truthyKey(t, 1, constraint.Segment{Kind: constraint.SegmentField, Name: "foo"})
	key3 := truthyKey(t, 2, constraint.Segment{Kind: constraint.SegmentField, Name: "foo"})
	key4 := truthyKey(t, 1, constraint.Segment{Kind: constraint.SegmentField, Name: "bar"})

	if key1 != key2 {
		t.Error("expected identical keys to be equal")
	}
	if key1 == key3 {
		t.Error("expected different symbols to produce different keys")
	}
	if key1 == key4 {
		t.Error("expected different fields to produce different keys")
	}
}

func TestTruthyPathKey_DistinguishesFieldAndStringIndex(t *testing.T) {
	fieldKey := truthyKey(t, 1, constraint.Segment{Kind: constraint.SegmentField, Name: "x-y"})
	indexKey := truthyKey(t, 1, constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"})

	if fieldKey == indexKey {
		t.Fatal("field and static string-index paths must not share a guard key")
	}
}

func TestCollectTruthyGuards_NilGraph(t *testing.T) {
	result := guard.CollectTruthyGuards(nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestCollectTruthyGuards_NilBindings(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NilExpr{}}},
		},
	}
	graph := cfg.Build(fn)
	result := guard.CollectTruthyGuards(graph, nil, nil)
	if result != nil {
		t.Error("expected nil for nil bindings")
	}
}

func TestExtractTruthyPathKeys_NilExpr(t *testing.T) {
	result := guard.ExtractTruthyPathKeys(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}

func TestExtractTruthyPathKeys_NilBindings(t *testing.T) {
	result := guard.ExtractTruthyPathKeys(&ast.IdentExpr{Value: "x"}, nil)
	if result != nil {
		t.Error("expected nil for nil bindings")
	}
}

func TestExtractTruthyPathKeys_IdentExpr(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "x"},
				Then:      []ast.Stmt{&ast.ReturnStmt{}},
			},
		},
	}
	bindings := bind.Bind(fn, nil)

	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	condIdent := ifStmt.Condition.(*ast.IdentExpr)

	result := guard.ExtractTruthyPathKeys(condIdent, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	if result[0].Path.Display() != "" {
		t.Errorf("expected empty field for simple ident, got %q", result[0].Path.Display())
	}
}

func TestExtractTruthyPathKeys_NestedAttrExpr(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "event"},
			Key:    &ast.StringExpr{Value: "payload"},
		},
		Key: &ast.StringExpr{Value: "from"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: expr, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)

	result := guard.ExtractTruthyPathKeys(expr, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	base := expr.Object.(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	sym, _ := bindings.SymbolOf(base)
	if result[0].Symbol != sym {
		t.Fatalf("expected symbol %d, got %d", sym, result[0].Symbol)
	}
	if result[0].Path.Display() != "payload.from" {
		t.Fatalf("expected field payload.from, got %q", result[0].Path.Display())
	}
}

func TestExtractTruthyPathKeys_StaticIndexIntExpr(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.NumberExpr{Value: "1"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: expr, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	result := guard.ExtractTruthyPathKeys(expr, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key for static index-int path, got %d", len(result))
	}
	if result[0].Path.Display() != "[1]" {
		t.Fatalf("expected [1], got %q", result[0].Path.Display())
	}
}

func TestExtractTruthyPathKeys_StaticIndexStringExpr(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.StringExpr{Value: "x-y"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: expr, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)

	result := guard.ExtractTruthyPathKeys(expr, bindings)
	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result))
	}
	if result[0].Path.Display() != "[\"x-y\"]" {
		t.Fatalf("expected [\"x-y\"], got %q", result[0].Path.Display())
	}
}

func TestTruthyKeyFromExpr_DynamicIdentKeyRejected(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.IdentExpr{Value: "k"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event", "k"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)

	if _, ok := guard.TruthyKeyFromExpr(expr, bindings); ok {
		t.Fatal("expected dynamic ident key to be rejected")
	}
}

func TestNarrowTableFieldsByGuard_NonRecord(t *testing.T) {
	result := guard.NarrowTableFieldsByGuard(typ.String, nil, 0, nil, nil, nil)
	if result != typ.String {
		t.Error("expected string type returned unchanged")
	}
}

func TestNarrowTableFieldsByGuard_NilRecord(t *testing.T) {
	result := guard.NarrowTableFieldsByGuard(nil, nil, 0, nil, nil, nil)
	if result != nil {
		t.Error("expected nil returned")
	}
}

func TestNarrowTableFieldsByGuard_EmptyGuards(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.String).Build()
	result := guard.NarrowTableFieldsByGuard(rec, &ast.TableExpr{}, 1, nil, nil, nil)
	if result != rec {
		t.Error("expected original record when no guards")
	}
}

func TestNarrowTableFieldsByGuard_NoMatchingGuards(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.NewOptional(typ.String)).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x"}, Value: &ast.StringExpr{Value: "val"}},
		},
	}
	guards := map[cfg.Point]map[guard.TruthyPathKey]bool{
		1: {},
	}
	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, nil, guards, nil)
	if result != rec {
		t.Error("expected original record when no matching guards")
	}
}

func TestNarrowTableFieldsByGuard_MatchingNestedPath(t *testing.T) {
	valueExpr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "event"},
			Key:    &ast.StringExpr{Value: "payload"},
		},
		Key: &ast.StringExpr{Value: "from"},
	}
	rec := typ.NewRecord().Field("from", typ.NewOptional(typ.String)).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "from"},
				Value: valueExpr,
			},
		},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{tbl},
			},
		},
	}
	bindings := bind.Bind(fn, nil)
	eventBase := valueExpr.Object.(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	eventSym, _ := bindings.SymbolOf(eventBase)

	guards := map[cfg.Point]map[guard.TruthyPathKey]bool{
		1: {
			truthyKey(t, eventSym,
				constraint.Segment{Kind: constraint.SegmentField, Name: "payload"},
				constraint.Segment{Kind: constraint.SegmentField, Name: "from"},
			): true,
		},
	}

	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, bindings, guards, nil)
	out, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", result)
	}
	if len(out.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(out.Fields))
	}
	if !typ.TypeEquals(out.Fields[0].Type, typ.String) {
		t.Fatalf("expected narrowed string field, got %s", out.Fields[0].Type.String())
	}
	if out.Fields[0].Optional {
		t.Fatalf("truthy source guard should make table literal field required")
	}
}

func TestNarrowTableFieldsByGuard_FieldOptionalityIsARecordShapeChange(t *testing.T) {
	valueExpr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "event"},
		Key:    &ast.StringExpr{Value: "from"},
	}
	rec := typ.NewRecord().OptField("from", typ.String).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "from"},
				Value: valueExpr,
			},
		},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{tbl},
			},
		},
	}
	bindings := bind.Bind(fn, nil)
	eventSym, _ := bindings.SymbolOf(valueExpr.Object.(*ast.IdentExpr))
	guards := map[cfg.Point]map[guard.TruthyPathKey]bool{
		1: {
			truthyKey(t, eventSym, constraint.Segment{Kind: constraint.SegmentField, Name: "from"}): true,
		},
	}

	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, bindings, guards, nil)
	out, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", result)
	}
	field := out.GetField("from")
	if field == nil {
		t.Fatal("missing from field")
	}
	if field.Optional {
		t.Fatalf("truthy source guard should make field required, got %s", out.String())
	}
	if !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("expected string field, got %s", field.Type.String())
	}
}

func TestCollectTypeGuards_TypeNotEqReturnPropagatesFallthrough(t *testing.T) {
	condExpr := &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{
				&ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "payload"},
					Key:    &ast.StringExpr{Value: "respond_to"},
				},
			},
		},
		Rhs: &ast.StringExpr{Value: "string"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"payload"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: condExpr,
				Then:      []ast.Stmt{&ast.ReturnStmt{}},
			},
			&ast.LocalAssignStmt{
				Names: []string{"topic"},
				Exprs: []ast.Expr{
					&ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "payload"},
						Key:    &ast.StringExpr{Value: "respond_to"},
					},
				},
			},
		},
	}
	graph := cfg.Build(fn)
	bindings := bind.Bind(fn, nil)
	evidence := trace.GraphEvidence(graph, bindings)
	guards := guard.CollectTypeGuards(graph, evidence.Branches, bindings)

	payloadSym, ok := bindings.SymbolOf(condExpr.Lhs.(*ast.FuncCallExpr).Args[0].(*ast.AttrGetExpr).Object.(*ast.IdentExpr))
	if !ok || payloadSym == 0 {
		t.Fatal("expected payload symbol")
	}
	wantKey := truthyKey(t, payloadSym, constraint.Segment{Kind: constraint.SegmentField, Name: "respond_to"})
	wantType := narrow.BuiltinTypeKey("string")

	found := false
	for _, atPoint := range guards {
		if tk, ok := atPoint[wantKey]; ok && tk == wantType {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected propagated type guard %v -> %v", wantKey, wantType)
	}
}

func TestExtractTypeEqualityProbe(t *testing.T) {
	target := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "page"},
		Key:    &ast.StringExpr{Value: "placement"},
	}
	expr := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{target},
		},
		Rhs: &ast.StringExpr{Value: "string"},
	}

	probe, ok := guard.ExtractTypeEqualityProbe(expr)
	if !ok {
		t.Fatal("expected type equality probe")
	}
	if probe.Expr != target {
		t.Fatal("expected probe expression to be preserved")
	}
	if probe.Key != narrow.BuiltinTypeKey("string") {
		t.Fatalf("probe key = %v, want string key", probe.Key)
	}
	if got := guard.TypeForTypeKey(probe.Key); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("probe type = %v, want string", got)
	}
}

func TestEvaluateTypeProbeComparison_ProvesDisjointFalse(t *testing.T) {
	cmp := typeProbeComparison("==", &ast.StringExpr{Value: "merge"}, "table")
	got := guard.EvaluateTypeProbeComparison(typ.LiteralString("merge"), cmp)
	if got != typ.False {
		t.Fatalf("type(\"merge\") == \"table\" = %v, want false", got)
	}
}

func TestEvaluateTypeProbeComparison_ProvesSubtypeTrue(t *testing.T) {
	cmp := typeProbeComparison("==", &ast.StringExpr{Value: "merge"}, "string")
	got := guard.EvaluateTypeProbeComparison(typ.LiteralString("merge"), cmp)
	if got != typ.True {
		t.Fatalf("type(\"merge\") == \"string\" = %v, want true", got)
	}
}

func TestEvaluateTypeProbeComparison_KeepsUnionUncertain(t *testing.T) {
	cmp := typeProbeComparison("==", &ast.IdentExpr{Value: "content"}, "table")
	observed := typ.NewUnion(typ.String, typ.NewRecord().Field("text", typ.String).Build())
	got := guard.EvaluateTypeProbeComparison(observed, cmp)
	if got != typ.Boolean {
		t.Fatalf("type(string|table) == \"table\" = %v, want boolean", got)
	}
}

func TestEvaluateTypeProbeComparison_ProvesNotEqualTrue(t *testing.T) {
	cmp := typeProbeComparison("~=", &ast.StringExpr{Value: "merge"}, "table")
	got := guard.EvaluateTypeProbeComparison(typ.LiteralString("merge"), cmp)
	if got != typ.True {
		t.Fatalf("type(\"merge\") ~= \"table\" = %v, want true", got)
	}
}

func typeProbeComparison(op string, target ast.Expr, typeName string) guard.TypeProbeComparison {
	expr := &ast.RelationalOpExpr{
		Operator: op,
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{target},
		},
		Rhs: &ast.StringExpr{Value: typeName},
	}
	cmp, ok := guard.ExtractTypeProbeComparison(expr)
	if !ok {
		panic("test constructed invalid type probe")
	}
	return cmp
}

func TestNarrowTableFieldsByGuard_TypeGuardNarrowsAny(t *testing.T) {
	valueExpr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "payload"},
		Key:    &ast.StringExpr{Value: "respond_to"},
	}
	rec := typ.NewRecord().Field("respond_to", typ.Any).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "respond_to"}, Value: valueExpr},
		},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"payload"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{tbl},
			},
		},
	}
	bindings := bind.Bind(fn, nil)
	payloadSym, _ := bindings.SymbolOf(valueExpr.Object.(*ast.IdentExpr))
	typeGuards := map[cfg.Point]map[guard.TruthyPathKey]narrow.TypeKey{
		1: {
			truthyKey(t, payloadSym, constraint.Segment{Kind: constraint.SegmentField, Name: "respond_to"}): narrow.BuiltinTypeKey("string"),
		},
	}

	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, bindings, nil, typeGuards)
	out, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", result)
	}
	if len(out.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(out.Fields))
	}
	if !typ.TypeEquals(out.Fields[0].Type, typ.String) {
		t.Fatalf("expected narrowed string field, got %s", out.Fields[0].Type.String())
	}
}

func TestNarrowTableFieldsByGuard_StaticStringIndexDoesNotCollapseToRecordField(t *testing.T) {
	valueExpr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "payload"},
		Key:    &ast.StringExpr{Value: "x-y"},
	}
	rec := typ.NewRecord().OptField("x-y", typ.String).Build()
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "x-y"}, Value: valueExpr},
		},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"payload"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{tbl},
			},
		},
	}
	bindings := bind.Bind(fn, nil)
	payloadSym, _ := bindings.SymbolOf(valueExpr.Object.(*ast.IdentExpr))
	guards := map[cfg.Point]map[guard.TruthyPathKey]bool{
		1: {
			truthyKey(t, payloadSym, constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"}): true,
		},
	}

	result := guard.NarrowTableFieldsByGuard(rec, tbl, 1, bindings, guards, nil)
	if result != rec {
		t.Fatalf("string-index source must not collapse to record field identity, got %s", result.String())
	}
}

func truthyKey(t *testing.T, sym cfg.SymbolID, segs ...constraint.Segment) guard.TruthyPathKey {
	t.Helper()
	key, ok := guard.TruthyPathKeyFromSegments(sym, segs)
	if !ok {
		t.Fatalf("invalid truthy key for symbol %d and segments %#v", sym, segs)
	}
	return key
}
