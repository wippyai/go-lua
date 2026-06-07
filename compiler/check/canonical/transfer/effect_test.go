package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSymbolWriteEffectClearsStaleProductAxes(t *testing.T) {
	sym := cfg.SymbolID(201)
	root := constraint.NewPath(sym, "value")
	fieldPath := root.Field("field")
	fieldKey := flow.SymbolPathKey(sym, fieldPath.Segments)
	ref := flow.FunctionRef{GraphID: 301}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 302}, flow.CaptureCellsDomain.Bottom(), nil)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Number),
		},
		StaticMembers: flow.StaticMemberFacts{}.WithAddress(testStaticMemberAddressKey(t, fieldKey), product.FromType(typ.Boolean)),
		KeyPresence:   testKeyPresenceWith(t, flow.KeyPresenceFacts{}, root, constraint.NewPath(cfg.SymbolID(202), "k")),
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target: flow.SymbolPathKey(sym, []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}}),
			Key:    product.FromType(typ.String),
			Value:  product.FromType(typ.Number),
		}),
		FunctionRefs: flow.WithFunctionRefPath(nil, fieldPath, flow.FunctionRefSetOf(ref)),
		ClosureRefs:  flow.WithClosureRefPath(nil, fieldPath, flow.ClosureRefSetOf(closure)),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:        Place{Root: sym},
		Value:        product.FromType(typ.String),
		Source:       &ast.NumberExpr{Value: "1"},
		References:   sourceReferenceWrite(),
		RecordStatic: true,
	})

	if got := out.Env[flow.SymbolValueKey(sym)].ProjectValue(); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("symbol value = %v, want string", got)
	}
	if _, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, fieldKey)); ok {
		t.Fatalf("stale static member survived symbol write: %s", out.StaticMembers.Format())
	}
	if testKeyPresenceHas(t, out.KeyPresence, root, constraint.NewPath(cfg.SymbolID(202), "k")) {
		t.Fatalf("stale key presence survived symbol write: %s", out.KeyPresence.Format())
	}
	if _, ok := testIndexWriteAdmission(t, out.IndexWrites, root.Field("items"), constraint.Path{}, typ.String); ok {
		t.Fatalf("stale index-write admission survived symbol write: %s", out.IndexWrites.Format())
	}
	if _, ok := flow.FunctionRefAtPath(out.FunctionRefs, fieldPath); ok {
		t.Fatalf("stale function ref survived symbol write: %#v", out.FunctionRefs)
	}
	if _, ok := flow.ClosureRefAtPath(out.ClosureRefs, fieldPath); ok {
		t.Fatalf("stale closure ref survived symbol write: %#v", out.ClosureRefs)
	}
}

func TestWriteEffectReplaysStaticAliasDescendantWrite(t *testing.T) {
	const (
		sSym   = cfg.SymbolID(1301)
		boxSym = cfg.SymbolID(1302)
	)
	sPath := constraint.NewPath(sSym, "s")
	boxCurPath := constraint.NewPath(boxSym, "box").Field("cur")
	fnType := typ.Func().Build()
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sSym): product.FromType(typ.NewRecord().
				OptField("hook", typ.Nil).
				Build()),
			flow.SymbolValueKey(boxSym): product.FromType(typ.NewRecord().
				Field("cur", typ.NewRecord().OptField("hook", typ.Nil).Build()).
				Build()),
		},
		ValueOrigins: flow.ValueOriginFacts{}.WithAddresses(
			testFlowPathAddress(t, boxCurPath),
			testFlowPathAddress(t, sPath),
			flow.ValueOriginAssignmentAlias,
			0,
		),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     boxSym,
			RootName: "box",
			Steps: []PlaceStep{
				{Kind: PlaceStepStaticMember, Member: value.MemberField("cur")},
				{Kind: PlaceStepStaticMember, Member: value.MemberField("hook")},
			},
		},
		Value:        product.FromType(fnType),
		References:   sourceReferenceWrite(),
		RecordStatic: true,
	})

	got, ok := flow.PointFactsOf(out).PathValue(sPath.Field("hook"))
	if !ok || !typ.TypeEquals(got.ProjectValue(), fnType) {
		t.Fatalf("alias replay s.hook = %v/%v, want function", got.ProjectValue(), ok)
	}
}

func TestUnresolvedContainerWriteInvalidatesStaleProductAxes(t *testing.T) {
	baseSym := cfg.SymbolID(301)
	root := constraint.NewPath(baseSym, "box")
	fieldPath := root.Field("field")
	fieldKey := flow.SymbolPathKey(baseSym, fieldPath.Segments)
	ref := flow.FunctionRef{GraphID: 302}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 303}, flow.CaptureCellsDomain.Bottom(), nil)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(baseSym): product.FromType(typ.NewRecord().Field("field", typ.String).Build()),
		},
		StaticMembers: flow.StaticMemberFacts{}.WithAddress(testStaticMemberAddressKey(t, fieldKey), product.FromType(typ.String)),
		KeyPresence:   testKeyPresenceWith(t, flow.KeyPresenceFacts{}, fieldPath, constraint.NewPath(cfg.SymbolID(304), "k")),
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target: flow.SymbolPathKey(baseSym, fieldPath.Segments),
			Key:    product.FromType(typ.String),
			Value:  product.FromType(typ.String),
		}),
		FunctionRefs: flow.WithFunctionRefPath(nil, fieldPath, flow.FunctionRefSetOf(ref)),
		ClosureRefs:  flow.WithClosureRefPath(nil, fieldPath, flow.ClosureRefSetOf(closure)),
	}

	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind:       cfg.TargetField,
		BaseName:   "box",
		BaseSymbol: baseSym,
		FieldPath:  []string{"field"},
	}, &ast.IdentExpr{Value: "unresolved"}, nil)

	if _, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, fieldKey)); ok {
		t.Fatalf("stale static member survived unresolved container write: %s", out.StaticMembers.Format())
	}
	if testKeyPresenceHas(t, out.KeyPresence, fieldPath, constraint.NewPath(cfg.SymbolID(304), "k")) {
		t.Fatalf("stale key presence survived unresolved container write: %s", out.KeyPresence.Format())
	}
	if _, ok := testIndexWriteAdmission(t, out.IndexWrites, fieldPath, constraint.Path{}, typ.String); ok {
		t.Fatalf("stale index-write admission survived unresolved container write: %s", out.IndexWrites.Format())
	}
	if _, ok := flow.FunctionRefAtPath(out.FunctionRefs, fieldPath); ok {
		t.Fatalf("stale function ref survived unresolved container write: %#v", out.FunctionRefs)
	}
	if _, ok := flow.ClosureRefAtPath(out.ClosureRefs, fieldPath); ok {
		t.Fatalf("stale closure ref survived unresolved container write: %#v", out.ClosureRefs)
	}
}

func TestSymbolWriteEffectSeedsKeyArrayProvenance(t *testing.T) {
	namesSym := cfg.SymbolID(401)
	container := constraint.NewPath(cfg.SymbolID(402), "container")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:         Place{Root: namesSym},
		Value:         product.FromType(typ.NewRecord().SetOpen(true).Build()),
		KeyArrayTable: container,
		RecordStatic:  true,
	})

	if tables := out.KeyPresence.KeyArrayTables(flow.SymbolPathKey(namesSym, nil)); len(tables) != 1 || tables[0] != flow.StablePathKey(container) {
		t.Fatalf("key-array provenance = %v, want %s", tables, flow.StablePathKey(container))
	}
}

func TestRootWriteEffectSeedsEmptyArrayInvariants(t *testing.T) {
	graphSym := cfg.SymbolID(411)
	graphPath := constraint.NewPath(graphSym, "graph")
	graphType := typ.NewRecord().
		Field("node_order", typ.NewArray(typ.Never)).
		Field("operations", typ.NewFreshArray()).
		Field("nodes", typ.NewFreshEmptyRecord()).
		Field("edges", typ.NewMap(typ.String, typ.Unknown)).
		Field("name", typ.String).
		Build()
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:        Place{Root: graphSym, RootName: "graph"},
		Value:        product.FromType(graphType),
		Source:       &ast.TableExpr{},
		RecordStatic: true,
	})

	if !out.KeyPresence.HasEmptyKeyArray(flow.StablePathKey(graphPath.Field("node_order"))) {
		t.Fatalf("node_order missing empty key-array invariant: %s", out.KeyPresence.Format())
	}
	if !out.KeyPresence.HasEmptyKeyArray(flow.StablePathKey(graphPath.Field("operations"))) {
		t.Fatalf("operations missing empty key-array invariant: %s", out.KeyPresence.Format())
	}
	if out.KeyPresence.HasEmptyKeyArray(flow.StablePathKey(graphPath.Field("name"))) {
		t.Fatalf("scalar sibling became an empty key-array carrier: %s", out.KeyPresence.Format())
	}
	if tables := out.KeyPresence.KeyArrayTables(flow.StablePathKey(graphPath.Field("node_order"))); len(tables) != 0 {
		t.Fatalf("root write enumerated concrete table pairs: %s", out.KeyPresence.Format())
	}
}

func TestRootWriteEffectSeedsEmptyArrayInvariantsForInferredRecord(t *testing.T) {
	graphSym := cfg.SymbolID(412)
	graphPath := constraint.NewPath(graphSym, "graph")
	graphBody := typ.NewRecord().
		Field("node_order", typ.NewArray(typ.Never)).
		Field("nodes", typ.NewFreshEmptyRecord()).
		Build()
	graphType := typ.NewRecursiveWithBody("Inferred", graphBody)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:        Place{Root: graphSym, RootName: "graph"},
		Value:        product.FromType(graphType),
		Source:       &ast.FuncCallExpr{},
		RecordStatic: true,
	})

	if !out.KeyPresence.HasEmptyKeyArray(flow.StablePathKey(graphPath.Field("node_order"))) {
		t.Fatalf("inferred record missing empty-array/table invariant: %s", out.KeyPresence.Format())
	}
}

func TestRootWriteEffectDoesNotSeedEmptyArrayInvariantFromReadAlias(t *testing.T) {
	graphSym := cfg.SymbolID(413)
	graphPath := constraint.NewPath(graphSym, "graph")
	graphType := typ.NewRecord().
		Field("node_order", typ.NewArray(typ.Never)).
		Field("nodes", typ.NewFreshEmptyRecord()).
		Build()
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:        Place{Root: graphSym, RootName: "graph"},
		Value:        product.FromType(graphType),
		Source:       &ast.AttrGetExpr{},
		RecordStatic: true,
	})

	if out.KeyPresence.HasEmptyKeyArray(flow.StablePathKey(graphPath.Field("node_order"))) {
		t.Fatalf("read alias seeded empty-array invariant: %s", out.KeyPresence.Format())
	}
}

