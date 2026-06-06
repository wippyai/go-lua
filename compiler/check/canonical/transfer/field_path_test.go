package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEvalTablePreservesStructuralTableKeys(t *testing.T) {
	tr := &Transfer{}
	out := flow.PointState{}
	table := &ast.TableExpr{Fields: []*ast.Field{
		{
			Key:       &ast.StringExpr{Value: "field"},
			KeySyntax: ast.AttrKeyDot,
			Value:     &ast.StringExpr{Value: "dot"},
		},
		{
			Key:       &ast.StringExpr{Value: "raw-key"},
			KeySyntax: ast.AttrKeyIndex,
			Value:     &ast.NumberExpr{Value: "42"},
		},
		{
			Key:       &ast.NumberExpr{Value: "1"},
			KeySyntax: ast.AttrKeyIndex,
			Value:     &ast.TrueExpr{},
		},
	}}

	av, ok := tr.evalTable(&out, table, nil)
	if !ok {
		t.Fatal("evalTable did not resolve static table literal")
	}
	rec, ok := av.ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("evalTable projected %T %[1]v, want record", av.ProjectValue())
	}
	if rec.GetField("field") == nil {
		t.Fatalf("missing dot field in %v", rec)
	}
	if rec.GetStaticStringIndex("raw-key") == nil {
		t.Fatalf("missing static string index in %v", rec)
	}
	if rec.GetStaticIntIndex(1) == nil {
		t.Fatalf("missing static int index in %v", rec)
	}
}

func TestEvalTableEmptyLiteralIsFreshAllocation(t *testing.T) {
	tr := &Transfer{}
	av, ok := tr.evalTable(&flow.PointState{}, &ast.TableExpr{}, nil)
	if !ok {
		t.Fatal("evalTable({}) did not resolve")
	}
	if !av.IsFreshAllocation() {
		t.Fatalf("evalTable({}) = %v, want fresh allocation", av.ProjectValue())
	}
}

func TestEvalTableCreateNestedFieldPreservesArrayAllocationSeed(t *testing.T) {
	tr, tableIdent := transferWithBoundTableGlobal()
	table := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       &ast.StringExpr{Value: "items"},
		KeySyntax: ast.AttrKeyDot,
		Value:     tableCreateCall(tableIdent, "4", "0"),
	}}}

	av, ok := tr.evalTable(&flow.PointState{}, table, nil)
	if !ok {
		t.Fatal("evalTable did not resolve table.create field")
	}
	items, ok := product.MemberOf(av, value.MemberField("items"))
	if !ok || items.IsZero() {
		t.Fatalf("items field missing from %v", av.ProjectValue())
	}
	arr, ok := items.ProjectValue().(*typ.Array)
	if !ok || !arr.Fresh {
		t.Fatalf("items = %T %[1]v, want fresh array allocation", items.ProjectValue())
	}
}

func TestEvalTableCreateHashCapacityUsesFreshTableSeed(t *testing.T) {
	tr, tableIdent := transferWithBoundTableGlobal()
	av, ok := tr.evalExpr(&flow.PointState{}, tableCreateCall(tableIdent, "0", "16"), nil)
	if !ok {
		t.Fatal("table.create hash allocation did not resolve")
	}
	if !av.IsFreshAllocation() {
		t.Fatalf("table.create(0, 16) = %v, want fresh table allocation", av.ProjectValue())
	}
}

func transferWithBoundTableGlobal() (*Transfer, *ast.IdentExpr) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil, "table")
	tableIdent := &ast.IdentExpr{Value: "table"}
	const tableSym = cfg.SymbolID(901)
	in.Graph.Bindings().Bind(tableIdent, tableSym)
	in.Graph.Bindings().SetKind(tableSym, cfg.SymbolGlobal)
	in.Graph.Bindings().SetName(tableSym, "table")
	return New(in, Config{}), tableIdent
}

