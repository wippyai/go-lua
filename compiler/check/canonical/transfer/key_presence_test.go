package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/iteration"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSeedKeyedIterKeyOfWritesKeyPresenceAxis(t *testing.T) {
	source := &ast.IdentExpr{Value: "items"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{source: cfg.SymbolID(11)})
	typer := keyPresenceTestTyper{source: source}
	tr := New(in, Config{CallTyper: typer})

	keySym := cfg.SymbolID(12)
	out := flow.PointState{}
	tr.seedKeyedIterKeyOf(&out, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "k",
			Symbol: keySym,
		}},
	}, &ast.FuncCallExpr{})

	tablePath := constraint.NewPath(cfg.SymbolID(11), "items")
	keyPath := constraint.NewPath(keySym, "k")
	if !out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("KeyPresence missing seeded keyed-iteration fact: %s", out.KeyPresence.Format())
	}
	valuePath := constraint.NewPath(cfg.SymbolID(13), "v")
	if out.KeyPresence.HasValuePaths(tablePath, keyPath, valuePath) {
		t.Fatal("unexpected value-origin fact without value target")
	}
	if out.Cond.HasConstraints() {
		t.Fatalf("KeyPresence seeding polluted Cond: %v", out.Cond)
	}
}

func TestSeedKeyedIterKeyOfWritesValueOriginFact(t *testing.T) {
	source := &ast.IdentExpr{Value: "items"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{source: cfg.SymbolID(41)})
	typer := keyPresenceTestTyper{source: source}
	tr := New(in, Config{CallTyper: typer})

	out := flow.PointState{}
	tr.seedKeyedIterKeyOf(&out, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{
			{Kind: cfg.TargetIdent, Name: "k", Symbol: cfg.SymbolID(42)},
			{Kind: cfg.TargetIdent, Name: "v", Symbol: cfg.SymbolID(43)},
		},
	}, &ast.FuncCallExpr{})

	tablePath := constraint.NewPath(cfg.SymbolID(41), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(42), "k")
	valuePath := constraint.NewPath(cfg.SymbolID(43), "v")
	if !out.KeyPresence.HasValuePaths(tablePath, keyPath, valuePath) {
		t.Fatalf("KeyPresence missing value-origin fact: %s", out.KeyPresence.Format())
	}
}

func TestRefineByKeyPresenceDoesNotScanCond(t *testing.T) {
	table := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(21),
		key:   cfg.SymbolID(22),
	})
	tr := New(in, Config{})

	tablePath := constraint.NewPath(cfg.SymbolID(21), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(22), "k")
	read := &ast.AttrGetExpr{Object: table, Key: key, KeySyntax: ast.AttrKeyIndex}
	result := typ.NewOptional(typ.String)

	out := flow.PointState{
		Cond: constraint.FromConstraints(constraint.KeyOf{Table: tablePath, Key: keyPath}),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(cfg.SymbolID(22)): product.FromType(typ.String),
		},
	}
	if got, ok := tr.refineByKeyPresence(&out, read, result); ok {
		t.Fatalf("refined from Cond without product proof: %v", got.ProjectValue())
	}

	out.KeyPresence = out.KeyPresence.WithPaths(tablePath, keyPath)
	got, ok := tr.refineByKeyPresence(&out, read, result)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("refineByKeyPresence = %v, %v; want string,true", got.ProjectValue(), ok)
	}
}

func TestGuardedDynamicIndexSeedsKeyPresenceAxis(t *testing.T) {
	table := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(221),
		key:   cfg.SymbolID(222),
	})
	tr := New(in, Config{})

	read := &ast.AttrGetExpr{Object: table, Key: key, KeySyntax: ast.AttrKeyIndex}
	info := &cfg.BranchInfo{
		Condition: read,
		CondCheck: cfg.CondCheck{Kind: cfg.CheckTruthy},
	}
	out := tr.narrowByCondCheck(flow.PointState{}, info, true, false)

	tablePath := constraint.NewPath(cfg.SymbolID(221), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(222), "k")
	if !out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("truthy dynamic-index guard did not seed KeyPresence: %s", out.KeyPresence.Format())
	}

	falsy := tr.narrowByCondCheck(flow.PointState{}, info, false, false)
	if falsy.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("false edge of truthy dynamic-index guard seeded KeyPresence: %s", falsy.KeyPresence.Format())
	}
}