func TestDynamicIndexWriteSeedsAdmissionFactOnlyWhenAdmitted(t *testing.T) {
	const tableSym = cfg.SymbolID(421)
	tablePath := constraint.NewPath(tableSym, "items")
	key := product.FromType(typ.String)
	val := product.FromType(typ.String)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(tableSym): product.FromType(typ.NewMap(typ.String, typ.Number)),
		},
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     tableSym,
			RootName: "items",
			Steps:    []PlaceStep{{Kind: PlaceStepDynamicIndex, Key: key}},
		},
		Value: val,
		IndexTarget: cfg.AssignTarget{
			Kind:       cfg.TargetIndex,
			BaseName:   "items",
			BaseSymbol: tableSym,
			Key:        &ast.IdentExpr{Value: "k"},
		},
		DynamicMode:  DynamicWriteForeign,
		RecordStatic: true,
	})

	got, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, constraint.Path{}, typ.String)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("IndexWrites admission = %v/%v, want string/true", got.ProjectValue(), ok)
	}

	out.IndexWrites = flow.IndexWriteAdmissionFactsDomain.Top()
	out.Env[flow.SymbolValueKey(tableSym)] = product.FromType(typ.NewReadonlyMap(typ.String, typ.Number))
	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     tableSym,
			RootName: "items",
			Steps:    []PlaceStep{{Kind: PlaceStepDynamicIndex, Key: key}},
		},
		Value: val,
		IndexTarget: cfg.AssignTarget{
			Kind:       cfg.TargetIndex,
			BaseName:   "items",
			BaseSymbol: tableSym,
			Key:        &ast.IdentExpr{Value: "k"},
		},
		DynamicMode:  DynamicWriteForeign,
		RecordStatic: true,
	})
	if _, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, constraint.Path{}, typ.String); ok {
		t.Fatalf("readonly map write seeded admission: %s", out.IndexWrites.Format())
	}
}

func TestForeignWriteToFreshEmptyTableKeepsIteratorTailAny(t *testing.T) {
	const tableSym = cfg.SymbolID(426)
	tablePath := constraint.NewPath(tableSym, "active_sessions")
	payload := typ.NewRecord().
		Field("created_at", typ.String).
		Field("last_activity", typ.NewOptional(typ.String)).
		Build()
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(tableSym): product.FromType(typ.NewFreshEmptyRecord()),
		},
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     tableSym,
			RootName: "active_sessions",
			Steps:    []PlaceStep{{Kind: PlaceStepDynamicIndex, Key: product.FromType(typ.LiteralString("s1"))}},
		},
		Value: product.FromType(payload),
		IndexTarget: cfg.AssignTarget{
			Kind:       cfg.TargetIndex,
			BaseName:   "active_sessions",
			BaseSymbol: tableSym,
			Key:        &ast.StringExpr{Value: "s1"},
		},
		DynamicMode:  DynamicWriteForeign,
		RecordStatic: true,
	})

	updated := out.Env[flow.SymbolValueKey(tableSym)].ProjectValue()
	iter := querycore.EntryValueType(updated)
	if !typ.TypeEquals(iter, typ.Any) {
		t.Fatalf("EntryValueType(updated fresh table) = %v, want any; updated=%v", iter, updated)
	}
	admitted, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, constraint.Path{}, typ.LiteralString("s1"))
	if !ok || !typ.TypeEquals(admitted.ProjectValue(), payload) {
		t.Fatalf("IndexWrites admission = %v/%v, want payload/true", admitted.ProjectValue(), ok)
	}
}

func TestDynamicIndexWriteAdmissionRejectsSealedAnnotatedTarget(t *testing.T) {
	const tableSym = cfg.SymbolID(431)
	tablePath := constraint.NewPath(tableSym, "items")
	declared := typ.NewRecord().
		Field("count", typ.Integer).
		Field("name", typ.String).
		Build()
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{
		tableSym: declared,
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(tableSym): product.FromType(declared),
		},
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     tableSym,
			RootName: "items",
			Steps:    []PlaceStep{{Kind: PlaceStepDynamicIndex, Key: product.FromType(typ.String)}},
		},
		Value: product.FromType(typ.Integer),
		IndexTarget: cfg.AssignTarget{
			Kind:       cfg.TargetIndex,
			BaseName:   "items",
			BaseSymbol: tableSym,
			Key:        &ast.IdentExpr{Value: "k"},
		},
		DynamicMode:  DynamicWriteForeign,
		RecordStatic: true,
	})

	if _, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, constraint.Path{}, typ.String); ok {
		t.Fatalf("sealed target seeded admission: %s", out.IndexWrites.Format())
	}
	if got := out.Env[flow.SymbolValueKey(tableSym)].ProjectValue(); !typ.TypeEquals(got, declared) {
		t.Fatalf("sealed rejected write mutated product state: got %v, want %v", got, declared)
	}
}

func TestSealedDynamicIndexWriteAdmissionUsesReadBackSlot(t *testing.T) {
	const selfSym = cfg.SymbolID(436)
	dataTargets := typ.NewMap(typ.String, typ.String)
	node := typ.NewRecord().
		Field("config", typ.NewRecord().Field("data_targets", dataTargets).Build()).
		Build()
	store := typ.NewRecord().
		Field("nodes", typ.NewMap(typ.String, node)).
		Build()
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{selfSym: store}
	key := product.FromType(typ.String)
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(selfSym): product.FromType(store),
		},
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     selfSym,
			RootName: "self",
			Steps: []PlaceStep{
				{Kind: PlaceStepStaticMember, Member: value.MemberField("nodes")},
				{Kind: PlaceStepDynamicIndex, Key: key},
			},
		},
		Value: product.FromType(node),
		IndexTarget: cfg.AssignTarget{
			Kind: cfg.TargetIndex,
			Key:  &ast.IdentExpr{Value: "id"},
		},
		DynamicMode:  DynamicWriteForeign,
		RecordStatic: true,
	})

	admitted, ok := testIndexWriteAdmission(t, out.IndexWrites, constraint.NewPath(selfSym, "self").Field("nodes"), constraint.Path{}, typ.String)
	if !ok {
		t.Fatalf("sealed map write did not seed admission: %s", out.IndexWrites.Format())
	}
	config, ok := product.FieldOf(admitted, "config")
	if !ok {
		t.Fatalf("admitted value has no config field: %v", admitted.ProjectValue())
	}
	targets, ok := product.FieldOf(config, "data_targets")
	if !ok || !typ.TypeEquals(targets.ProjectValue(), dataTargets) {
		t.Fatalf("admitted data_targets = %v/%v, want %v/true; admitted=%v", targets.ProjectValue(), ok, dataTargets, admitted.ProjectValue())
	}
}

func TestDynamicIndexWriteProofKeepsOpaqueUnsealedReadbackLightweight(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(438), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(439), "node_id")
	out := flow.PointState{}

	proof, ok := flow.MapWriteTransactionOfPath(flow.MapWritePathTransaction{
		TablePath:              tablePath,
		KeyPath:                keyPath,
		KeyValue:               product.FromType(typ.Unknown),
		Value:                  product.FromType(typ.String),
		AllowOpaqueKeyReadback: false,
	})
	if !ok {
		t.Fatal("dynamic index write proof did not resolve")
	}
	changed := flow.ApplyMapWriteProof(&out, proof)

	if !changed {
		t.Fatal("dynamic index write proof did not report key-presence change")
	}
	if !testKeyPresenceHas(t, out.KeyPresence, tablePath, keyPath) {
		t.Fatalf("dynamic index write proof did not seed KeyPresence: %s", out.KeyPresence.Format())
	}
	if len(out.IndexWrites.Entries()) != 0 {
		t.Fatalf("opaque unsealed key seeded heavy readback proof: %s", out.IndexWrites.Format())
	}
}

func TestWriteEffectPublishesStableDynamicIndexProofWhenProductUpdateCannotCarryShape(t *testing.T) {
	self := &ast.IdentExpr{Value: "self"}
	id := &ast.IdentExpr{Value: "node_id"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		self: cfg.SymbolID(440),
		id:   cfg.SymbolID(441),
	})
	tr := New(in, Config{})
	nodesBase := &ast.AttrGetExpr{
		Object:    self,
		Key:       &ast.StringExpr{Value: "nodes"},
		KeySyntax: ast.AttrKeyDot,
	}
	targetExpr := &ast.AttrGetExpr{
		Object:    nodesBase,
		Key:       id,
		KeySyntax: ast.AttrKeyIndex,
	}
	nodeType := typ.NewRecord().Field("node_id", typ.String).Build()
	nodeValue := product.FromType(nodeType)
	tablePath := constraint.NewPath(cfg.SymbolID(440), "self").Field("nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(441), "node_id")
	out := flow.PointState{}

	changed := tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     cfg.SymbolID(440),
			RootName: "self",
			Steps: []PlaceStep{
				{Kind: PlaceStepStaticMember, Member: value.MemberField("nodes")},
				{Kind: PlaceStepDynamicIndex, Key: product.FromType(typ.Any)},
			},
		},
		Value: nodeValue,
		IndexTarget: cfg.AssignTarget{
			Kind: cfg.TargetIndex,
			Base: nodesBase,
			Key:  id,
			Expr: targetExpr,
		},
		DynamicMode:  DynamicWriteForeign,
		RecordStatic: true,
	})

	if !changed {
		t.Fatal("write effect did not report a proof change")
	}
	if !testKeyPresenceHas(t, out.KeyPresence, tablePath, keyPath) {
		t.Fatalf("stable dynamic write did not seed key presence: %s", out.KeyPresence.Format())
	}
	got, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, keyPath, typ.Any)
	if !ok || !product.Domain.Equal(got, nodeValue) {
		t.Fatalf("stable dynamic write readback = %v/%v, want node/true; facts=%s", got.ProjectValue(), ok, out.IndexWrites.Format())
	}
}

func TestUnresolvedDynamicIndexAssignPublishesStablePathProof(t *testing.T) {
	self := &ast.IdentExpr{Value: "self"}
	id := &ast.IdentExpr{Value: "node_id"}
	const (
		selfSym = cfg.SymbolID(446)
		idSym   = cfg.SymbolID(447)
	)
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		self: selfSym,
		id:   idSym,
	})
	tr := New(in, Config{})
	nodesBase := &ast.AttrGetExpr{
		Object:    self,
		Key:       &ast.StringExpr{Value: "nodes"},
		KeySyntax: ast.AttrKeyDot,
	}
	targetExpr := &ast.AttrGetExpr{
		Object:    nodesBase,
		Key:       id,
		KeySyntax: ast.AttrKeyIndex,
	}
	src := &ast.TableExpr{Fields: []*ast.Field{
		{
			Key:       &ast.StringExpr{Value: "node_id"},
			KeySyntax: ast.AttrKeyDot,
			Value:     id,
		},
		{
			Key:       &ast.StringExpr{Value: "status"},
			KeySyntax: ast.AttrKeyDot,
			Value:     &ast.StringExpr{Value: "pending"},
		},
	}}
	out := flow.PointState{}

	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind: cfg.TargetIndex,
		Base: nodesBase,
		Key:  id,
		Expr: targetExpr,
	}, src, nil)

	tablePath := constraint.NewPath(selfSym, "self").Field("nodes")
	keyPath := constraint.NewPath(idSym, "node_id")
	if !testKeyPresenceHas(t, out.KeyPresence, tablePath, keyPath) {
		t.Fatalf("unresolved dynamic assign did not seed key presence: %s", out.KeyPresence.Format())
	}
	got, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, keyPath, typ.Unknown)
	if !ok {
		t.Fatalf("unresolved dynamic assign did not seed readback proof: %s", out.IndexWrites.Format())
	}
	status, ok := product.FieldOf(got, "status")
	if !ok || !typ.TypeEquals(status.ProjectValue(), typ.LiteralString("pending")) {
		t.Fatalf("readback status = %v/%v, want pending/true; value=%v", status.ProjectValue(), ok, got.ProjectValue())
	}
	if _, ok := out.Env[flow.SymbolValueKey(selfSym)]; ok {
		t.Fatalf("unresolved dynamic assign fabricated root product state: %v", out.Env[flow.SymbolValueKey(selfSym)].ProjectValue())
	}
}