func tableCreateCall(tableIdent *ast.IdentExpr, narray, nhash string) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{
		Func: &ast.AttrGetExpr{
			Object:    tableIdent,
			Key:       &ast.StringExpr{Value: "create"},
			KeySyntax: ast.AttrKeyDot,
		},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: narray},
			&ast.NumberExpr{Value: nhash},
		},
	}
}

func TestEvalLogicalDefaultUsesRuntimeNilForMissingTableField(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	in := input.BuildFromFunction(&ast.FunctionExpr{ParList: &ast.ParList{}}, nil, nil)
	const entrySym = cfg.SymbolID(101)
	in.Graph.Bindings().Bind(entry, entrySym)
	in.Graph.Bindings().SetName(entrySym, entry.Value)
	tr := New(in, Config{})
	out := flow.PointState{}
	tr.setSymbolValue(&out, entrySym, product.FromType(typ.NewRecord().
		Field("data", typ.NewRecord().Build()).
		Build()), false)

	data := &ast.AttrGetExpr{
		Object:    entry,
		Key:       &ast.StringExpr{Value: "data"},
		KeySyntax: ast.AttrKeyDot,
	}
	maxTokens := &ast.AttrGetExpr{
		Object:    data,
		Key:       &ast.StringExpr{Value: "max_tokens"},
		KeySyntax: ast.AttrKeyDot,
	}
	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs: &ast.LogicalOpExpr{
			Operator: "and",
			Lhs:      data,
			Rhs:      maxTokens,
		},
		Rhs: &ast.NumberExpr{Value: "0"},
	}

	got, ok := tr.evalExpr(&out, expr, nil)
	want := typ.LiteralInt(0)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("evalExpr(default) = %v, %v; want %v,true", got.ProjectValue(), ok, want)
	}
}

func TestMethodSelfPlaceWritePublishesPrototypeSelf(t *testing.T) {
	const (
		instanceSym = cfg.SymbolID(1101)
		protoSym    = cfg.SymbolID(1103)
	)
	prototype := typ.NewRecord().
		Field("all", typ.Func().Returns(typ.NewArray(typ.String), typ.Nil).Build()).
		Build()
	instance, ok := product.WithMetatable(
		product.FromType(typ.NewRecord().Build()),
		product.FromType(prototype),
	)
	if !ok {
		t.Fatal("test setup failed to attach metatable")
	}
	tr := &Transfer{
		prototypeReceiverSym: protoSym,
		prototypeSelfSymbol:  instanceSym,
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(instanceSym): instance,
		},
	}

	changed := tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: instanceSym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("_session_id"),
			}},
		},
		Value:       product.FromType(typ.String),
		RecordProto: true,
	})
	if !changed {
		t.Fatal("write effect reported no change")
	}
	self, ok := out.PrototypeSelf.Value(protoSym)
	if !ok {
		root, _ := tr.symbolValue(&out, instanceSym)
		t.Fatalf("PrototypeSelf missing for proto %d: %s instances=%s root=%v", protoSym, out.PrototypeSelf.Format(), out.PrototypeInstances.Format(), root.ProjectValue())
	}
	sessionID, ok := product.RuntimeMemberOf(self, value.MemberField("_session_id"))
	if !ok || !typ.TypeEquals(sessionID.ProjectValue(), typ.String) {
		t.Fatalf("published self._session_id = %v/%v, want string; self=%v", sessionID.ProjectValue(), ok, self.ProjectValue())
	}
}

func TestPrototypeInstanceWriteDoesNotPublishConstructionHistory(t *testing.T) {
	const (
		instanceSym = cfg.SymbolID(1105)
		protoSym    = cfg.SymbolID(1106)
	)
	tr := &Transfer{}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(instanceSym): product.FromType(typ.NewRecord().Build()),
		},
		PrototypeInstances: flow.PrototypeInstancesOf([]flow.PrototypeInstanceEntry{{
			Symbol:     instanceSym,
			Prototypes: []cfg.SymbolID{protoSym},
		}}),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: instanceSym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("_session_id"),
			}},
		},
		Value:       product.FromType(typ.String),
		RecordProto: true,
	})
	if _, ok := out.PrototypeSelf.Value(protoSym); ok {
		t.Fatalf("constructor-local write published intermediate PrototypeSelf: %s", out.PrototypeSelf.Format())
	}
}