func TestGuardedDynamicIndexNilComparisonSeedsOnlyNotNilEdge(t *testing.T) {
	table := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(231),
		key:   cfg.SymbolID(232),
	})
	tr := New(in, Config{})

	read := &ast.AttrGetExpr{Object: table, Key: key, KeySyntax: ast.AttrKeyIndex}
	info := &cfg.BranchInfo{
		Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: read, Rhs: &ast.NilExpr{}},
		CondCheck: cfg.CondCheck{Kind: cfg.CheckNotNil},
	}
	out := tr.narrowByCondCheck(flow.PointState{}, info, true, false)

	tablePath := constraint.NewPath(cfg.SymbolID(231), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(232), "k")
	if !out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("not-nil dynamic-index guard did not seed KeyPresence: %s", out.KeyPresence.Format())
	}

	nilEdge := tr.narrowByCondCheck(flow.PointState{}, info, false, false)
	if nilEdge.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("nil edge of dynamic-index guard seeded KeyPresence: %s", nilEdge.KeyPresence.Format())
	}
}

func TestGuardedNegatedDynamicIndexSeedsSurvivingTruthyEdge(t *testing.T) {
	table := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(241),
		key:   cfg.SymbolID(242),
	})
	tr := New(in, Config{})

	read := &ast.AttrGetExpr{Object: table, Key: key, KeySyntax: ast.AttrKeyIndex}
	info := &cfg.BranchInfo{
		Condition: &ast.UnaryNotOpExpr{Expr: read},
		CondCheck: cfg.CondCheck{Kind: cfg.CheckFalsy},
	}

	tablePath := constraint.NewPath(cfg.SymbolID(241), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(242), "k")
	nilOrFalseEdge := tr.narrowByCondCheck(flow.PointState{}, info, true, false)
	if nilOrFalseEdge.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("taken edge of negated dynamic-index guard seeded KeyPresence: %s", nilOrFalseEdge.KeyPresence.Format())
	}

	truthyEdge := tr.narrowByCondCheck(flow.PointState{}, info, false, false)
	if !truthyEdge.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("surviving truthy edge of negated dynamic-index guard did not seed KeyPresence: %s", truthyEdge.KeyPresence.Format())
	}
}

func TestDynamicWriteKeyConsumesKeyPresenceAxis(t *testing.T) {
	key := &ast.IdentExpr{Value: "k"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{key: cfg.SymbolID(32)})
	tr := New(in, Config{})

	baseSym := cfg.SymbolID(31)
	basePath := constraint.NewPath(baseSym, "items")
	keyPath := constraint.NewPath(cfg.SymbolID(32), "k")
	target := cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseName:   "items",
		BaseSymbol: baseSym,
		Key:        key,
	}
	base := product.FromType(typ.NewRecord().
		Field("a", typ.Number).
		Field("b", typ.String).
		Build())

	out := flow.PointState{
		Env:  map[flow.ValueKey]product.AbstractValue{},
		Cond: constraint.FromConstraints(constraint.KeyOf{Table: basePath, Key: keyPath}),
	}
	if got := tr.dynamicWriteKey(&out, target, base, nil); !got.IsZero() {
		t.Fatalf("dynamicWriteKey used Cond without product proof: %v", got.ProjectValue())
	}

	out.KeyPresence = out.KeyPresence.WithPaths(basePath, keyPath)
	if got := tr.dynamicWriteKey(&out, target, product.AbstractValue{}, nil); !got.IsZero() {
		t.Fatalf("dynamicWriteKey synthesized key from zero base: %v", got.ProjectValue())
	}
	got := tr.dynamicWriteKey(&out, target, base, nil)
	want := typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b"))
	if got.IsZero() || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("dynamicWriteKey = %v; want %v", got.ProjectValue(), want)
	}

	target.FieldPath = []string{"inner"}
	out.KeyPresence = flow.KeyPresenceFacts{}.WithPaths(basePath.Field("inner"), keyPath)
	got = tr.dynamicWriteKey(&out, target, base, nil)
	if got.IsZero() || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("nested dynamicWriteKey = %v; want %v", got.ProjectValue(), want)
	}
}