func TestTransferPreservesIncomingIndexWriteAdmissionFacts(t *testing.T) {
	const tableSym = cfg.SymbolID(441)
	in := input.BuildFromFunction(&ast.FunctionExpr{ParList: &ast.ParList{}}, nil, nil)
	tr := New(in, Config{})
	incoming := flow.PointState{
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target: flow.SymbolPathKey(tableSym, nil),
			Key:    product.FromType(typ.String),
			Value:  product.FromType(typ.Number),
		}),
	}

	out := tr.Transfer(in.Graph, in.Graph.Entry(), incoming, nil, nil)
	if got, ok := testIndexWriteAdmission(t, out.IndexWrites, constraint.NewPath(tableSym, "items"), constraint.Path{}, typ.String); !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("Transfer dropped durable index-write admission: %v/%v in %s", got.ProjectValue(), ok, out.IndexWrites.Format())
	}
}

func TestBoundaryIndexWriteReplayAdmitsOpaqueStableKeyPath(t *testing.T) {
	self := &ast.IdentExpr{Value: "self"}
	nodeID := &ast.IdentExpr{Value: "node_id"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		self:   cfg.SymbolID(442),
		nodeID: cfg.SymbolID(443),
	})
	tr := New(in, Config{})
	out := flow.PointState{}
	nodeType := typ.NewRecord().
		Field("config", typ.NewRecord().Field("data_targets", typ.NewArray(typ.String)).Build()).
		Build()
	call := &ast.FuncCallExpr{Args: []ast.Expr{self, nodeID}}

	changed := tr.applyBoundaryFacts(&out, call, flow.BoundaryFactsOf(nil, nil, nil, nil, nil, []flow.BoundaryIndexWriteFact{{
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 1,
		},
		Value: product.FromType(nodeType),
	}}), nil)

	if !changed {
		t.Fatal("boundary index-write replay did not report a fact change")
	}
	tablePath := constraint.NewPath(cfg.SymbolID(442), "self").Field("nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(443), "node_id")
	if !testKeyPresenceHas(t, out.KeyPresence, tablePath, keyPath) {
		t.Fatalf("boundary index-write replay did not seed key presence: %s", out.KeyPresence.Format())
	}
	got, ok := testIndexWriteAdmission(t, out.IndexWrites, tablePath, keyPath, typ.Unknown)
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("boundary index-write replay = %v/%v, want node/true; facts=%s", got.ProjectValue(), ok, out.IndexWrites.Format())
	}
}

func TestBoundaryFactBatchPreservesFinalKeyArrayValueProofs(t *testing.T) {
	self := &ast.IdentExpr{Value: "self"}
	nodeID := &ast.IdentExpr{Value: "node_id"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		self:   cfg.SymbolID(444),
		nodeID: cfg.SymbolID(445),
	})
	tr := New(in, Config{})
	out := flow.PointState{}
	nodeType := typ.NewRecord().Field("node_id", typ.String).Build()
	call := &ast.FuncCallExpr{Args: []ast.Expr{self, nodeID}}
	arrayPath := constraint.NewPath(cfg.SymbolID(444), "self").Field("node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(444), "self").Field("nodes")

	changed := tr.applyBoundaryFacts(&out, call, flow.BoundaryFactsOf(nil, nil, []flow.BoundaryKeyArrayValueFact{{
		Array: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "node_order"},
			},
		},
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Value: product.FromType(nodeType),
	}}, nil, nil, []flow.BoundaryIndexWriteFact{{
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 1,
		},
		Value: product.FromType(nodeType),
	}}), nil)

	if !changed {
		t.Fatal("boundary fact batch did not report a change")
	}
	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), nodeType) {
		t.Fatalf("boundary key-array value = %s, want final node_order -> nodes proof", out.KeyPresence.Format())
	}
}

func TestBoundaryAppendKeyBatchDerivesFreshEmptyTableFromSameBatchIndexWrite(t *testing.T) {
	self := &ast.IdentExpr{Value: "self"}
	nodeID := &ast.IdentExpr{Value: "node_id"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		self:   cfg.SymbolID(446),
		nodeID: cfg.SymbolID(447),
	})
	tr := New(in, Config{})
	nodeType := typ.NewRecord().Field("node_id", typ.String).Build()
	call := &ast.FuncCallExpr{Args: []ast.Expr{self, nodeID}}
	arrayPath := constraint.NewPath(cfg.SymbolID(446), "self").Field("node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(446), "self").Field("nodes")
	facts := flow.BoundaryFactsOf(nil, nil, nil, []flow.BoundaryAppendKeyFact{{
		Array: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "node_order"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 1,
		},
	}}, nil, []flow.BoundaryIndexWriteFact{{
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 1,
		},
		Value: product.FromType(nodeType),
	}})
	out := flow.PointState{
		KeyPresence: testKeyPresenceWithEmptyKeyArray(t, flow.KeyPresenceFacts{}, arrayPath),
	}

	if !tr.applyBoundaryFacts(&out, call, facts, nil) {
		t.Fatal("boundary append/index-write batch did not report a change")
	}
	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), nodeType) {
		t.Fatalf("fresh-empty append batch value = %s, want node_order -> nodes proof", out.KeyPresence.Format())
	}

	out = flow.PointState{}
	tr.applyBoundaryFacts(&out, call, facts, nil)
	if values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath)); len(values) != 0 {
		t.Fatalf("non-empty/unknown append batch derived table proof: %s", out.KeyPresence.Format())
	}
	appends := out.KeyPresence.AppendedKeyEntries()
	if len(appends) != 1 ||
		appends[0].Array != flow.StablePathKey(arrayPath) ||
		appends[0].Key != flow.StablePathKey(constraint.NewPath(cfg.SymbolID(447), "node_id")) {
		t.Fatalf("non-empty/unknown append batch dropped direct append event: %s", out.KeyPresence.Format())
	}
}

func TestNestedDynamicElementWritePreservesContainerKeyPresence(t *testing.T) {
	itemsPath := constraint.NewPath(cfg.SymbolID(441), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(442), "k")
	out := flow.PointState{
		KeyPresence: testKeyPresenceWith(t, flow.KeyPresenceFacts{}, itemsPath, keyPath),
	}
	tr := New(input.Inputs{}, Config{})

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     cfg.SymbolID(441),
			RootName: "items",
			Steps: []PlaceStep{
				{Kind: PlaceStepDynamicIndex, Key: product.FromType(typ.Any)},
				{Kind: PlaceStepStaticMember, Member: value.MemberField("name")},
			},
		},
		Value:        product.FromType(typ.String),
		RecordStatic: true,
	})

	if !testKeyPresenceHas(t, out.KeyPresence, itemsPath, keyPath) {
		t.Fatalf("nested dynamic element write dropped container key-presence: %s", out.KeyPresence.Format())
	}
}

func TestBoundaryAppendKeyBatchRebasesReturnKeyForFreshEmptyTable(t *testing.T) {
	self := &ast.IdentExpr{Value: "self"}
	nodeID := &ast.IdentExpr{Value: "node_id"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		self:   cfg.SymbolID(448),
		nodeID: cfg.SymbolID(449),
	})
	tr := New(in, Config{})
	nodeType := typ.NewRecord().Field("node_id", typ.String).Build()
	call := &ast.FuncCallExpr{Args: []ast.Expr{self}}
	arrayPath := constraint.NewPath(cfg.SymbolID(448), "self").Field("node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(448), "self").Field("nodes")
	facts := flow.BoundaryFactsOf(nil, nil, nil, []flow.BoundaryAppendKeyFact{{
		Array: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "node_order"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathReturn,
			Index: 0,
		},
	}}, nil, []flow.BoundaryIndexWriteFact{{
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathReturn,
			Index: 0,
		},
		Value: product.FromType(nodeType),
	}})
	out := flow.PointState{
		KeyPresence: testKeyPresenceWithEmptyKeyArray(t, flow.KeyPresenceFacts{}, arrayPath),
	}

	if !tr.applyBoundaryFacts(&out, call, facts, map[int]constraint.Path{0: constraint.NewPath(cfg.SymbolID(449), "node_id")}) {
		t.Fatal("return-relative boundary append/index-write batch did not report a change")
	}
	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), nodeType) {
		t.Fatalf("return-key append batch value = %s, want node_order -> nodes proof", out.KeyPresence.Format())
	}
}

func TestAssignCallPostconditionDerivesFreshEmptyKeyArrayFromReturnAppend(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"graph"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	graphSym := in.Scope.ParamSymbols[0]
	nodeIDSym := cfg.SymbolID(450)
	graph := &ast.IdentExpr{Value: "graph"}
	in.Graph.Bindings().Bind(graph, graphSym)
	in.Graph.Bindings().SetName(graphSym, "graph")
	call := &ast.FuncCallExpr{
		Receiver: graph,
		Method:   "create_node",
	}
	callInfo := &cfg.CallInfo{
		Call:     call,
		Method:   call.Method,
		Receiver: graph,
	}
	nodeType := typ.NewRecord().Field("node_id", typ.String).Build()
	facts := flow.BoundaryFactsOf(nil, nil, nil, []flow.BoundaryAppendKeyFact{{
		Array: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "node_order"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathReturn,
			Index: 0,
		},
	}}, nil, []flow.BoundaryIndexWriteFact{{
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathReturn,
			Index: 0,
		},
		Value: product.FromType(nodeType),
	}})
	tr := New(in, Config{CallTyper: boundaryAndContainerElementUnionTyper{facts: facts}})
	arrayPath := constraint.NewPath(graphSym, "graph").Field("node_order")
	tablePath := constraint.NewPath(graphSym, "graph").Field("nodes")
	out := flow.PointState{
		KeyPresence: testKeyPresenceWithEmptyKeyArray(t, flow.KeyPresenceFacts{}, arrayPath),
	}
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "node_id",
			Symbol: nodeIDSym,
		}},
		Sources:     []ast.Expr{call},
		SourceCalls: []*cfg.CallInfo{callInfo},
	}

	tr.applyAssign(&out, 0, info, nil)

	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), nodeType) {
		t.Fatalf("assign-call return append value = %s, want graph.node_order -> graph.nodes proof", out.KeyPresence.Format())
	}
}