func TestSetMetatableAssignmentBindsPrototypeInstance(t *testing.T) {
	const (
		targetSym = cfg.SymbolID(1201)
		metaSym   = cfg.SymbolID(1202)
		protoSym  = cfg.SymbolID(1203)
		setSym    = cfg.SymbolID(1204)
	)
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil, "setmetatable")
	set := &ast.IdentExpr{Value: "setmetatable"}
	meta := &ast.IdentExpr{Value: "context_query"}
	in.Graph.Bindings().Bind(set, setSym)
	in.Graph.Bindings().SetKind(setSym, cfg.SymbolGlobal)
	in.Graph.Bindings().SetName(setSym, "setmetatable")
	in.Graph.Bindings().Bind(meta, metaSym)
	in.Graph.Bindings().SetName(metaSym, "context_query")

	prototype := typ.NewRecord().Field("all", typ.Func().Returns(typ.NewArray(typ.String), typ.Nil).Build()).Build()
	instance, ok := product.WithMetatable(product.FromType(typ.NewRecord().Build()), product.FromType(prototype))
	if !ok {
		t.Fatal("test setup failed to attach metatable")
	}
	tr := New(in, Config{})
	tr.metatablePrototypeBySym = map[cfg.SymbolID]cfg.SymbolID{metaSym: protoSym}
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(targetSym): instance,
		flow.SymbolValueKey(metaSym):   product.FromType(prototype),
	}}
	call := &ast.FuncCallExpr{
		Func: set,
		Args: []ast.Expr{&ast.TableExpr{}, meta},
	}

	if !tr.applySetMetatableInstanceBinding(&out, call, targetSym) {
		t.Fatal("setmetatable source did not bind target as a prototype instance")
	}
	protos, ok := out.PrototypeInstances.Prototypes(targetSym)
	if !ok || len(protos) != 1 || protos[0] != protoSym {
		t.Fatalf("PrototypeInstances[%d] = %v/%v; map=%s", targetSym, protos, ok, out.PrototypeInstances.Format())
	}
	if _, ok := out.PrototypeSelf.Value(protoSym); ok {
		t.Fatalf("setmetatable binding published construction history: %s", out.PrototypeSelf.Format())
	}
}

func TestReturnedPrototypeInstancePublishesFinalPrototypeSelf(t *testing.T) {
	const (
		targetSym = cfg.SymbolID(1211)
		protoSym  = cfg.SymbolID(1212)
	)
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	self := &ast.IdentExpr{Value: "self"}
	in.Graph.Bindings().Bind(self, targetSym)
	in.Graph.Bindings().SetName(targetSym, "self")
	tr := New(in, Config{})
	finalSelf := product.FromType(typ.NewRecord().Field("session_id", typ.String).Field("user_id", typ.String).Build())
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(targetSym): finalSelf,
		},
		PrototypeInstances: flow.PrototypeInstancesOf([]flow.PrototypeInstanceEntry{{
			Symbol:     targetSym,
			Prototypes: []cfg.SymbolID{protoSym},
		}}),
	}

	tr.publishReturnedPrototypeSelf(&out, self)
	published, ok := out.PrototypeSelf.Value(protoSym)
	if !ok {
		t.Fatalf("returned instance did not publish PrototypeSelf: %s", out.PrototypeSelf.Format())
	}
	sessionID, ok := product.RuntimeMemberOf(published, value.MemberField("session_id"))
	if !ok || !typ.TypeEquals(sessionID.ProjectValue(), typ.String) {
		t.Fatalf("published session_id = %v/%v, want string; self=%v", sessionID.ProjectValue(), ok, published.ProjectValue())
	}
	userID, ok := product.RuntimeMemberOf(published, value.MemberField("user_id"))
	if !ok || !typ.TypeEquals(userID.ProjectValue(), typ.String) {
		t.Fatalf("published user_id = %v/%v, want string; self=%v", userID.ProjectValue(), ok, published.ProjectValue())
	}
}