func TestDynamicIndexWriteSeedsKeyPresenceForOpaqueKey(t *testing.T) {
	table := &ast.IdentExpr{Value: "nodes"}
	key := &ast.IdentExpr{Value: "node_id"}
	src := &ast.StringExpr{Value: "node"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(141),
		key:   cfg.SymbolID(142),
	})
	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(cfg.SymbolID(141)): product.FromType(typ.NewMap(typ.String, typ.Any)),
			flow.SymbolValueKey(cfg.SymbolID(142)): product.FromType(typ.Any),
		},
	}
	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseName:   "nodes",
		BaseSymbol: cfg.SymbolID(141),
		Key:        key,
	}, src, nil)

	tablePath := constraint.NewPath(cfg.SymbolID(141), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(142), "node_id")
	if !out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("opaque dynamic write did not seed key presence: %s", out.KeyPresence.Format())
	}
}

func TestWriteIsSelfDerivedUsesLiveKeyPresenceValueOrigin(t *testing.T) {
	table := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	value := &ast.IdentExpr{Value: "v"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(51),
		key:   cfg.SymbolID(52),
		value: cfg.SymbolID(53),
	})
	tr := New(in, Config{})
	target := cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseName:   "items",
		BaseSymbol: cfg.SymbolID(51),
		Key:        key,
	}
	tablePath := constraint.NewPath(cfg.SymbolID(51), "items")
	keyPath := constraint.NewPath(cfg.SymbolID(52), "k")
	valuePath := constraint.NewPath(cfg.SymbolID(53), "v")
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithValuePaths(tablePath, keyPath, valuePath),
	}

	if !tr.writeIsSelfDerived(&out, target, value) {
		t.Fatal("value-origin fact did not prove self-derived write")
	}
	out.KeyPresence = out.KeyPresence.KillSubtree(flow.SymbolPathKey(cfg.SymbolID(53), nil))
	if tr.writeIsSelfDerived(&out, target, value) {
		t.Fatal("stale self-derived write survived value reassignment")
	}
}

func TestWriteIsSelfDerivedUsesStaticContainerPath(t *testing.T) {
	table := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		table: cfg.SymbolID(61),
		key:   cfg.SymbolID(62),
	})
	tr := New(in, Config{})
	target := cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseName:   "items",
		BaseSymbol: cfg.SymbolID(61),
		FieldPath:  []string{"inner"},
		Key:        key,
	}
	inner := &ast.AttrGetExpr{
		Object:    table,
		Key:       &ast.StringExpr{Value: "inner"},
		KeySyntax: ast.AttrKeyDot,
	}
	same := &ast.AttrGetExpr{
		Object:    inner,
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}
	other := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object:    table,
			Key:       &ast.StringExpr{Value: "other"},
			KeySyntax: ast.AttrKeyDot,
		},
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}

	if !tr.writeIsSelfDerived(&flow.PointState{}, target, same) {
		t.Fatal("nested base[key] = base[key] should be self-derived by static container path")
	}
	if tr.writeIsSelfDerived(&flow.PointState{}, target, other) {
		t.Fatal("different static container path must not be self-derived")
	}
}

func TestKeyPresenceKillMemberWriteDropsTableRootValueOrigin(t *testing.T) {
	tableSym := cfg.SymbolID(51)
	keySym := cfg.SymbolID(52)
	valueSym := cfg.SymbolID(53)
	tr := New(input.Inputs{}, Config{})
	tablePath := constraint.NewPath(tableSym, "items")
	keyPath := constraint.NewPath(keySym, "k")
	valuePath := constraint.NewPath(valueSym, "v")
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithValuePaths(tablePath, keyPath, valuePath),
	}

	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind:       cfg.TargetField,
		BaseSymbol: tableSym,
		FieldPath:  []string{"x"},
	}, &ast.StringExpr{Value: "next"}, nil)

	if out.KeyPresence.HasPaths(tablePath, keyPath) || out.KeyPresence.HasValuePaths(tablePath, keyPath, valuePath) {
		t.Fatalf("member write kept stale table-root KeyPresence: %s", out.KeyPresence.Format())
	}
}