func TestAssignCallPostconditionPreservesAppendHistoryAcrossReceiverMutation(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"graph"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	graphSym := in.Scope.ParamSymbols[0]
	nodeIDSym := cfg.SymbolID(451)
	graph := &ast.IdentExpr{Value: "graph"}
	in.Graph.Bindings().Bind(graph, graphSym)
	in.Graph.Bindings().SetName(graphSym, "graph")
	call := &ast.FuncCallExpr{
		Receiver: graph,
		Method:   "create_node",
	}
	callInfo := &cfg.CallInfo{
		Call:     call,
		Method:   call.Method,
		Receiver: graph,
	}
	nodeType := typ.NewRecord().Field("node_id", typ.String).Build()
	facts := flow.BoundaryFactsOf(nil, nil, nil, []flow.BoundaryAppendKeyFact{{
		Array: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "node_order"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathReturn,
			Index: 0,
		},
	}}, nil, []flow.BoundaryIndexWriteFact{{
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Key: flow.BoundaryPath{
			Kind:  flow.BoundaryPathReturn,
			Index: 0,
		},
		Value: product.FromType(nodeType),
	}})
	updatedReceiver := product.FromType(typ.NewRecord().
		Field("node_order", typ.NewArray(typ.Unknown)).
		Field("nodes", typ.NewMap(typ.String, nodeType)).
		Build())
	tr := New(in, Config{CallTyper: boundaryAndReceiverEffectTyper{
		facts:   facts,
		effects: flow.ReceiverMustWrite(0, updatedReceiver),
	}})
	arrayPath := constraint.NewPath(graphSym, "graph").Field("node_order")
	tablePath := constraint.NewPath(graphSym, "graph").Field("nodes")
	arrayKey := flow.StablePathKey(arrayPath)
	tableKey := flow.StablePathKey(tablePath)
	out := flow.PointState{
		KeyPresence: testKeyPresenceWithEmptyKeyArray(t, flow.KeyPresenceFacts{}, arrayPath),
	}
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "node_id",
			Symbol: nodeIDSym,
		}},
		Sources:     []ast.Expr{call},
		SourceCalls: []*cfg.CallInfo{callInfo},
	}

	tr.applyAssign(&out, 0, info, nil)

	if !out.KeyPresence.HasAppendHistoryBase(arrayKey) {
		t.Fatalf("receiver mutation erased append-history base: %s", out.KeyPresence.Format())
	}
	values := out.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), nodeType) {
		t.Fatalf("receiver mutation append value = %s, want graph.node_order -> graph.nodes proof", out.KeyPresence.Format())
	}
	coverage := out.KeyPresence.AppendHistoryCoverageEntries()
	if len(coverage) != 1 || coverage[0].Array != arrayKey || coverage[0].Table != tableKey {
		t.Fatalf("receiver mutation did not restore append-history coverage: %s", out.KeyPresence.Format())
	}
}

func TestConditionEffectOwnsConditionAxis(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	x := constraint.NewPath(cfg.SymbolID(451), "x")
	y := constraint.NewPath(cfg.SymbolID(452), "y")
	first := constraint.FromConstraints(constraint.Truthy{Path: x})
	second := constraint.FromConstraints(constraint.NotNil{Path: y})
	out := flow.PointState{}

	if !tr.applyConditionEffect(&out, ConditionEffect{Fact: first}) || !constraint.Domain.Equal(out.Cond, first) {
		t.Fatalf("first condition effect = %v, want %v", out.Cond, first)
	}
	if !tr.applyConditionEffect(&out, ConditionEffect{Fact: second}) {
		t.Fatal("second condition effect did not report a change")
	}
	want := constraint.And(first, second)
	if !constraint.Domain.Equal(out.Cond, want) {
		t.Fatalf("combined condition = %v, want %v", out.Cond, want)
	}
	if tr.applyConditionEffect(&out, ConditionEffect{Fact: constraint.TrueCondition()}) {
		t.Fatal("true condition fact should not mutate Cond")
	}
}

func TestConditionEffectContradictionCollapsesPointState(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	x := constraint.NewPath(cfg.SymbolID(453), "x")
	out := flow.PointState{
		Cond: constraint.FromConstraints(constraint.HasType{
			Path: x,
			Type: typeKeyFor("number"),
		}),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(x.Symbol): product.FromType(typ.Number),
		},
	}

	if !tr.applyConditionEffect(&out, ConditionEffect{Fact: constraint.FromConstraints(constraint.HasType{
		Path: x,
		Type: typeKeyFor("string"),
	})}) {
		t.Fatal("contradictory condition effect did not report a change")
	}
	if !flow.PointStateDomain.Equal(out, flow.PointStateDomain.Bottom()) {
		t.Fatalf("contradictory condition effect = %#v, want point-state bottom", out)
	}
}

func TestWriteEffectInvalidatesStaleConditionFactsForStaticPlace(t *testing.T) {
	graphSym := cfg.SymbolID(461)
	graph := constraint.NewPath(graphSym, "graph")
	lastNode := graph.Field("last_node_id")
	inputData := graph.Field("input_data")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Cond: constraint.FromDisjuncts([][]constraint.Constraint{
			{constraint.Truthy{Path: lastNode}, constraint.Truthy{Path: inputData}},
			{constraint.Truthy{Path: lastNode}, constraint.Falsy{Path: inputData}},
		}),
	}

	changed := tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     graphSym,
			RootName: "graph",
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("last_node_id"),
			}},
		},
		Value:        product.FromType(typ.String),
		References:   sourceReferenceWrite(),
		RecordStatic: true,
	})

	if !changed {
		t.Fatal("write effect did not report condition invalidation")
	}
	if conditionMentionsPath(out.Cond, lastNode) {
		t.Fatalf("stale last_node_id condition survived write: %v", out.Cond)
	}
	if !conditionMentionsPath(out.Cond, inputData) {
		t.Fatalf("sibling input_data condition was incorrectly invalidated: %v", out.Cond)
	}
}

func TestWriteEffectInvalidatesIndexConditionFactsForStaticPlace(t *testing.T) {
	graphSym := cfg.SymbolID(462)
	graph := constraint.NewPath(graphSym, "graph")
	lastNode := graph.IndexStr("last_node_id")
	inputData := graph.Field("input_data")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Cond: constraint.FromDisjuncts([][]constraint.Constraint{
			{
				constraint.IndexEquals{
					Target: graph,
					Key:    typ.LiteralString("last_node_id"),
					Value:  typ.LiteralString("node-1"),
				},
				constraint.Truthy{Path: inputData},
			},
			{
				constraint.IndexEquals{
					Target: graph,
					Key:    typ.LiteralString("last_node_id"),
					Value:  typ.LiteralString("node-1"),
				},
				constraint.Falsy{Path: inputData},
			},
		}),
	}

	changed := tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root:     graphSym,
			RootName: "graph",
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberStringIndex("last_node_id"),
			}},
		},
		Value:        product.FromType(typ.String),
		References:   sourceReferenceWrite(),
		RecordStatic: true,
	})

	if !changed {
		t.Fatal("write effect did not report index-condition invalidation")
	}
	if conditionMentionsPath(out.Cond, lastNode) {
		t.Fatalf("stale last_node_id index condition survived write: %v", out.Cond)
	}
	if !conditionMentionsPath(out.Cond, inputData) {
		t.Fatalf("sibling input_data condition was incorrectly invalidated: %v", out.Cond)
	}
}

func TestMutatorEffectInvalidatesStaleConditionFactsForPlace(t *testing.T) {
	itemsSym := cfg.SymbolID(463)
	items := constraint.NewPath(itemsSym, "items")
	first := items.IndexInt(1)
	other := constraint.NewPath(cfg.SymbolID(464), "other")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(itemsSym): product.FromType(typ.NewArray(typ.String)),
		},
		Cond: constraint.FromDisjuncts([][]constraint.Constraint{
			{constraint.Truthy{Path: first}, constraint.Truthy{Path: other}},
			{constraint.Truthy{Path: first}, constraint.Falsy{Path: other}},
		}),
	}

	changed := tr.applyMutatorEffect(&out, MutatorEffect{
		Place:   Place{Root: itemsSym, RootName: "items"},
		Kind:    MutatorAppendElement,
		Element: product.FromType(typ.Number),
	})

	if !changed {
		t.Fatal("mutator effect did not report a change")
	}
	if conditionMentionsPath(out.Cond, first) {
		t.Fatalf("stale index condition survived mutator: %v", out.Cond)
	}
	if !conditionMentionsPath(out.Cond, other) {
		t.Fatalf("unrelated condition was incorrectly invalidated: %v", out.Cond)
	}
}

func TestSymbolWriteEffectKillsSiblingNilRelation(t *testing.T) {
	valueSym := cfg.SymbolID(501)
	errSym := cfg.SymbolID(502)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Rel: flow.PointRelations{}.WithSiblingNil(errSym, []cfg.SymbolID{valueSym}),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:         Place{Root: valueSym},
		References:    sourceReferenceWrite(),
		KillRelations: true,
		RecordStatic:  true,
	})

	if _, ok := out.Rel.SiblingNil(errSym); ok {
		t.Fatalf("stale sibling-nil relation survived symbol write: %#v", out.Rel)
	}
}

func TestLoopAppendLengthFactSeedsNumericAndPointRelation(t *testing.T) {
	root := cfg.SymbolID(521)
	target, ok := flow.LocalAddressOfPath(constraint.Path{Symbol: root, Version: 1})
	if !ok {
		t.Fatal("local target address was not produced")
	}
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}

	changed := tr.applyLoopAppendLengthFacts(&out, []input.LoopAppendLengthFact{{
		Point:       7,
		TargetLocal: target,
		Count:       4,
		ParamIndex:  1,
	}})

	if !changed {
		t.Fatal("loop append length fact did not report a state change")
	}
	if lower, _, ok := out.Num.LenBoundsFor(target.Key()); !ok || lower != 4 {
		t.Fatalf("length lower bound = %v/%v, want 4", lower, ok)
	}
	if !out.Rel.HasTargetLengthParamLocal(target, 1) {
		t.Fatalf("point relation = %#v, want target length >= param 1", out.Rel)
	}
}

func TestFieldWriteKillsTargetLengthRelationOnly(t *testing.T) {
	root := cfg.SymbolID(531)
	errSym := cfg.SymbolID(532)
	valueSym := cfg.SymbolID(533)
	target, ok := flow.LocalAddressOfPath(constraint.Path{Symbol: root, Version: 1})
	if !ok {
		t.Fatal("local target address was not produced")
	}
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Rel: flow.PointRelations{}.
			WithTargetLengthParamLocal(target, 0).
			WithSiblingNil(errSym, []cfg.SymbolID{valueSym}),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: root,
			Steps: []PlaceStep{{
				Kind: PlaceStepDynamicIndex,
				Key:  product.FromType(typ.String),
			}},
		},
		References: sourceReferenceWrite(),
	})

	if out.Rel.HasTargetLengthParamLocal(target, 0) {
		t.Fatalf("field/index write kept stale target-length relation: %#v", out.Rel)
	}
	if _, ok := out.Rel.SiblingNil(errSym); !ok {
		t.Fatalf("field/index write removed unrelated sibling-nil relation: %#v", out.Rel)
	}
}

func TestReturnEffectOwnsProjectionAxes(t *testing.T) {
	ref := flow.FunctionRef{GraphID: 521, ParentHash: 522}
	sig := typ.Func().Returns(typ.String).Build()
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	rel := flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}
	tr := New(input.Inputs{}, Config{FuncTyper: functionRefTestTyper{sig: sig, ref: ref}})
	out := flow.PointState{}

	changed := tr.applyReturnEffect(&out, ReturnEffect{
		Relations: flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{rel}),
		Slots: []ReturnSlotEffect{
			{Index: 0, Source: fn, Value: product.FromType(sig)},
			{Index: 1, Source: &ast.NumberExpr{Value: "1"}, Value: product.FromType(typ.Number)},
		},
	})

	if !changed {
		t.Fatal("return effect reported no change")
	}
	if !out.ReturnRel.HasErrorReturn(rel) {
		t.Fatalf("return relation = %#v, want %#v", out.ReturnRel.ErrorReturns(), rel)
	}
	if got := out.Env[flow.ReturnSlotValueKey(0)].ProjectValue(); !typ.TypeEquals(got, sig) {
		t.Fatalf("return slot 0 value = %v, want %v", got, sig)
	}
	if got := out.Env[flow.ReturnSlotValueKey(1)].ProjectValue(); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("return slot 1 value = %v, want number", got)
	}
	returnPath := constraint.NewPlaceholder(0)
	refs, ok := flow.FunctionRefAtPath(out.FunctionRefs, returnPath)
	if !ok {
		t.Fatalf("return slot function refs missing: %#v", out.FunctionRefs)
	}
	gotRef, singleton := refs.Singleton()
	if !singleton || gotRef != ref {
		t.Fatalf("return slot function refs = %s, want %v", refs.Format(), ref)
	}
	closures, ok := flow.ClosureRefAtPath(out.ClosureRefs, returnPath)
	if !ok {
		t.Fatalf("return slot closure refs missing: %#v", out.ClosureRefs)
	}
	gotClosure, singleton := closures.Singleton()
	if !singleton || gotClosure.Ref != ref {
		t.Fatalf("return slot closure refs = %s, want %v", closures.Format(), ref)
	}
}