func TestNestedDynamicIndexWriteUpdatesRootMapValue(t *testing.T) {
	root := &ast.IdentExpr{Value: "subscribers"}
	cid := &ast.IdentExpr{Value: "cid"}
	pid := &ast.IdentExpr{Value: "pid"}
	base := &ast.AttrGetExpr{
		Object:    root,
		Key:       cid,
		KeySyntax: ast.AttrKeyIndex,
	}
	rootSym := cfg.SymbolID(201)
	cidSym := cfg.SymbolID(202)
	pidSym := cfg.SymbolID(203)
	in := input.BuildFromFunction(&ast.FunctionExpr{ParList: &ast.ParList{}}, nil, nil)
	in.Graph.Bindings().Bind(root, rootSym)
	in.Graph.Bindings().SetName(rootSym, root.Value)
	in.Graph.Bindings().Bind(cid, cidSym)
	in.Graph.Bindings().SetName(cidSym, cid.Value)
	in.Graph.Bindings().Bind(pid, pidSym)
	in.Graph.Bindings().SetName(pidSym, pid.Value)
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.NewMap(typ.String, typ.NewRecord().Build())),
		flow.SymbolValueKey(cidSym):  product.FromType(typ.String),
		flow.SymbolValueKey(pidSym):  product.FromType(typ.String),
	}}

	target := cfg.AssignTarget{
		Kind: cfg.TargetIndex,
		Base: base,
		Key:  pid,
	}
	tr.applyContainerWrite(&out, target, &ast.TrueExpr{}, nil)

	rootAV, ok := tr.symbolValue(&out, rootSym)
	if !ok || rootAV.IsZero() {
		t.Fatalf("root value missing after nested dynamic write: env=%v cells=%s", out.Env, out.Cells.Format())
	}
	outerValue, ok := querycore.Index(rootAV.ProjectValue(), typ.String)
	if !ok {
		t.Fatalf("outer map read did not resolve: %v", rootAV.ProjectValue())
	}
	inner := narrow.RemoveNil(outerValue)
	if got := querycore.EntryKeyType(inner); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("inner entry key type = %v, want string; root=%v inner=%v", got, rootAV.ProjectValue(), inner)
	}
	if got := querycore.EntryValueType(inner); !typ.TypeEquals(got, typ.True) {
		t.Fatalf("inner entry value type = %v, want true; root=%v inner=%v", got, rootAV.ProjectValue(), inner)
	}
}