func TestKeysCollectorAssignmentSeedsLiveIndexedKeyPresence(t *testing.T) {
	namesSym := cfg.SymbolID(91)
	keySym := cfg.SymbolID(92)
	containerPath := constraint.NewPath(cfg.SymbolID(93), "container")
	namesPath := constraint.NewPath(namesSym, "names")
	call := &ast.FuncCallExpr{}
	typer := keyPresenceTestTyper{
		indexedSource: namesPath,
		keysContainer: containerPath,
	}
	tr := New(input.Inputs{}, Config{CallTyper: typer})
	out := flow.PointState{}
	assign := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "names", Symbol: namesSym}},
		Sources: []ast.Expr{call},
		SourceCalls: []*cfg.CallInfo{{
			Call: call,
		}},
	}

	tr.seedKeyArrayForAssignment(&out, assign, 0, assign.Targets[0])
	if tables := out.KeyPresence.KeyArrayTables(flow.KeyPresencePathKey(namesPath)); len(tables) != 1 || tables[0] != flow.KeyPresencePathKey(containerPath) {
		t.Fatalf("keys-collector assignment did not seed key-array fact: %s", out.KeyPresence.Format())
	}

	tr.seedIndexedIterKeyOf(&out, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{
			{Kind: cfg.TargetIdent, Name: "_", Symbol: cfg.SymbolID(94)},
			{Kind: cfg.TargetIdent, Name: "name", Symbol: keySym},
		},
	}, &ast.FuncCallExpr{})
	if !out.KeyPresence.Has(flow.KeyPresencePathKey(containerPath), flow.SymbolPathKey(keySym, nil)) {
		t.Fatalf("indexed iteration did not consume live key-array fact: %s", out.KeyPresence.Format())
	}
}

func TestIndexedKeyArrayIterationRefinesKeyValueFromTableDomain(t *testing.T) {
	namesSym := cfg.SymbolID(111)
	keySym := cfg.SymbolID(112)
	containerSym := cfg.SymbolID(113)
	namesPath := constraint.NewPath(namesSym, "names")
	containerPath := constraint.NewPath(containerSym, "state").Field("suites")
	typer := keyPresenceTestTyper{indexedSource: namesPath}
	tr := New(input.Inputs{}, Config{CallTyper: typer})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(containerSym): product.FromType(typ.NewRecord().
				Field("suites", typ.NewRecord().MapComponent(typ.String, typ.Any).Build()).
				Build()),
		},
		KeyPresence: flow.KeyPresenceFacts{}.WithKeyArrayPaths(namesPath, containerPath),
	}

	tr.seedIndexedIterKeyOf(&out, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{
			{Kind: cfg.TargetIdent, Name: "_", Symbol: cfg.SymbolID(114)},
			{Kind: cfg.TargetIdent, Name: "name", Symbol: keySym},
		},
	}, &ast.FuncCallExpr{})

	got, ok := tr.symbolValue(&out, keySym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("indexed key-array value = %v/%v, want string", got.ProjectValue(), ok)
	}
}

func TestIndexedKeyPresenceKilledByKeysArrayMutation(t *testing.T) {
	namesSym := cfg.SymbolID(101)
	keySym := cfg.SymbolID(102)
	containerPath := constraint.NewPath(cfg.SymbolID(103), "container")
	namesPath := constraint.NewPath(namesSym, "names")
	typer := keyPresenceTestTyper{indexedSource: namesPath}
	tr := New(input.Inputs{}, Config{CallTyper: typer})
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithKeyArrayPaths(namesPath, containerPath),
	}

	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseName:   "names",
		BaseSymbol: namesSym,
		Key:        &ast.NumberExpr{Value: "1"},
	}, &ast.StringExpr{Value: "changed"}, nil)
	tr.seedIndexedIterKeyOf(&out, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{
			{Kind: cfg.TargetIdent, Name: "_", Symbol: cfg.SymbolID(104)},
			{Kind: cfg.TargetIdent, Name: "name", Symbol: keySym},
		},
	}, &ast.FuncCallExpr{})
	if out.KeyPresence.Has(flow.KeyPresencePathKey(containerPath), flow.SymbolPathKey(keySym, nil)) {
		t.Fatalf("indexed iteration consumed stale mutated key-array fact: %s", out.KeyPresence.Format())
	}
}

func TestTableInsertKillsKeyArrayProvenance(t *testing.T) {
	names := &ast.IdentExpr{Value: "names"}
	namesSym := cfg.SymbolID(111)
	containerPath := constraint.NewPath(cfg.SymbolID(112), "container")
	namesPath := constraint.NewPath(namesSym, "names")
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{names: namesSym})
	tr := New(in, Config{})
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithKeyArrayPaths(namesPath, containerPath),
	}

	tr.applyTableInsert(&out, &cfg.CallInfo{
		Call: &ast.FuncCallExpr{
			Args: []ast.Expr{names, &ast.StringExpr{Value: "extra"}},
		},
		CalleeName: "insert",
		CalleePath: constraint.NewPath(0, "table").Field("insert"),
	}, nil)

	if tables := out.KeyPresence.KeyArrayTables(flow.KeyPresencePathKey(namesPath)); len(tables) != 0 {
		t.Fatalf("table.insert kept stale key-array provenance: %s", out.KeyPresence.Format())
	}
}