func TestReturnEffectRebasesPathBackedNestedRefsToPlaceholder(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	sourceSym := cfg.SymbolID(701)
	source := &ast.IdentExpr{Value: "database"}
	in.Graph.Bindings().Bind(source, sourceSym)
	in.Graph.Bindings().SetName(sourceSym, "database")

	tr := New(in, Config{})
	fnRef := flow.FunctionRef{GraphID: 702}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 703}, flow.CaptureCellsDomain.Bottom(), nil)
	sourcePath := constraint.NewPath(sourceSym, "database").Field("query")
	out := flow.PointState{
		FunctionRefs: flow.WithFunctionRefPath(nil, sourcePath, flow.FunctionRefSetOf(fnRef)),
		ClosureRefs:  flow.WithClosureRefPath(nil, sourcePath, flow.ClosureRefSetOf(closure)),
	}

	tr.applyReturnEffect(&out, ReturnEffect{
		Slots: []ReturnSlotEffect{{
			Index:  0,
			Source: source,
		}},
	})

	target := constraint.NewPlaceholder(0).Field("query")
	if refs, ok := flow.FunctionRefAtPath(out.FunctionRefs, target); !ok {
		t.Fatalf("return slot nested function refs missing: %#v", out.FunctionRefs)
	} else if got, singleton := refs.Singleton(); !singleton || got != fnRef {
		t.Fatalf("return slot nested function refs = %s, want %v", refs.Format(), fnRef)
	}
	if refs, ok := flow.ClosureRefAtPath(out.ClosureRefs, target); !ok {
		t.Fatalf("return slot nested closure refs missing: %#v", out.ClosureRefs)
	} else if got, singleton := refs.Singleton(); !singleton || got.Ref != closure.Ref {
		t.Fatalf("return slot nested closure refs = %s, want %v", refs.Format(), closure.Ref)
	}
}

func TestReturnEffectClearsStaleReturnSlotValue(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.ReturnSlotValueKey(0): product.FromType(typ.Number),
		},
	}

	tr.applyReturnEffect(&out, ReturnEffect{
		Relations: flow.ReturnRelationsDomain.Top(),
		Slots: []ReturnSlotEffect{{
			Index:  0,
			Source: &ast.IdentExpr{Value: "x"},
		}},
	})

	if _, ok := out.Env[flow.ReturnSlotValueKey(0)]; ok {
		t.Fatalf("stale return slot value survived identifier return: %#v", out.Env)
	}
}

func TestReferenceEffectInstallsRefTreesAtStaticPlace(t *testing.T) {
	sym := cfg.SymbolID(531)
	path := constraint.NewPath(sym, "").Field("make")
	stalePath := path.Field("stale")
	ref := flow.FunctionRef{GraphID: 532}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 533}, flow.CaptureCellsDomain.Bottom(), nil)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		FunctionRefs: flow.WithFunctionRefPath(nil, stalePath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 534})),
		ClosureRefs:  flow.WithClosureRefPath(nil, stalePath, flow.ClosureRefSetOf(flow.ClosureRefOf(flow.FunctionRef{GraphID: 535}, flow.CaptureCellsDomain.Bottom(), nil))),
	}

	changed := tr.applyReferenceEffect(&out, ReferenceEffect{
		Place: Place{
			Root: sym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("make"),
			}},
		},
		References: sourceReferenceWrite().WithFunctionTree(flow.FunctionRefTree{
			Root:    flow.FunctionRefSetOf(ref),
			HasRoot: true,
		}).WithClosureTree(flow.ClosureRefTree{
			Root:    flow.ClosureRefSetOf(closure),
			HasRoot: true,
		}),
	})

	if !changed {
		t.Fatal("reference effect reported no change")
	}
	if _, ok := flow.FunctionRefAtPath(out.FunctionRefs, stalePath); ok {
		t.Fatalf("stale nested function ref survived tree reference write: %#v", out.FunctionRefs)
	}
	if _, ok := flow.ClosureRefAtPath(out.ClosureRefs, stalePath); ok {
		t.Fatalf("stale nested closure ref survived tree reference write: %#v", out.ClosureRefs)
	}
	refs, ok := flow.FunctionRefAtPath(out.FunctionRefs, path)
	if !ok {
		t.Fatalf("function refs missing at static place: %#v", out.FunctionRefs)
	}
	gotRef, singleton := refs.Singleton()
	if !singleton || gotRef != ref {
		t.Fatalf("function refs = %s, want %v", refs.Format(), ref)
	}
	closures, ok := flow.ClosureRefAtPath(out.ClosureRefs, path)
	if !ok {
		t.Fatalf("closure refs missing at static place: %#v", out.ClosureRefs)
	}
	gotClosure, singleton := closures.Singleton()
	if !singleton || gotClosure.Ref != closure.Ref {
		t.Fatalf("closure refs = %s, want %v", closures.Format(), closure.Ref)
	}
}

func TestReferenceEffectDynamicPlaceClearsRootSubtree(t *testing.T) {
	sym := cfg.SymbolID(541)
	fieldPath := constraint.NewPath(sym, "").Field("handler")
	ref := flow.FunctionRef{GraphID: 542}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 543}, flow.CaptureCellsDomain.Bottom(), nil)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		FunctionRefs: flow.WithFunctionRefPath(nil, fieldPath, flow.FunctionRefSetOf(ref)),
		ClosureRefs:  flow.WithClosureRefPath(nil, fieldPath, flow.ClosureRefSetOf(closure)),
	}

	tr.applyReferenceEffect(&out, ReferenceEffect{
		Place: Place{
			Root: sym,
			Steps: []PlaceStep{{
				Kind: PlaceStepDynamicIndex,
				Key:  product.FromType(typ.String),
			}},
		},
		Source:     &ast.NumberExpr{Value: "1"},
		References: sourceReferenceWrite(),
	})

	if _, ok := flow.FunctionRefAtPath(out.FunctionRefs, fieldPath); ok {
		t.Fatalf("dynamic reference write kept stale function ref: %#v", out.FunctionRefs)
	}
	if _, ok := flow.ClosureRefAtPath(out.ClosureRefs, fieldPath); ok {
		t.Fatalf("dynamic reference write kept stale closure ref: %#v", out.ClosureRefs)
	}
}

func TestWriteEffectRecordsPrototypeSelfThroughReducer(t *testing.T) {
	proto := cfg.SymbolID(561)
	selfSym := cfg.SymbolID(562)
	self := product.FromType(typ.NewRecord().Field("name", typ.String).Build())
	tr := New(input.Inputs{}, Config{})
	tr.prototypeReceiverSym = proto
	tr.prototypeSelfSymbol = selfSym
	out := flow.PointState{}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:       Place{Root: selfSym},
		Value:       self,
		RecordProto: true,
	})

	got, ok := out.PrototypeSelf.Value(proto)
	if !ok || !product.Domain.Equal(got, self) {
		t.Fatalf("write effect prototype-self = %v/%v, want %v", got.ProjectValue(), ok, self.ProjectValue())
	}
}

func TestNestedWriteEffectRecordsPrototypeSelfThroughReducer(t *testing.T) {
	proto := cfg.SymbolID(563)
	selfSym := cfg.SymbolID(564)
	tr := New(input.Inputs{}, Config{})
	tr.prototypeReceiverSym = proto
	tr.prototypeSelfSymbol = selfSym
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(selfSym): product.FromType(typ.NewRecord().Build()),
		},
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: selfSym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("name"),
			}},
		},
		Value:       product.FromType(typ.String),
		RecordProto: true,
	})

	got, ok := out.PrototypeSelf.Value(proto)
	if !ok {
		t.Fatalf("nested write effect did not record prototype-self: %s", out.PrototypeSelf.Format())
	}
	member, ok := product.MemberOf(got, value.MemberField("name"))
	if !ok || !typ.TypeEquals(member.ProjectValue(), typ.String) {
		t.Fatalf("prototype-self nested member = %v/%v, want string", member.ProjectValue(), ok)
	}
	effects := out.ReceiverEffects.Entries()
	if len(effects) != 1 ||
		effects[0].Slot != 0 ||
		!effects[0].MustWrite ||
		!product.Domain.Equal(effects[0].Value, got) {
		t.Fatalf("receiver effects = %s, want must-write slot 0 to published self", out.ReceiverEffects.Format())
	}
}

func TestNestedWriteEffectRecordsParameterReceiverEffectWithoutPrototype(t *testing.T) {
	selfSym := cfg.SymbolID(567)
	tr := New(input.Inputs{
		Scope: input.ScopeFacts{
			ParamSymbols: []cfg.SymbolID{selfSym},
		},
	}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(selfSym): product.FromType(typ.NewRecord().Build()),
		},
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: selfSym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("name"),
			}},
		},
		Value:       product.FromType(typ.String),
		RecordProto: true,
	})

	effects := out.ReceiverEffects.Entries()
	if len(effects) != 1 ||
		effects[0].Slot != 0 ||
		!effects[0].MustWrite {
		t.Fatalf("receiver effects = %s, want must-write slot 0", out.ReceiverEffects.Format())
	}
	member, ok := product.MemberOf(effects[0].Value, value.MemberField("name"))
	if !ok || !typ.TypeEquals(member.ProjectValue(), typ.String) {
		t.Fatalf("receiver effect self.name = %v/%v, want string", member.ProjectValue(), ok)
	}
}

func TestMutatorEffectRecordsParameterReceiverMutationWithoutRootValue(t *testing.T) {
	selfSym := cfg.SymbolID(568)
	tr := New(input.Inputs{
		Scope: input.ScopeFacts{
			ParamSymbols: []cfg.SymbolID{selfSym},
		},
	}, Config{})
	out := flow.PointState{}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place: Place{
			Root: selfSym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("items"),
			}},
		},
		Kind:    MutatorAppendElement,
		Element: product.FromType(typ.String),
	})

	effects := out.ReceiverEffects.Entries()
	if len(effects) != 1 ||
		effects[0].Slot != 0 ||
		effects[0].MustWrite ||
		!effects[0].Value.IsZero() ||
		len(effects[0].Mutations) != 1 {
		t.Fatalf("receiver effects = %#v (%s), want mutation-only slot 0", effects, out.ReceiverEffects.Format())
	}
}