func TestPlaceWriteGeneralizesMixedStaticAndDynamicSteps(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	root := &ast.IdentExpr{Value: "subscribers"}
	cid := &ast.IdentExpr{Value: "cid"}
	pid := &ast.IdentExpr{Value: "pid"}
	rootSym := cfg.SymbolID(301)
	cidSym := cfg.SymbolID(302)
	pidSym := cfg.SymbolID(303)
	in.Graph.Bindings().Bind(root, rootSym)
	in.Graph.Bindings().SetName(rootSym, root.Value)
	in.Graph.Bindings().Bind(cid, cidSym)
	in.Graph.Bindings().SetName(cidSym, cid.Value)
	in.Graph.Bindings().Bind(pid, pidSym)
	in.Graph.Bindings().SetName(pidSym, pid.Value)

	t.Run("dynamic_then_static", func(t *testing.T) {
		tr := New(in, Config{})
		out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(rootSym): product.FromType(typ.NewMap(typ.String, typ.NewRecord().Build())),
			flow.SymbolValueKey(cidSym):  product.FromType(typ.String),
		}}
		target := cfg.AssignTarget{
			Kind: cfg.TargetField,
			Expr: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object:    root,
					Key:       cid,
					KeySyntax: ast.AttrKeyIndex,
				},
				Key:       &ast.StringExpr{Value: "active"},
				KeySyntax: ast.AttrKeyDot,
			},
		}

		tr.applyContainerWrite(&out, target, &ast.TrueExpr{}, nil)
		rootAV, ok := tr.symbolValue(&out, rootSym)
		if !ok || rootAV.IsZero() {
			t.Fatalf("root value missing after dynamic/static write: env=%v", out.Env)
		}
		elem, ok := product.IndexOf(rootAV, product.FromType(typ.String))
		if !ok {
			t.Fatalf("outer dynamic read did not resolve: %v", rootAV.ProjectValue())
		}
		active, ok := product.MemberOf(product.NarrowPresent(elem), value.MemberField("active"))
		if !ok || !typ.TypeEquals(active.ProjectValue(), typ.True) {
			t.Fatalf("active member = %v, %v; want true,true; root=%v", active.ProjectValue(), ok, rootAV.ProjectValue())
		}
	})

	t.Run("static_then_dynamic", func(t *testing.T) {
		tr := New(in, Config{})
		out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(rootSym): product.FromType(typ.NewRecord().Build()),
			flow.SymbolValueKey(pidSym):  product.FromType(typ.String),
		}}
		target := cfg.AssignTarget{
			Kind: cfg.TargetIndex,
			Expr: &ast.AttrGetExpr{
				Object: &ast.AttrGetExpr{
					Object:    root,
					Key:       &ast.StringExpr{Value: "active"},
					KeySyntax: ast.AttrKeyDot,
				},
				Key:       pid,
				KeySyntax: ast.AttrKeyIndex,
			},
		}

		tr.applyContainerWrite(&out, target, &ast.TrueExpr{}, nil)
		rootAV, ok := tr.symbolValue(&out, rootSym)
		if !ok || rootAV.IsZero() {
			t.Fatalf("root value missing after static/dynamic write: env=%v", out.Env)
		}
		active, ok := product.MemberOf(rootAV, value.MemberField("active"))
		if !ok {
			t.Fatalf("active member missing after static/dynamic write: root=%v", rootAV.ProjectValue())
		}
		got, ok := product.IndexOf(active, product.FromType(typ.String))
		if !ok || !typ.TypeEquals(narrow.RemoveNil(got.ProjectValue()), typ.True) {
			t.Fatalf("active[string] = %v, %v; want true,true; root=%v", got.ProjectValue(), ok, rootAV.ProjectValue())
		}
	})
}

func TestPlaceStaticProjectionOwnsPathKeys(t *testing.T) {
	sym := cfg.SymbolID(41)
	place := Place{
		Root:     sym,
		RootName: "root",
		Steps: []PlaceStep{
			{Kind: PlaceStepStaticMember, Member: value.MemberField("items")},
			{Kind: PlaceStepStaticMember, Member: value.MemberStringIndex("active")},
		},
	}

	path, ok := place.StaticPath()
	if !ok || path.Root != "root" || path.Symbol != sym || len(path.Segments) != 2 {
		t.Fatalf("StaticPath = %#v/%v, want root/static two-segment path", path, ok)
	}
	pathKey, ok := place.StaticPathKey()
	if !ok || pathKey != path.Key() {
		t.Fatalf("StaticPathKey = %q/%v, want %q", pathKey, ok, path.Key())
	}
	symbolKey, ok := symbolPathKey(place)
	if !ok || symbolKey != flow.SymbolPathKey(sym, path.Segments) {
		t.Fatalf("SymbolPathKey = %q/%v, want %q", symbolKey, ok, flow.SymbolPathKey(sym, path.Segments))
	}

	dynamic := Place{
		Root:     sym,
		RootName: "root",
		Steps: []PlaceStep{
			{Kind: PlaceStepStaticMember, Member: value.MemberField("items")},
			{Kind: PlaceStepDynamicIndex, Key: product.FromType(typ.String)},
			{Kind: PlaceStepStaticMember, Member: value.MemberField("name")},
		},
	}
	if path, ok := dynamic.StaticPath(); ok {
		t.Fatalf("dynamic StaticPath = %#v, want no exact path", path)
	}
	prefix, ok := dynamic.StaticPrefixPath()
	if !ok || prefix.Symbol != sym || len(prefix.Segments) != 1 || prefix.Segments[0].Name != "items" {
		t.Fatalf("dynamic StaticPrefixPath = %#v/%v, want root.items", prefix, ok)
	}
}