func TestPresentNestedIndexWritePreservesKeyPresence(t *testing.T) {
	root := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	base := &ast.AttrGetExpr{
		Object:    root,
		Key:       &ast.StringExpr{Value: "byName"},
		KeySyntax: ast.AttrKeyDot,
	}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		root: cfg.SymbolID(61),
		key:  cfg.SymbolID(62),
	})
	tr := New(in, Config{})

	tablePath := constraint.NewPath(cfg.SymbolID(61), "items").Field("byName")
	keyPath := constraint.NewPath(cfg.SymbolID(62), "k")
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithPaths(tablePath, keyPath),
	}
	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind: cfg.TargetIndex,
		Base: base,
		Key:  key,
	}, &ast.StringExpr{Value: "changed"}, nil)
	if !out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("present nested index write dropped KeyPresence: %s", out.KeyPresence.Format())
	}
}

func TestNilNestedIndexWriteKillsKeyPresence(t *testing.T) {
	root := &ast.IdentExpr{Value: "items"}
	key := &ast.IdentExpr{Value: "k"}
	base := &ast.AttrGetExpr{
		Object:    root,
		Key:       &ast.StringExpr{Value: "byName"},
		KeySyntax: ast.AttrKeyDot,
	}
	in := keyPresenceInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		root: cfg.SymbolID(61),
		key:  cfg.SymbolID(62),
	})
	tr := New(in, Config{})

	tablePath := constraint.NewPath(cfg.SymbolID(61), "items").Field("byName")
	keyPath := constraint.NewPath(cfg.SymbolID(62), "k")
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithPaths(tablePath, keyPath),
	}
	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind: cfg.TargetIndex,
		Base: base,
		Key:  key,
	}, &ast.NilExpr{}, nil)
	if out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("nil nested index write kept stale KeyPresence: %s", out.KeyPresence.Format())
	}
}

func TestGenericForRebindingKillsKeyPresenceBeforeIteratorTypingGate(t *testing.T) {
	keySym := cfg.SymbolID(72)
	valueSym := cfg.SymbolID(73)
	tablePath := constraint.NewPath(cfg.SymbolID(71), "items")
	keyPath := constraint.NewPath(keySym, "k")
	valuePath := constraint.NewPath(valueSym, "v")
	staleField := keyPath.Field("stale")
	staleFieldKey := flow.SymbolPathKey(keySym, staleField.Segments)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		KeyPresence:   flow.KeyPresenceFacts{}.WithValuePaths(tablePath, keyPath, valuePath),
		StaticMembers: flow.StaticMemberFacts{}.With(staleFieldKey, product.FromType(typ.String)),
		FunctionRefs:  flow.WithFunctionRef(nil, staleField.Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 74})),
		Rel:           flow.PointRelations{}.WithSiblingNil(valueSym, []cfg.SymbolID{keySym}),
	}

	tr.applyGenericFor(&out, &cfg.AssignInfo{
		IterExprs: []ast.Expr{&ast.IdentExpr{Value: "unknown_iter"}},
		Targets: []cfg.AssignTarget{
			{Kind: cfg.TargetIdent, Name: "k", Symbol: keySym},
			{Kind: cfg.TargetIdent, Name: "v", Symbol: valueSym},
		},
	}, nil)

	if out.KeyPresence.HasPaths(tablePath, keyPath) || out.KeyPresence.HasValuePaths(tablePath, keyPath, valuePath) {
		t.Fatalf("generic-for rebinding kept stale KeyPresence: %s", out.KeyPresence.Format())
	}
	if _, ok := out.StaticMembers.Value(staleFieldKey); ok {
		t.Fatalf("generic-for rebinding kept stale StaticMembers: %s", out.StaticMembers.Format())
	}
	if _, ok := flow.FunctionRefAt(out.FunctionRefs, staleField.Key()); ok {
		t.Fatalf("generic-for rebinding kept stale FunctionRefs: %#v", out.FunctionRefs)
	}
	if _, ok := out.Rel.SiblingNil(valueSym); ok {
		t.Fatalf("generic-for rebinding kept stale relation: %#v", out.Rel)
	}
}