func TestMutatorEffectRecordsPrototypeSelfThroughReducer(t *testing.T) {
	proto := cfg.SymbolID(565)
	selfSym := cfg.SymbolID(566)
	tr := New(input.Inputs{}, Config{})
	tr.prototypeReceiverSym = proto
	tr.prototypeSelfSymbol = selfSym
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(selfSym): product.FromType(typ.NewRecord().
				Field("items", typ.NewArray(typ.String)).
				Build()),
		},
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place: Place{
			Root: selfSym,
			Steps: []PlaceStep{{
				Kind:   PlaceStepStaticMember,
				Member: value.MemberField("items"),
			}},
		},
		Kind:    MutatorAppendElement,
		Element: product.FromType(typ.Number),
	})

	got, ok := out.PrototypeSelf.Value(proto)
	if !ok {
		t.Fatalf("mutator effect did not record prototype-self: %s", out.PrototypeSelf.Format())
	}
	items, ok := product.MemberOf(got, value.MemberField("items"))
	if !ok {
		t.Fatalf("published self.items missing: %v", got.ProjectValue())
	}
	elem, ok := querycore.Index(product.NarrowPresent(items).ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.String, elem) || !subtype.IsSubtype(typ.Number, elem) {
		t.Fatalf("published self.items element = %v/%v, want precise string|number supertype", elem, ok)
	}
	effects := out.ReceiverEffects.Entries()
	if len(effects) != 1 ||
		effects[0].Slot != 0 ||
		!effects[0].MustWrite ||
		!product.Domain.Equal(effects[0].Value, got) {
		t.Fatalf("receiver effects = %s, want must-write slot 0 to published mutator self", out.ReceiverEffects.Format())
	}
}

func TestCellEffectReducerUpdatesClosureEnvironment(t *testing.T) {
	calleeSym := cfg.SymbolID(601)
	cellSym := cfg.SymbolID(602)
	path := constraint.NewPath(calleeSym, "fn")
	effects := flow.CaptureMustWrite(cellSym, product.FromType(typ.String))
	closure := flow.ClosureRefOf(
		flow.FunctionRef{GraphID: 603},
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cellSym, Value: product.FromType(typ.Number)}}),
		nil,
	)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		ClosureRefs: flow.WithClosureRefPath(nil, path, flow.ClosureRefSetOf(closure)),
	}

	tr.applyCellEffect(&out, CellEffect{Effects: effects, ClosurePath: path})

	if _, ok := out.Cells.Value(cellSym); ok {
		t.Fatalf("closure cell effect polluted caller cells: %s", out.Cells.Format())
	}
	if !flow.CaptureEffectsDomain.Equal(out.CellEffects, effects) {
		t.Fatalf("recorded cell effects = %s, want %s", out.CellEffects.Format(), effects.Format())
	}
	refs, ok := flow.ClosureRefAtPath(out.ClosureRefs, path)
	if !ok {
		t.Fatalf("closure refs missing after cell effect: %#v", out.ClosureRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton {
		t.Fatalf("closure refs after cell effect = %s, want singleton", refs.Format())
	}
	if av, ok := got.EntryCells().Value(cellSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("closure env after cell effect = %v/%v, want string", av.ProjectValue(), ok)
	}
}

func TestMutatorEffectAppendsElementAndUpdatesSideAxes(t *testing.T) {
	namesSym := cfg.SymbolID(701)
	namesPath := constraint.NewPath(namesSym, "names")
	container := constraint.NewPath(cfg.SymbolID(702), "container")
	arrKey := constraint.PathKey(flow.SymbolValueKey(namesSym))
	arrRef, ok := flow.ContainerRefOfPath(namesPath)
	if !ok {
		t.Fatalf("ContainerRefOfPath(%v) failed", namesPath)
	}
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(namesSym): product.FromType(typ.NewArray(typ.String)),
		},
		Num:         numeric.NewState(),
		KeyPresence: testKeyPresenceWithKeyArray(t, flow.KeyPresenceFacts{}, namesPath, container),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:           Place{Root: namesSym},
		Kind:            MutatorAppendElement,
		Element:         product.FromType(typ.Number),
		LengthRef:       arrRef,
		LengthIncrement: 1,
	})

	if tables := out.KeyPresence.KeyArrayTables(flow.StablePathKey(namesPath)); len(tables) != 0 {
		t.Fatalf("mutator kept stale key-array provenance: %v", tables)
	}
	if lower, _, ok := out.Num.LenBoundsFor(arrKey); !ok || lower != 1 {
		t.Fatalf("length bound = %v/%v, want lower 1", lower, ok)
	}
	elem, ok := querycore.Index(out.Env[flow.SymbolValueKey(namesSym)].ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.String, elem) || !subtype.IsSubtype(typ.Number, elem) {
		t.Fatalf("array element = %v/%v, want precise string|number supertype", elem, ok)
	}
}

func TestMutatorEffectAppendKeySeedsKeyArrayForFreshEmptyArray(t *testing.T) {
	arraySym := cfg.SymbolID(711)
	keySym := cfg.SymbolID(712)
	tableSym := cfg.SymbolID(713)
	arrayPath := constraint.NewPath(arraySym, "node_order")
	keyPath := constraint.NewPath(keySym, "node_id")
	tablePath := constraint.NewPath(tableSym, "nodes")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(arraySym): product.FromType(typ.NewFreshArray()),
			flow.SymbolValueKey(keySym):   product.FromType(typ.String),
		},
		KeyPresence: testKeyPresenceWith(t, flow.KeyPresenceFacts{}, tablePath, keyPath),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:       Place{Root: arraySym},
		Kind:        MutatorAppendElement,
		Element:     product.FromType(typ.String),
		ElementPath: keyPath,
	})

	if tables := out.KeyPresence.KeyArrayTables(flow.StablePathKey(arrayPath)); len(tables) != 1 ||
		tables[0] != flow.StablePathKey(tablePath) {
		t.Fatalf("append of proven key did not seed key-array provenance: %s", out.KeyPresence.Format())
	}
}

func TestMutatorEffectAppendKeySeedsKeyArrayValueForFreshEmptyArray(t *testing.T) {
	arraySym := cfg.SymbolID(716)
	keySym := cfg.SymbolID(717)
	tableSym := cfg.SymbolID(718)
	arrayPath := constraint.NewPath(arraySym, "node_order")
	keyPath := constraint.NewPath(keySym, "node_id")
	tablePath := constraint.NewPath(tableSym, "nodes")
	nodeType := typ.NewRecord().Field("node_id", typ.String).Field("status", typ.String).Build()
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(arraySym): product.FromType(typ.NewFreshArray()),
			flow.SymbolValueKey(keySym):   product.FromType(typ.String),
		},
		KeyPresence: testKeyPresenceWith(t, flow.KeyPresenceFacts{}, tablePath, keyPath),
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target:  flow.StablePathKey(tablePath),
			KeyPath: flow.StablePathKey(keyPath),
			Key:     product.FromType(typ.String),
			Value:   product.FromType(nodeType),
		}),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:       Place{Root: arraySym},
		Kind:        MutatorAppendElement,
		Element:     product.FromType(typ.String),
		ElementPath: keyPath,
	})

	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), nodeType) {
		t.Fatalf("append of proven key did not seed key-array value: %s", out.KeyPresence.Format())
	}
}

func TestDelayedIndexWriteMaterializesPendingAppendKeyArrayValue(t *testing.T) {
	arraySym := cfg.SymbolID(719)
	keySym := cfg.SymbolID(720)
	tableSym := cfg.SymbolID(721)
	arrayPath := constraint.NewPath(arraySym, "node_order")
	keyPath := constraint.NewPath(keySym, "node_id")
	tablePath := constraint.NewPath(tableSym, "edges")
	edgeType := typ.NewRecord().
		Field("targets", typ.NewArray(typ.Unknown)).
		Field("error_targets", typ.NewArray(typ.Unknown)).
		Build()
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(arraySym): product.FromType(typ.NewFreshArray()),
			flow.SymbolValueKey(keySym):   product.FromType(typ.String),
		},
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:       Place{Root: arraySym},
		Kind:        MutatorAppendElement,
		Element:     product.FromType(typ.String),
		ElementPath: keyPath,
	})
	if tables := out.KeyPresence.KeyArrayTables(flow.StablePathKey(arrayPath)); len(tables) != 0 {
		t.Fatalf("pending append materialized before table key proof: %s", out.KeyPresence.Format())
	}

	proof, ok := flow.MapWriteTransactionOfPath(flow.MapWritePathTransaction{
		TablePath:              tablePath,
		KeyPath:                keyPath,
		KeyValue:               product.FromType(typ.Unknown),
		Value:                  product.FromType(edgeType),
		AllowOpaqueKeyReadback: true,
	})
	if !ok {
		t.Fatal("dynamic index write proof did not resolve")
	}
	flow.ApplyMapWriteProof(&out, proof)

	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), edgeType) {
		t.Fatalf("delayed index write did not materialize key-array value: %s", out.KeyPresence.Format())
	}
}

func TestMutatorEffectAppendKeyDoesNotSeedKeyArrayForUnknownExistingArray(t *testing.T) {
	arraySym := cfg.SymbolID(722)
	keySym := cfg.SymbolID(723)
	tableSym := cfg.SymbolID(724)
	arrayPath := constraint.NewPath(arraySym, "node_order")
	keyPath := constraint.NewPath(keySym, "node_id")
	tablePath := constraint.NewPath(tableSym, "nodes")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(arraySym): product.FromType(typ.NewArray(typ.String)),
			flow.SymbolValueKey(keySym):   product.FromType(typ.String),
		},
		KeyPresence: testKeyPresenceWith(t, flow.KeyPresenceFacts{}, tablePath, keyPath),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:       Place{Root: arraySym},
		Kind:        MutatorAppendElement,
		Element:     product.FromType(typ.String),
		ElementPath: keyPath,
	})

	if tables := out.KeyPresence.KeyArrayTables(flow.StablePathKey(arrayPath)); len(tables) != 0 {
		t.Fatalf("append into existing array seeded unsound key-array provenance: %s", out.KeyPresence.Format())
	}
}

func TestMutatorEffectAppendsMapElementAtNestedPlace(t *testing.T) {
	rootSym := cfg.SymbolID(801)
	keyAV := product.FromType(typ.String)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.NewMap(typ.String, typ.NewMap(typ.String, typ.NewArray(typ.String)))),
	}}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place: Place{
			Root: rootSym,
			Steps: []PlaceStep{{
				Kind: PlaceStepDynamicIndex,
				Key:  keyAV,
			}},
		},
		Kind:    MutatorAppendMapElement,
		Key:     keyAV,
		Element: product.FromType(typ.Boolean),
	})

	outer, ok := product.IndexOf(out.Env[flow.SymbolValueKey(rootSym)], keyAV)
	if !ok {
		t.Fatalf("outer map read did not resolve: %v", out.Env[flow.SymbolValueKey(rootSym)].ProjectValue())
	}
	arr, ok := product.IndexOf(product.NarrowPresent(outer), keyAV)
	if !ok {
		t.Fatalf("inner map read did not resolve: %v", product.NarrowPresent(outer).ProjectValue())
	}
	elem, ok := querycore.Index(product.NarrowPresent(arr).ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.String, elem) || !subtype.IsSubtype(typ.Boolean, elem) {
		t.Fatalf("nested map array element = %v/%v, want precise string|boolean supertype", elem, ok)
	}
}

func TestTableInsertCallLowersToAppendElementAndLength(t *testing.T) {
	tr, arr, msg, arrSym, msgSym := tableInsertTransferFixture(t, "arr", "msg")
	arrKey := constraint.PathKey(flow.SymbolValueKey(arrSym))
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(arrSym): product.FromType(typ.NewArray(typ.String)),
			flow.SymbolValueKey(msgSym): product.FromType(typ.Number),
		},
		Num: numeric.NewState(),
	}

	tr.applyTableInsert(&out, tableInsertCallInfo(&ast.FuncCallExpr{Args: []ast.Expr{arr, msg}}), nil)

	elem, ok := querycore.Index(out.Env[flow.SymbolValueKey(arrSym)].ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.String, elem) || !subtype.IsSubtype(typ.Number, elem) {
		t.Fatalf("table.insert element = %v/%v, want precise string|number supertype", elem, ok)
	}
	if lower, _, ok := out.Num.LenBoundsFor(arrKey); !ok || lower != 1 {
		t.Fatalf("table.insert length lower = %v/%v, want 1", lower, ok)
	}
}