func TestStaticMemberWriteFactsInstallAndKillByStructuralPath(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(7)
	fieldSeg := []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}}
	indexSeg := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}}
	emptySeg := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}}

	out := flow.PointState{}
	tr.installStaticMemberWriteFact(&out, sym, fieldSeg, product.FromType(typ.String))
	tr.installStaticMemberWriteFact(&out, sym, indexSeg, product.FromType(typ.Number))
	tr.installStaticMemberWriteFact(&out, sym, emptySeg, product.FromType(typ.Boolean))

	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, fieldSeg); !ok {
		t.Fatal("missing dot-field write fact")
	}
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, indexSeg); !ok {
		t.Fatal("missing string-index write fact")
	}
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, emptySeg); !ok {
		t.Fatal("missing empty-string-index write fact")
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: sym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberStringIndex("foo"),
			}},
		},
		RecordStatic: true,
	})
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, fieldSeg); !ok {
		t.Fatal("string-index kill removed dot-field fact")
	}
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, indexSeg); ok {
		t.Fatal("string-index kill kept matching string-index fact")
	}
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, emptySeg); !ok {
		t.Fatal("string-index kill removed unrelated empty-string-index fact")
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:        Place{Root: sym},
		RecordStatic: true,
	})
	if entries := out.StaticMembers.Entries(); len(entries) != 0 {
		t.Fatalf("root kill left facts: %v", entries)
	}
}

func TestStaticMemberWriteInvalidationKillsAncestorSnapshotsOnly(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(70)
	parentSeg := []constraint.Segment{{Kind: constraint.SegmentField, Name: "active_sessions"}}
	writtenSeg := append(append([]constraint.Segment(nil), parentSeg...), constraint.Segment{Kind: constraint.SegmentIndexString, Name: "s1"})
	siblingSeg := append(append([]constraint.Segment(nil), parentSeg...), constraint.Segment{Kind: constraint.SegmentIndexString, Name: "s2"})

	out := flow.PointState{
		StaticMembers: flow.StaticMemberFactsDomain.Top().
			WithAddress(testStaticMemberAddress(t, sym, parentSeg), product.FromType(typ.NewRecord().Build())).
			WithAddress(testStaticMemberAddress(t, sym, writtenSeg), product.FromType(typ.String)).
			WithAddress(testStaticMemberAddress(t, sym, siblingSeg), product.FromType(typ.Number)),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: sym,
			Steps: []PlaceStep{
				{Kind: PlaceStepStaticMember, Member: value.MemberField("active_sessions")},
				{Kind: PlaceStepStaticMember, Member: value.MemberStringIndex("s1")},
			},
		},
		RecordStatic: true,
	})

	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, parentSeg); ok {
		t.Fatalf("descendant write kept stale parent static fact: %s", out.StaticMembers.Format())
	}
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, writtenSeg); ok {
		t.Fatalf("descendant write kept written child static fact: %s", out.StaticMembers.Format())
	}
	if got, ok := testStaticMemberValue(t, out.StaticMembers, sym, siblingSeg); !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("descendant write lost sibling static fact: %s", out.StaticMembers.Format())
	}
}

func TestStaticMemberWriteFactRequiresDefinitelyPresentValue(t *testing.T) {
	tr := &Transfer{}
	sym := cfg.SymbolID(7)
	segs := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "maybe"}}
	out := flow.PointState{}

	tr.installStaticMemberWriteFact(&out, sym, segs, product.FromType(typ.NewOptional(typ.String)))
	if _, ok := testStaticMemberValue(t, out.StaticMembers, sym, segs); ok {
		t.Fatal("optional write must not install a must-present member fact")
	}

	tr.installStaticMemberWriteFact(&out, sym, segs, product.FromType(typ.String))
	if got, ok := testStaticMemberValue(t, out.StaticMembers, sym, segs); !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("present write fact = %v, %v; want string,true", got.ProjectValue(), ok)
	}
}