func TestGenericForBodyEdgeRebindsIteratorProvenance(t *testing.T) {
	in := keyPresenceSourceInput(t, `
local item = { count = 1, name = "ready" }
for key, value in pairs(item) do
	item[key] = value
end
`, "pairs")
	tr := New(in, Config{CallTyper: firstArgKeyedIterTyper{}})

	var branch cfg.Point
	in.Graph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		node := in.Graph.Node(p)
		if branch == 0 && node != nil && node.LoopPreheaderSet {
			if assign := in.Graph.Assign(node.LoopPreheader); assign != nil && len(assign.IterExprs) > 0 {
				branch = p
			}
		}
	})
	if branch == 0 {
		t.Fatal("generic-for branch not found")
	}
	itemSym, ok := in.Graph.SymbolAt(branch, "item")
	if !ok || itemSym == 0 {
		t.Fatal("item symbol not found")
	}
	keySym, ok := in.Graph.SymbolAt(branch, "key")
	if !ok || keySym == 0 {
		t.Fatal("key symbol not found")
	}
	valueSym, ok := in.Graph.SymbolAt(branch, "value")
	if !ok || valueSym == 0 {
		t.Fatal("value symbol not found")
	}

	itemPath := constraint.NewPath(itemSym, "item")
	keyPath := constraint.NewPath(keySym, "key")
	valuePath := constraint.NewPath(valueSym, "value")
	stale := flow.KeyPresenceFacts{}.WithValuePaths(itemPath, keyPath, valuePath)
	elem := typ.NewRecord().
		Field("count", typ.Number).
		Field("name", typ.String).
		Build()
	out := flow.PointState{
		KeyPresence: stale.KillAffectedByWrite(flow.KeyPresencePathKey(itemPath)),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(itemSym):  product.FromType(typ.NewMap(typ.String, elem)),
			flow.SymbolValueKey(valueSym): product.FromType(typ.NewOptional(typ.String)),
		},
	}
	if out.KeyPresence.HasValuePaths(itemPath, keyPath, valuePath) {
		t.Fatal("test setup failed to kill iterator provenance")
	}

	got := tr.genericForBodyEdgeState(in.Graph, branch, out)
	if !got.KeyPresence.HasValuePaths(itemPath, keyPath, valuePath) {
		t.Fatalf("generic-for body edge did not rebind iterator provenance: %s", got.KeyPresence.Format())
	}
	valueAV, _ := tr.symbolValue(&got, valueSym)
	if !typ.TypeEquals(valueAV.ProjectValue(), elem) {
		t.Fatalf("generic-for value rebinding = %v, want present element %v", valueAV.ProjectValue(), elem)
	}
}

func TestGenericForBodyEdgeRebindsCapturedCellIteratorValuePresent(t *testing.T) {
	in := keyPresenceSourceInput(t, `
for _, s in pairs(state.sessions) do
	local t = s.last_activity
end
`, "pairs")
	tr := New(in, Config{CallTyper: firstArgKeyedIterTyper{}})

	var branch cfg.Point
	var info *cfg.AssignInfo
	in.Graph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		node := in.Graph.Node(p)
		if branch == 0 && node != nil && node.LoopPreheaderSet {
			if assign := in.Graph.Assign(node.LoopPreheader); assign != nil && len(assign.IterExprs) > 0 {
				branch = p
				info = assign
			}
		}
	})
	if branch == 0 || info == nil {
		t.Fatal("generic-for branch not found")
	}
	stateSym, ok := in.Graph.SymbolAt(branch, "state")
	if !ok || stateSym == 0 {
		t.Fatal("state symbol not found")
	}
	sSym, ok := in.Graph.SymbolAt(branch, "s")
	if !ok || sSym == 0 {
		t.Fatal("s symbol not found")
	}
	if got := tr.symbolStorage.class(stateSym); got != symbolStorageCapturedCell {
		t.Fatalf("state storage class = %v, want captured cell", got)
	}

	elem := typ.NewRecord().
		Field("created_at", typ.Number).
		Field("last_activity", typ.Number).
		Build()
	stateValue := typ.NewRecord().
		Field("sessions", typ.NewMap(typ.String, elem)).
		Build()
	out := flow.PointState{
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: stateSym,
			Value:  product.FromType(stateValue),
		}}),
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sSym): product.FromType(typ.NewOptional(elem)),
		},
	}

	iterCall, ok := info.IterExprs[0].(*ast.FuncCallExpr)
	if !ok || iterCall == nil || len(iterCall.Args) == 0 {
		t.Fatal("iterator call not found")
	}
	sourceType := tr.resolveExprType(&out, iterCall.Args[0], nil)
	proj, projOK := iteration.ProjectVarTypes(effect.IterateKeyed, len(info.Targets), sourceType)
	if !projOK || len(proj.Types) < 2 || !typ.TypeEquals(proj.Types[1], elem) {
		t.Fatalf("captured iterator projection = %#v/%v from source %v, want value %v", proj, projOK, sourceType, elem)
	}

	got := tr.genericForBodyEdgeState(in.Graph, branch, out)
	valueAV, _ := tr.symbolValue(&got, sSym)
	if !typ.TypeEquals(valueAV.ProjectValue(), elem) {
		t.Fatalf("generic-for captured value rebinding = %v, want present element %v", valueAV.ProjectValue(), elem)
	}

	var body cfg.Point
	for _, succ := range in.Graph.Successors(branch) {
		if taken, ok := in.Graph.EdgeCond(branch, succ); ok && taken {
			body = succ
			break
		}
	}
	if body == 0 {
		t.Fatal("generic-for body successor not found")
	}
	edge := tr.NarrowEdge(in.Graph, branch, body, out)
	edgeAV, _ := tr.symbolValue(&edge, sSym)
	if !typ.TypeEquals(edgeAV.ProjectValue(), elem) {
		t.Fatalf("generic-for captured edge rebinding = %v, want present element %v", edgeAV.ProjectValue(), elem)
	}
}