func TestTableInsertCallDemandsSequenceElementContract(t *testing.T) {
	tr, arr, msg, _, msgSym := tableInsertTransferFixture(t, "arr", "msg")
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(msgSym): product.FromType(typ.Number),
		},
		Num: numeric.NewState(),
	}

	got := collectDemand(t, func(demand func(idx int, c paramevidence.ParamContract)) {
		tr.applyTableInsert(&out, tableInsertCallInfo(&ast.FuncCallExpr{Args: []ast.Expr{arr, msg}}), demand)
	})
	assertDemandType(t, got, 0, typ.NewArray(typ.Number))
}

func TestTableInsertCallLowersDynamicTargetToAppendMapElement(t *testing.T) {
	tr, groups, msg, groupsSym, msgSym := tableInsertTransferFixture(t, "groups", "msg")
	key := &ast.IdentExpr{Value: "suite"}
	keySym := cfg.SymbolID(904)
	tr.in.Graph.Bindings().Bind(key, keySym)
	tr.in.Graph.Bindings().SetName(keySym, "suite")
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(groupsSym): product.FromType(typ.NewMap(typ.String, typ.NewArray(typ.Number))),
			flow.SymbolValueKey(keySym):    product.FromType(typ.String),
			flow.SymbolValueKey(msgSym):    product.FromType(typ.Boolean),
		},
		Num: numeric.NewState(),
	}
	target := &ast.AttrGetExpr{Object: groups, Key: key, KeySyntax: ast.AttrKeyIndex}

	tr.applyTableInsert(&out, tableInsertCallInfo(&ast.FuncCallExpr{Args: []ast.Expr{target, msg}}), nil)

	arr, ok := product.IndexOf(out.Env[flow.SymbolValueKey(groupsSym)], product.FromType(typ.String))
	if !ok {
		t.Fatalf("table.insert dynamic target did not resolve map value: %v", out.Env[flow.SymbolValueKey(groupsSym)].ProjectValue())
	}
	elem, ok := querycore.Index(product.NarrowPresent(arr).ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.Number, elem) || !subtype.IsSubtype(typ.Boolean, elem) {
		t.Fatalf("table.insert dynamic element = %v/%v, want precise number|boolean supertype", elem, ok)
	}
	if out.Num != nil && !out.Num.IsTop() {
		t.Fatalf("dynamic table.insert target changed sequence numeric state: %v", out.Num)
	}
}

func TestMutatorEffectAppendsElementOnCapturedCell(t *testing.T) {
	tr, _, rootSym := captureCellTestTransfer(t)
	out := flow.PointState{
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: rootSym,
			Value:  product.FromType(typ.NewArray(typ.String)),
		}}),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:   Place{Root: rootSym},
		Kind:    MutatorAppendElement,
		Element: product.FromType(typ.Number),
	})

	got, ok := out.Cells.Value(rootSym)
	if !ok {
		t.Fatal("captured append did not write cell")
	}
	elem, ok := querycore.Index(got.ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.String, elem) || !subtype.IsSubtype(typ.Number, elem) {
		t.Fatalf("captured array element = %v/%v, want precise string|number supertype", elem, ok)
	}
	if _, ok := out.Env[flow.SymbolValueKey(rootSym)]; ok {
		t.Fatalf("captured append wrote Env[%s], want only Cells", flow.SymbolValueKey(rootSym))
	}
	assertOneCellEffect(t, out.CellEffects, rootSym, got)
}

func TestMutatorEffectAppendsMapElementOnCapturedCell(t *testing.T) {
	tr, _, rootSym := captureCellTestTransfer(t)
	keyAV := product.FromType(typ.String)
	out := flow.PointState{
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: rootSym,
			Value:  product.FromType(typ.NewMap(typ.String, typ.NewArray(typ.Number))),
		}}),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:   Place{Root: rootSym},
		Kind:    MutatorAppendMapElement,
		Key:     keyAV,
		Element: product.FromType(typ.Boolean),
	})

	got, ok := out.Cells.Value(rootSym)
	if !ok {
		t.Fatal("captured map append did not write cell")
	}
	arr, ok := product.IndexOf(got, keyAV)
	if !ok {
		t.Fatalf("captured map read did not resolve: %v", got.ProjectValue())
	}
	elem, ok := querycore.Index(product.NarrowPresent(arr).ProjectValue(), typ.LiteralInt(1))
	if !ok || typ.IsAny(elem) || !subtype.IsSubtype(typ.Number, elem) || !subtype.IsSubtype(typ.Boolean, elem) {
		t.Fatalf("captured map-array element = %v/%v, want precise number|boolean supertype", elem, ok)
	}
	if _, ok := out.Env[flow.SymbolValueKey(rootSym)]; ok {
		t.Fatalf("captured map append wrote Env[%s], want only Cells", flow.SymbolValueKey(rootSym))
	}
	assertOneCellEffect(t, out.CellEffects, rootSym, got)
}

func TestDynamicIndexWriteUpdatesNestedCapturedCell(t *testing.T) {
	tr, _, rootSym := captureCellTestTransfer(t)
	keyAV := product.FromType(typ.String)
	initial := typ.NewRecord().
		Field("by_id", typ.NewMap(typ.String, typ.NewRecord().Field("name", typ.Number).Build())).
		Build()
	out := flow.PointState{
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: rootSym,
			Value:  product.FromType(initial),
		}}),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place: Place{
			Root: rootSym,
			Steps: []PlaceStep{
				{Kind: PlaceStepStaticMember, Member: value.MemberField("by_id")},
				{Kind: PlaceStepDynamicIndex, Key: keyAV},
				{Kind: PlaceStepStaticMember, Member: value.MemberField("name")},
			},
		},
		Value:        product.FromType(typ.String),
		DynamicMode:  DynamicWriteSelfDerived,
		RecordStatic: true,
	})

	got, ok := out.Cells.Value(rootSym)
	if !ok {
		t.Fatal("captured nested write did not write cell")
	}
	byID, ok := product.FieldOf(got, "by_id")
	if !ok {
		t.Fatalf("captured by_id field missing: %v", got.ProjectValue())
	}
	row, ok := product.IndexOf(product.NarrowPresent(byID), keyAV)
	if !ok {
		t.Fatalf("captured by_id index missing: %v", product.NarrowPresent(byID).ProjectValue())
	}
	name, ok := product.FieldOf(product.NarrowPresent(row), "name")
	if !ok ||
		typ.IsAny(name.ProjectValue()) ||
		!subtype.IsSubtype(typ.Number, name.ProjectValue()) ||
		!subtype.IsSubtype(typ.String, name.ProjectValue()) {
		t.Fatalf("captured nested name = %v/%v, want precise number|string supertype", name.ProjectValue(), ok)
	}
	if _, ok := out.Env[flow.SymbolValueKey(rootSym)]; ok {
		t.Fatalf("captured nested write wrote Env[%s], want only Cells", flow.SymbolValueKey(rootSym))
	}
	assertOneCellEffect(t, out.CellEffects, rootSym, got)
}

func TestMutatorEffectWidensGenericContainerElement(t *testing.T) {
	rootSym := cfg.SymbolID(901)
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.Instantiate(channel, typ.Unknown)),
	}}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:   Place{Root: rootSym},
		Kind:    MutatorContainerElementUnion,
		Element: product.FromType(typ.String),
	})

	want := typ.Instantiate(channel, typ.String)
	got := out.Env[flow.SymbolValueKey(rootSym)].ProjectValue()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("container after mutator = %v, want %v", got, want)
	}
}

func TestStatementCallAppliesContainerElementUnionEffectToCapturedCell(t *testing.T) {
	tr, ch, chSym := captureCellTestTransfer(t)
	msgSym := cfg.SymbolID(902)
	msg := &ast.IdentExpr{Value: "msg"}
	tr.in.Graph.Bindings().Bind(msg, msgSym)
	tr.in.Graph.Bindings().SetName(msgSym, "msg")
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	tr.callTyper = containerElementUnionTyper{effects: []effect.ContainerElementUnion{{
		Container: effect.ParamRef{Index: 0},
		Value:     effect.ParamRef{Index: 1},
	}}}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(msgSym): product.FromType(typ.String),
		},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: chSym,
			Value:  product.FromType(typ.Instantiate(channel, typ.Unknown)),
		}}),
	}

	dead := tr.applyCallArgs(&out, 0, &cfg.CallInfo{
		Call:   &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "send"}, Args: []ast.Expr{ch, msg}},
		Callee: &ast.IdentExpr{Value: "send"},
		Args:   []ast.Expr{ch, msg},
	}, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}

	want := typ.Instantiate(channel, typ.String)
	got, ok := out.Cells.Value(chSym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("captured container after call = %v/%v, want %v", got.ProjectValue(), ok, want)
	}
	if _, ok := out.Env[flow.SymbolValueKey(chSym)]; ok {
		t.Fatalf("captured call mutator wrote Env[%s], want only Cells", flow.SymbolValueKey(chSym))
	}
	assertOneCellEffect(t, out.CellEffects, chSym, got)
}

func TestMutatorEffectOnCapturedCellEmitsCellEffect(t *testing.T) {
	tr, _, rootSym := captureCellTestTransfer(t)
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	out := flow.PointState{
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: rootSym,
			Value:  product.FromType(typ.Instantiate(channel, typ.Unknown)),
		}}),
	}

	tr.applyMutatorEffect(&out, MutatorEffect{
		Place:   Place{Root: rootSym},
		Kind:    MutatorContainerElementUnion,
		Element: product.FromType(typ.String),
	})

	want := typ.Instantiate(channel, typ.String)
	got, ok := out.Cells.Value(rootSym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("captured cell after mutator = %v/%v, want %v", got.ProjectValue(), ok, want)
	}
	if _, ok := out.Env[flow.SymbolValueKey(rootSym)]; ok {
		t.Fatalf("captured-cell mutator wrote Env[%s], want only Cells", flow.SymbolValueKey(rootSym))
	}
	effects := out.CellEffects.Entries()
	if len(effects) != 1 ||
		effects[0].Symbol != rootSym ||
		!effects[0].MustWrite ||
		!typ.TypeEquals(effects[0].Value.ProjectValue(), want) {
		t.Fatalf("captured-cell mutator effects = %s, want must-write %d:%v", out.CellEffects.Format(), rootSym, want)
	}
}

func assertOneCellEffect(t *testing.T, effects flow.CaptureEffects, sym cfg.SymbolID, want product.AbstractValue) {
	t.Helper()
	entries := effects.Entries()
	if len(entries) != 1 ||
		entries[0].Symbol != sym ||
		!entries[0].MustWrite ||
		!product.Domain.Equal(entries[0].Value, want) {
		t.Fatalf("cell effects = %s, want one must-write %d:%v", effects.Format(), sym, want.ProjectValue())
	}
}

func TestStatementCallAppliesContainerElementUnionEffect(t *testing.T) {
	tr, out, callInfo, channel, chSym := containerElementUnionCallFixture(false)

	dead := tr.applyCallArgs(&out, 0, callInfo, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}

	want := typ.Instantiate(channel, typ.String)
	got := out.Env[flow.SymbolValueKey(chSym)].ProjectValue()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("container after call = %v, want %v", got, want)
	}
}