func TestDynamicExactKeyWriteInstallsStaticMemberFact(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	keyExpr := &ast.IdentExpr{Value: "key"}
	baseSym := cfg.SymbolID(31)
	keySym := cfg.SymbolID(32)
	in.Graph.Bindings().Bind(keyExpr, keySym)
	in.Graph.Bindings().SetName(keySym, "key")

	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(baseSym): product.FromType(typ.NewMap(typ.String, typ.Number)),
			flow.SymbolValueKey(keySym):  product.FromType(typ.LiteralString("foo")),
		},
	}

	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseSymbol: baseSym,
		BaseName:   "t",
		Key:        keyExpr,
	}, &ast.NumberExpr{Value: "42"}, nil)

	got, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddress(t, baseSym, []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "foo"},
	}))
	if !ok {
		t.Fatal("dynamic exact key write did not install must static-member fact")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.LiteralInt(42)) {
		t.Fatalf("static-member fact = %v, want literal 42", got.ProjectValue())
	}
}

func TestStaticMemberWriteFactUsesPlaceForMixedStaticPath(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	root := &ast.IdentExpr{Value: "registry"}
	rootSym := cfg.SymbolID(33)
	in.Graph.Bindings().Bind(root, rootSym)
	in.Graph.Bindings().SetName(rootSym, root.Value)

	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.NewRecord().Build()),
	}}
	target := cfg.AssignTarget{
		Kind: cfg.TargetIndex,
		Expr: &ast.AttrGetExpr{
			Object: &ast.AttrGetExpr{
				Object:    root,
				Key:       &ast.StringExpr{Value: "handlers"},
				KeySyntax: ast.AttrKeyIndex,
			},
			Key:       &ast.StringExpr{Value: "ready"},
			KeySyntax: ast.AttrKeyDot,
		},
	}

	tr.applyContainerWrite(&out, target, &ast.TrueExpr{}, nil)

	got, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddress(t, rootSym, []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "handlers"},
		{Kind: constraint.SegmentField, Name: "ready"},
	}))
	if !ok {
		t.Fatalf("mixed static path member fact missing: %#v", out.StaticMembers)
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.True) {
		t.Fatalf("mixed static path fact = %v, want true", got.ProjectValue())
	}
}

func TestEvalAttrGetUsesSyntaxAwareStaticMemberFacts(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	base := &ast.IdentExpr{Value: "obj"}
	sym := cfg.SymbolID(23)
	in.Graph.Bindings().Bind(base, sym)
	in.Graph.Bindings().SetName(sym, "obj")

	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.NewRecord().Build()),
		},
	}
	out.StaticMembers = out.StaticMembers.WithAddress(
		testStaticMemberAddress(t, sym, []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}}),
		product.FromType(typ.String),
	)
	out.StaticMembers = out.StaticMembers.WithAddress(
		testStaticMemberAddress(t, sym, []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}}),
		product.FromType(typ.Number),
	)

	dot := &ast.AttrGetExpr{
		Object:    base,
		Key:       &ast.StringExpr{Value: "foo"},
		KeySyntax: ast.AttrKeyDot,
	}
	dotValue, ok := tr.evalAttrGet(&out, dot, nil)
	if !ok {
		t.Fatal("dot read did not resolve")
	}
	if !typ.TypeEquals(dotValue.ProjectValue(), typ.String) {
		t.Fatalf("dot read = %v; want string", dotValue.ProjectValue())
	}

	bracket := &ast.AttrGetExpr{
		Object:    base,
		Key:       &ast.StringExpr{Value: "foo"},
		KeySyntax: ast.AttrKeyIndex,
	}
	bracketValue, ok := tr.evalAttrGet(&out, bracket, nil)
	if !ok {
		t.Fatal("bracket read did not resolve")
	}
	if !typ.TypeEquals(bracketValue.ProjectValue(), typ.Number) {
		t.Fatalf("bracket read = %v; want number", bracketValue.ProjectValue())
	}
}