func TestFuncDefKillsKeyPresenceEvenWhenValueCannotBeTyped(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(81), "items").Field("factory")
	keyPath := constraint.NewPath(cfg.SymbolID(82), "k")
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithPaths(tablePath, keyPath),
	}

	tr.applyFuncDef(&out, &cfg.FuncDefInfo{
		TargetPath: tablePath,
		FuncExpr:   &ast.FunctionExpr{},
	})

	if out.KeyPresence.HasPaths(tablePath, keyPath) {
		t.Fatalf("function definition kept stale KeyPresence: %s", out.KeyPresence.Format())
	}
}

func keyPresenceInput(t *testing.T, symbols map[*ast.IdentExpr]cfg.SymbolID) input.Inputs {
	t.Helper()
	in := input.BuildFromFunction(&ast.FunctionExpr{ParList: &ast.ParList{}}, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	for ident, sym := range symbols {
		in.Graph.Bindings().Bind(ident, sym)
		in.Graph.Bindings().SetName(sym, ident.Value)
	}
	return in
}

func keyPresenceSourceInput(t *testing.T, body string, globals ...string) input.Inputs {
	t.Helper()
	stmts, err := parse.ParseString(body, "key_presence.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return input.BuildFromFunction(&ast.FunctionExpr{Stmts: stmts, ParList: &ast.ParList{}}, nil, nil, globals...)
}

type keyPresenceTestTyper struct {
	captureEffectTyper
	source        ast.Expr
	indexedSource constraint.Path
	keysContainer constraint.Path
}

func (t keyPresenceTestTyper) KeyedIterSource(*ast.FuncCallExpr) (ast.Expr, bool) {
	return t.source, t.source != nil
}

func (t keyPresenceTestTyper) IndexedIterSource(*ast.FuncCallExpr) (constraint.Path, bool) {
	return t.indexedSource, !t.indexedSource.IsEmpty()
}

func (t keyPresenceTestTyper) KeysCollectorContainer(*cfg.CallInfo, int) (constraint.Path, bool) {
	return t.keysContainer, !t.keysContainer.IsEmpty()
}

type firstArgKeyedIterTyper struct {
	captureEffectTyper
}

func (t firstArgKeyedIterTyper) IterVarProjection(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) (iteration.VarProjection, bool) {
	if iter == nil || len(iter.Args) == 0 || exprType == nil {
		return iteration.VarProjection{}, false
	}
	return iteration.ProjectVarTypes(effect.IterateKeyed, count, exprType(iter.Args[0]))
}

func (t firstArgKeyedIterTyper) KeyedIterSource(call *ast.FuncCallExpr) (ast.Expr, bool) {
	if call == nil || len(call.Args) == 0 {
		return nil, false
	}
	return call.Args[0], true
}