func TestCallBoundaryFactsReplayAfterGenericMutatorEffects(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"graph", "msg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	graphSym := in.Scope.ParamSymbols[0]
	msgSym := in.Scope.ParamSymbols[1]
	graph := &ast.IdentExpr{Value: "graph"}
	msg := &ast.IdentExpr{Value: "msg"}
	in.Graph.Bindings().Bind(graph, graphSym)
	in.Graph.Bindings().Bind(msg, msgSym)
	in.Graph.Bindings().SetName(graphSym, "graph")
	in.Graph.Bindings().SetName(msgSym, "msg")

	facts := flow.BoundaryFactsOf(nil, nil, []flow.BoundaryKeyArrayValueFact{{
		Array: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "node_order"},
			},
		},
		Table: flow.BoundaryPath{
			Kind:  flow.BoundaryPathParam,
			Index: 0,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "nodes"},
			},
		},
		Value: product.FromType(typ.String),
	}}, nil, nil, nil)
	typer := boundaryAndContainerElementUnionTyper{
		facts: facts,
		effects: []effect.ContainerElementUnion{{
			Container: effect.ParamRef{Index: 0},
			Value:     effect.ParamRef{Index: 1},
		}},
	}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(graphSym): product.FromType(typ.NewRecord().Build()),
		flow.SymbolValueKey(msgSym):   product.FromType(typ.String),
	}}
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "mutate"}, Args: []ast.Expr{graph, msg}}

	dead := tr.applyCallArgs(&out, 0, &cfg.CallInfo{
		Call:   call,
		Callee: call.Func,
		Args:   call.Args,
	}, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}

	arrayPath := constraint.NewPath(graphSym, "graph").Field("node_order")
	tablePath := constraint.NewPath(graphSym, "graph").Field("nodes")
	values := out.KeyPresence.KeyArrayValues(flow.StablePathKey(arrayPath), flow.StablePathKey(tablePath))
	if len(values) != 1 || !typ.TypeEquals(values[0].ProjectValue(), typ.String) {
		t.Fatalf("boundary key-array value after mutator = %s, want graph.node_order -> graph.nodes string", out.KeyPresence.Format())
	}
}

func TestMethodCallAppliesContainerElementUnionEffectToReceiver(t *testing.T) {
	tr, out, callInfo, channel, chSym := containerElementUnionCallFixture(true)

	dead := tr.applyCallArgs(&out, 0, callInfo, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}

	want := typ.Instantiate(channel, typ.String)
	got := out.Env[flow.SymbolValueKey(chSym)].ProjectValue()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("receiver container after method call = %v, want %v", got, want)
	}
}

func TestMethodCallAppliesReceiverEffectToReceiverPlace(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"self"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	selfSym := in.Scope.ParamSymbols[0]
	self := &ast.IdentExpr{Value: "self"}
	in.Graph.Bindings().Bind(self, selfSym)
	in.Graph.Bindings().SetName(selfSym, "self")

	updated := product.FromType(typ.NewRecord().Field("nodes", typ.NewMap(typ.String, typ.String)).Build())
	tr := New(in, Config{CallTyper: receiverEffectTyper{effects: flow.ReceiverMustWrite(0, updated)}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(selfSym): product.FromType(typ.NewRecord().Field("nodes", typ.NewRecord().Build()).Build()),
	}}
	call := &ast.FuncCallExpr{Receiver: self, Method: "load_state"}
	info := &cfg.CallInfo{Call: call, Method: "load_state", Receiver: self}

	dead := tr.applyCallArgs(&out, 0, info, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}
	got := out.Env[flow.SymbolValueKey(selfSym)]
	if !product.Domain.Equal(got, updated) {
		t.Fatalf("receiver after method effect = %v, want %v", got.ProjectValue(), updated.ProjectValue())
	}
}

func TestExpressionMethodCallAppliesReceiverEffectToReceiverPlace(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"self"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	selfSym := in.Scope.ParamSymbols[0]
	self := &ast.IdentExpr{Value: "self"}
	in.Graph.Bindings().Bind(self, selfSym)
	in.Graph.Bindings().SetName(selfSym, "self")

	updated := product.FromType(typ.NewRecord().Field("nodes", typ.NewMap(typ.String, typ.String)).Build())
	tr := New(in, Config{CallTyper: receiverEffectTyper{effects: flow.ReceiverMustWrite(0, updated)}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(selfSym): product.FromType(typ.NewRecord().Field("nodes", typ.NewRecord().Build()).Build()),
	}}
	call := &ast.FuncCallExpr{Receiver: self, Method: "load_state"}

	if returns, ok := tr.evalCall(&out, call, nil); !ok || len(returns) == 0 {
		t.Fatalf("expression call returns = (%v, %v), want typed call", returns, ok)
	}
	got := out.Env[flow.SymbolValueKey(selfSym)]
	if !product.Domain.Equal(got, updated) {
		t.Fatalf("receiver after expression method effect = %v, want %v", got.ProjectValue(), updated.ProjectValue())
	}
}

func TestExpressionCallAppliesContainerElementUnionEffect(t *testing.T) {
	for _, method := range []bool{false, true} {
		name := "function"
		if method {
			name = "method"
		}
		t.Run(name, func(t *testing.T) {
			tr, out, callInfo, channel, chSym := containerElementUnionCallFixture(method)

			if returns, ok := tr.evalCall(&out, callInfo.Call, nil); !ok || len(returns) == 0 {
				t.Fatalf("expression call returns = (%v, %v), want typed call", returns, ok)
			}

			want := typ.Instantiate(channel, typ.String)
			got := out.Env[flow.SymbolValueKey(chSym)].ProjectValue()
			if !typ.TypeEquals(got, want) {
				t.Fatalf("container after expression %s call = %v, want %v", name, got, want)
			}
		})
	}
}

func conditionMentionsPath(cond constraint.Condition, path constraint.Path) bool {
	for i := 0; i < cond.NumDisjuncts(); i++ {
		for _, c := range cond.DisjunctConstraints(i) {
			for _, p := range constraint.SemanticAffectedPaths(c) {
				if p.Equal(path) {
					return true
				}
			}
		}
	}
	return false
}

func containerElementUnionCallFixture(method bool) (*Transfer, flow.PointState, *cfg.CallInfo, *typ.Generic, cfg.SymbolID) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"ch", "msg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	chSym := in.Scope.ParamSymbols[0]
	msgSym := in.Scope.ParamSymbols[1]
	ch := &ast.IdentExpr{Value: "ch"}
	msg := &ast.IdentExpr{Value: "msg"}
	in.Graph.Bindings().Bind(ch, chSym)
	in.Graph.Bindings().Bind(msg, msgSym)

	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(chSym):  product.FromType(typ.Instantiate(channel, typ.Unknown)),
		flow.SymbolValueKey(msgSym): product.FromType(typ.String),
	}}
	typer := containerElementUnionTyper{effects: []effect.ContainerElementUnion{{
		Container: effect.ParamRef{Index: 0},
		Value:     effect.ParamRef{Index: 1},
	}}}
	tr := New(in, Config{CallTyper: typer})

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "send"}, Args: []ast.Expr{ch, msg}}
	info := &cfg.CallInfo{Call: call, Callee: call.Func, Args: call.Args}
	if method {
		call = &ast.FuncCallExpr{Receiver: ch, Method: "send", Args: []ast.Expr{msg}}
		info = &cfg.CallInfo{Call: call, Method: "send", Receiver: ch, Args: call.Args}
	}
	return tr, out, info, channel, chSym
}

func tableInsertTransferFixture(t *testing.T, containerName, valueName string) (*Transfer, *ast.IdentExpr, *ast.IdentExpr, cfg.SymbolID, cfg.SymbolID) {
	t.Helper()
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{containerName, valueName}}}
	in := input.BuildFromFunction(fn, nil, nil)
	containerSym := in.Scope.ParamSymbols[0]
	valueSym := in.Scope.ParamSymbols[1]
	container := &ast.IdentExpr{Value: containerName}
	value := &ast.IdentExpr{Value: valueName}
	in.Graph.Bindings().Bind(container, containerSym)
	in.Graph.Bindings().Bind(value, valueSym)
	in.Graph.Bindings().SetName(containerSym, containerName)
	in.Graph.Bindings().SetName(valueSym, valueName)
	return New(in, Config{}), container, value, containerSym, valueSym
}

func tableInsertCallInfo(call *ast.FuncCallExpr) *cfg.CallInfo {
	call.Func = &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "table"},
		Key:    &ast.IdentExpr{Value: "insert"},
	}
	call.Args = append([]ast.Expr(nil), call.Args...)
	return &cfg.CallInfo{
		Call:       call,
		Callee:     call.Func,
		CalleeName: "insert",
		CalleePath: constraint.Path{
			Root: "table",
			Segments: []constraint.Segment{{
				Kind: constraint.SegmentField,
				Name: "insert",
			}},
		},
		Args: call.Args,
	}
}

type containerElementUnionTyper struct {
	captureEffectTyper
	numberReturnTyper
	effects []effect.ContainerElementUnion
}

func (c containerElementUnionTyper) ProductCallFromValues(*ast.FuncCallExpr, ProductCallContext) ProductCallResult {
	result := productReturnResultForTest(product.FromType(typ.Number))
	result.Effects = CallEffects{
		CellEffects:     c.captureEffectTyper.effects,
		ReceiverEffects: flow.ReceiverEffectsDomain.Bottom(),
		BoundaryFacts:   flow.BoundaryFactsDomain.Top(),
		ElementUnions:   append([]effect.ContainerElementUnion(nil), c.effects...),
	}
	return result
}

type boundaryAndContainerElementUnionTyper struct {
	captureEffectTyper
	facts   flow.BoundaryFacts
	effects []effect.ContainerElementUnion
}

func (c boundaryAndContainerElementUnionTyper) ProductCallFromValues(*ast.FuncCallExpr, ProductCallContext) ProductCallResult {
	result := EmptyProductCallResult()
	result.Effects = CallEffects{
		CellEffects:     c.captureEffectTyper.effects,
		ReceiverEffects: flow.ReceiverEffectsDomain.Bottom(),
		BoundaryFacts:   c.facts,
		ElementUnions:   append([]effect.ContainerElementUnion(nil), c.effects...),
	}
	return result
}

type boundaryAndReceiverEffectTyper struct {
	captureEffectTyper
	facts   flow.BoundaryFacts
	effects flow.ReceiverEffects
}

func (b boundaryAndReceiverEffectTyper) ProductCallFromValues(*ast.FuncCallExpr, ProductCallContext) ProductCallResult {
	result := EmptyProductCallResult()
	result.Effects = CallEffects{
		CellEffects:     b.captureEffectTyper.effects,
		ReceiverEffects: b.effects,
		BoundaryFacts:   b.facts,
	}
	return result
}

type receiverEffectTyper struct {
	captureEffectTyper
	numberReturnTyper
	effects flow.ReceiverEffects
}

func (r receiverEffectTyper) ProductCallFromValues(*ast.FuncCallExpr, ProductCallContext) ProductCallResult {
	result := productReturnResultForTest(product.FromType(typ.Number))
	result.Effects = CallEffects{
		CellEffects:     r.captureEffectTyper.effects,
		ReceiverEffects: r.effects,
		BoundaryFacts:   flow.BoundaryFactsDomain.Top(),
	}
	return result
}
