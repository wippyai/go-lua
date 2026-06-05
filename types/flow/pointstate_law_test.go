package flow

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// TestPointStateDomain_Laws validates the canonical intraprocedural carrier.
//
// PointStateDomain is the componentwise reduced product of independently
// law-tested domains (envDomain over product.Domain, constraint.Domain,
// numeric.StateDomain, relation domains, CaptureCellsDomain, and
// CaptureEffectsDomain). A product of law-satisfying lattices is itself a
// law-satisfying lattice, so the laws must hold here by construction; this suite
// exists to catch a COMPOSITION bug — a field delegating to the wrong component,
// a component left nil where the carrier requires a value, or a Meet wired where
// a component lacks one — rather than to re-prove the components.
//
// The sample crosses each component independently (one component non-trivial
// at a time) and jointly (all three non-trivial), so a swapped delegation
// surfaces as an antisymmetry / upper-bound / termination violation.
func TestPointStateDomain_Laws(t *testing.T) {
	lattice.LawSuite[PointState]{
		Name:   "PointState",
		Domain: PointStateDomain,
		Sample: pointStateSample(),
		Format: formatPointState,
	}.Run(t)
}

func TestPointStateJoinKeepsBranchLocalStaticIndexInstallOptional(t *testing.T) {
	const sym = cfg.SymbolID(901)
	message := typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("self", typ.Any).Returns(typ.String).Build()).
		Build()
	base := product.FromType(typ.NewMap(typ.String, message))
	installed := product.WithMember(base, value.MemberStringIndex("root"), product.FromType(message))

	left := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): base,
		},
	}
	right := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): installed,
		},
		KeyPresence: KeyPresenceFacts{}.With(
			SymbolPathKey(sym, nil),
			SymbolPathKey(cfg.SymbolID(902), nil),
		),
	}

	joined := PointStateDomain.Join(left, right)
	av, ok := SymbolValue(joined, sym)
	if !ok || av.IsZero() {
		t.Fatal("joined PointState lost installed/base symbol")
	}
	got, ok := product.MemberOf(av, value.MemberStringIndex("root"))
	want := typ.NewOptional(message)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("PointState join [\"root\"] = %v, %v; want %v,true", got.ProjectValue(), ok, want)
	}
	if joined.KeyPresence.Has(SymbolPathKey(sym, nil), SymbolPathKey(cfg.SymbolID(902), nil)) {
		t.Fatalf("PointState join kept one-branch KeyPresence: %s", joined.KeyPresence.Format())
	}
}

func TestPointStateJoinKeepsCommonStaticMemberFact(t *testing.T) {
	const sym = cfg.SymbolID(905)
	path := SymbolPathKey(sym, []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "config"},
		{Kind: constraint.SegmentField, Name: "data_targets"},
	})
	installed := reachableEmptyPointState()
	installed.StaticMembers = installed.StaticMembers.With(path, product.FromType(typ.NewRecord().Build()))
	present := reachableEmptyPointState()
	present.StaticMembers = present.StaticMembers.With(path, product.PresentDynamic())

	joined := PointStateDomain.Join(installed, present)
	got, ok := joined.StaticMembers.Value(path)
	if !ok || got.IsZero() {
		t.Fatalf("join dropped common static member fact: %s", joined.StaticMembers.Format())
	}
	if !got.DefinitelyPresent() {
		t.Fatalf("join static member fact = %s, want definitely-present value", got.ProjectValue())
	}
	if !PointStateDomain.LessOrEq(installed, joined) {
		t.Fatalf("installed arm must be below static-member join:\ninstalled=%s\njoined=%s", formatPointState(installed), formatPointState(joined))
	}
	if !PointStateDomain.LessOrEq(present, joined) {
		t.Fatalf("present arm must be below static-member join:\npresent=%s\njoined=%s", formatPointState(present), formatPointState(joined))
	}
}

func TestPointStateJoinKeepsNilGatedKeyPresence(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(930), "nodes")
	key := constraint.NewPath(cfg.SymbolID(931), "last_id")
	valuePath := constraint.NewPath(cfg.SymbolID(932), "node")
	nilArm := reachableEmptyPointState()
	nilArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.Nil),
	}
	factArm := reachableEmptyPointState()
	factArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.String),
	}
	factArm.KeyPresence = factArm.KeyPresence.WithValuePaths(table, key, valuePath)

	joined := PointStateDomain.Join(nilArm, factArm)
	if !joined.KeyPresence.HasValuePaths(table, key, valuePath) {
		t.Fatalf("nil-gated join dropped key-presence fact: %s", joined.KeyPresence.Format())
	}
	if !PointStateDomain.LessOrEq(nilArm, joined) {
		t.Fatalf("nil predecessor must be below guarded key-presence join:\nnil=%s\njoined=%s", formatPointState(nilArm), formatPointState(joined))
	}
	if !PointStateDomain.LessOrEq(factArm, joined) {
		t.Fatalf("fact predecessor must be below guarded key-presence join:\nfact=%s\njoined=%s", formatPointState(factArm), formatPointState(joined))
	}
}

func TestPointStateJoinDropsOneBranchKeyPresenceWhenMissingBranchMayHaveKey(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(940), "nodes")
	key := constraint.NewPath(cfg.SymbolID(941), "last_id")
	missingArm := reachableEmptyPointState()
	missingArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.String),
	}
	factArm := reachableEmptyPointState()
	factArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.String),
	}
	factArm.KeyPresence = factArm.KeyPresence.WithPaths(table, key)

	joined := PointStateDomain.Join(missingArm, factArm)
	if joined.KeyPresence.HasPaths(table, key) {
		t.Fatalf("join kept non-nil one-branch key-presence fact: %s", joined.KeyPresence.Format())
	}
}

func TestPointStateOrderSeesKeyArrayValuePayload(t *testing.T) {
	array := constraint.NewPath(cfg.SymbolID(945), "node_order")
	table := constraint.NewPath(cfg.SymbolID(946), "nodes")
	tableOnly := reachableEmptyPointState()
	tableOnly.KeyPresence = tableOnly.KeyPresence.WithKeyArrayPaths(array, table)
	withValue := reachableEmptyPointState()
	withValue.KeyPresence = withValue.KeyPresence.WithKeyArrayValuePaths(array, table, product.FromType(typ.String))

	if !PointStateDomain.LessOrEq(withValue, tableOnly) {
		t.Fatalf("value-carrying key-array proof should imply table-only proof:\nvalue=%s\ntable=%s", formatPointState(withValue), formatPointState(tableOnly))
	}
	if PointStateDomain.LessOrEq(tableOnly, withValue) {
		t.Fatalf("table-only key-array proof must not imply value payload:\ntable=%s\nvalue=%s", formatPointState(tableOnly), formatPointState(withValue))
	}
}

func TestPointStateOrderSeesAppendHistoryAxes(t *testing.T) {
	array := KeyPresencePathKey(constraint.NewPath(cfg.SymbolID(947), "node_order"))
	key := KeyPresencePathKey(constraint.NewPath(cfg.SymbolID(948), "node_id"))
	plain := reachableEmptyPointState()
	base := reachableEmptyPointState()
	base.KeyPresence = base.KeyPresence.WithAppendHistoryBase(array)
	event := reachableEmptyPointState()
	event.KeyPresence = base.KeyPresence.WithAppendHistoryEvent(array, key)

	if PointStateDomain.LessOrEq(plain, base) {
		t.Fatalf("plain state must not imply append-history base:\nplain=%s\nbase=%s", formatPointState(plain), formatPointState(base))
	}
	if !PointStateDomain.LessOrEq(base, plain) {
		t.Fatalf("append-history base should imply plain key-presence state:\nbase=%s\nplain=%s", formatPointState(base), formatPointState(plain))
	}
	if PointStateDomain.LessOrEq(event, base) {
		t.Fatalf("possible append event must not imply event-free base:\nevent=%s\nbase=%s", formatPointState(event), formatPointState(base))
	}
	if !PointStateDomain.LessOrEq(base, event) {
		t.Fatalf("event-free base should be below tracked-base plus possible event:\nbase=%s\nevent=%s", formatPointState(base), formatPointState(event))
	}
}

func TestPointStateJoinKeepsNilGatedPathAlias(t *testing.T) {
	key := constraint.NewPath(cfg.SymbolID(950), "last_id")
	source := constraint.NewPath(cfg.SymbolID(951), "node_id")
	nilArm := reachableEmptyPointState()
	nilArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.Nil),
	}
	factArm := reachableEmptyPointState()
	factArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.String),
	}
	factArm.PathAliases = factArm.PathAliases.WithPaths(key, source)

	joined := PointStateDomain.Join(nilArm, factArm)
	if len(joined.PathAliases.AliasesOfPath(key)) != 1 {
		t.Fatalf("nil-gated join dropped path alias: %s", joined.PathAliases.Format())
	}
	if !PointStateDomain.LessOrEq(nilArm, joined) {
		t.Fatalf("nil predecessor must be below guarded path-alias join:\nnil=%s\njoined=%s", formatPointState(nilArm), formatPointState(joined))
	}
	if !PointStateDomain.LessOrEq(factArm, joined) {
		t.Fatalf("fact predecessor must be below guarded path-alias join:\nfact=%s\njoined=%s", formatPointState(factArm), formatPointState(joined))
	}
}

func TestPointStateJoinDropsOneBranchPathAliasWhenMissingBranchMayHaveValue(t *testing.T) {
	key := constraint.NewPath(cfg.SymbolID(960), "last_id")
	source := constraint.NewPath(cfg.SymbolID(961), "node_id")
	missingArm := reachableEmptyPointState()
	missingArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.String),
	}
	factArm := reachableEmptyPointState()
	factArm.Env = map[ValueKey]product.AbstractValue{
		SymbolValueKey(key.Symbol): product.FromType(typ.String),
	}
	factArm.PathAliases = factArm.PathAliases.WithPaths(key, source)

	joined := PointStateDomain.Join(missingArm, factArm)
	if len(joined.PathAliases.AliasesOfPath(key)) != 0 {
		t.Fatalf("join kept non-nil one-branch path alias: %s", joined.PathAliases.Format())
	}
}

func TestPointStateJoinDropsOneBranchMustFacts(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(910), "messages")
	key := constraint.NewPath(cfg.SymbolID(911), "key")
	valuePath := constraint.NewPath(cfg.SymbolID(912), "entry")
	sourcePath := constraint.NewPath(cfg.SymbolID(913), "items")
	errSym := cfg.SymbolID(914)

	empty := reachableEmptyPointState()
	factful := reachableEmptyPointState()
	factful.StaticMembers = factful.StaticMembers.With(
		KeyPresencePathKey(table.IndexStr("root")),
		product.FromType(typ.String),
	)
	factful.KeyPresence = factful.KeyPresence.
		WithPaths(table, key).
		WithValuePaths(table, key, valuePath)
	factful.ValueOrigins = factful.ValueOrigins.WithPaths(
		valuePath,
		sourcePath,
		ValueOriginIndexedIterator,
		1,
	)
	factful.PathAliases = factful.PathAliases.WithPaths(key, sourcePath)
	factful.IndexWrites = factful.IndexWrites.With(IndexWriteAdmissionFact{
		Target:    KeyPresencePathKey(table),
		KeyPath:   KeyPresencePathKey(key),
		Key:       product.FromType(typ.String),
		ValuePath: KeyPresencePathKey(valuePath),
		Value:     product.FromType(typ.Number),
	})
	factful.Rel = factful.Rel.
		WithSiblingNil(errSym, []cfg.SymbolID{valuePath.Symbol}).
		WithContainerLowerBound(table.Symbol, KeyPresencePathKey(table), 2)

	if !PointStateDomain.LessOrEq(factful, empty) {
		t.Fatalf("factful reachable state should be below empty must-fact state:\nfactful=%s\nempty=%s", formatPointState(factful), formatPointState(empty))
	}
	if PointStateDomain.LessOrEq(empty, factful) {
		t.Fatalf("empty must-fact state should not imply branch-local facts:\nempty=%s\nfactful=%s", formatPointState(empty), formatPointState(factful))
	}

	joined := PointStateDomain.Join(empty, factful)
	reverse := PointStateDomain.Join(factful, empty)
	if !PointStateDomain.Equal(joined, reverse) {
		t.Fatalf("PointState join is not deterministic/commutative:\nleft=%s\nright=%s", formatPointState(joined), formatPointState(reverse))
	}
	assertPointStateDroppedOneBranchMustFacts(t, joined, table, key, valuePath, errSym)

	widened := PointStateDomain.Widen(empty, factful)
	assertPointStateDroppedOneBranchMustFacts(t, widened, table, key, valuePath, errSym)
}

func reachableEmptyPointState() PointState {
	return PointState{
		Env:                envDomain.Bottom(),
		Cond:               constraint.Domain.Top(),
		Num:                numeric.NewState(),
		Rel:                PointRelationsDomain.Top(),
		ReturnRel:          ReturnRelationsDomain.Top(),
		Cells:              CaptureCellsDomain.Bottom(),
		CellEffects:        CaptureEffectsIdentity(),
		PrototypeSelf:      PrototypeSelfDomain.Bottom(),
		PrototypeInstances: PrototypeInstancesDomain.Bottom(),
		FunctionRefs:       FunctionRefsDomain.Bottom(),
		ClosureRefs:        ClosureRefsDomain.Bottom(),
		StaticMembers:      StaticMemberFactsDomain.Top(),
		KeyPresence:        KeyPresenceFactsDomain.Top(),
		ValueOrigins:       ValueOriginFactsDomain.Top(),
		PathAliases:        PathAliasFactsDomain.Top(),
	}
}

func assertPointStateDroppedOneBranchMustFacts(t *testing.T, ps PointState, table, key, valuePath constraint.Path, errSym cfg.SymbolID) {
	t.Helper()
	if _, ok := ps.StaticMembers.Value(KeyPresencePathKey(table.IndexStr("root"))); ok {
		t.Fatalf("PointState kept one-branch StaticMembers fact: %s", ps.StaticMembers.Format())
	}
	if ps.KeyPresence.HasPaths(table, key) || ps.KeyPresence.HasValuePaths(table, key, valuePath) {
		t.Fatalf("PointState kept one-branch KeyPresence fact: %s", ps.KeyPresence.Format())
	}
	if got := ps.ValueOrigins.OriginsOfPath(valuePath); len(got) != 0 {
		t.Fatalf("PointState kept one-branch ValueOrigins fact: %s", ps.ValueOrigins.Format())
	}
	if got := ps.PathAliases.AliasesOfPath(key); len(got) != 0 {
		t.Fatalf("PointState kept one-branch PathAliases fact: %s", ps.PathAliases.Format())
	}
	if rel, ok := ps.Rel.SiblingNil(errSym); ok {
		t.Fatalf("PointState kept one-branch relation: %#v", rel)
	}
	if ps.Rel.HasContainerLowerBound(table.Symbol, KeyPresencePathKey(table), 1) {
		t.Fatalf("PointState kept one-branch container cardinality relation: %#v", ps.Rel)
	}
	if _, ok := ps.IndexWrites.Admission(IndexWriteQuery{
		Target:    table,
		KeySymbol: key.Symbol,
		KeyType:   typ.String,
		ValuePath: valuePath,
	}); ok {
		t.Fatalf("PointState kept one-branch IndexWrites fact: %s", ps.IndexWrites.Format())
	}
}

// pointStateSample builds Bottom, Top, and a structural cross-section in which
// each component varies independently and jointly. Every PointState sets all
// fields to valid component elements: Env may be nil (MapLattice reads
// absence as product.Domain.Bottom()), but Cond and Num must be real domain
// values, never their Go zero value.
func pointStateSample() []PointState {
	x := constraint.Path{Root: "x", Symbol: cfg.SymbolID(1)}
	y := constraint.Path{Root: "y", Symbol: cfg.SymbolID(2)}

	// Condition cross-section: domain extremes plus two finite conditions.
	condTruthy := constraint.FromConstraints(constraint.Truthy{Path: x})
	condTwo := constraint.And(
		constraint.FromConstraints(constraint.Truthy{Path: x}),
		constraint.FromConstraints(constraint.NotNil{Path: y}),
	)

	// Numeric cross-section: domain extremes plus a bounded state.
	numBounded := numeric.NewState()
	numBounded.ApplyGeConst("x", 0)
	numBounded.ApplyLeConst("x", 100)

	// Env cross-section: empty (Bottom), single key, multi key, top sentinel.
	envOne := map[ValueKey]product.AbstractValue{ValueKey("x"): product.FromType(typ.String)}
	envTwo := map[ValueKey]product.AbstractValue{
		ValueKey("x"): product.FromType(typ.Number),
		ValueKey("y"): product.FromType(typ.Integer),
	}

	cellsOne := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)}})
	cellsTwo := CaptureCellsOf([]CaptureCell{
		{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.Number)},
		{Symbol: cfg.SymbolID(11), Value: product.FromType(typ.Boolean)},
	})
	effectsOne := CaptureMustWrite(cfg.SymbolID(10), product.FromType(typ.String))
	effectsTwo := CaptureEffectsOf([]CaptureEffect{
		{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.Number), MustWrite: false},
		{Symbol: cfg.SymbolID(12), Value: product.FromType(typ.Boolean), MustWrite: true},
	})
	protoOne := PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: cfg.SymbolID(20), Value: product.FromType(typ.String)}})
	protoTwo := PrototypeSelfOf([]PrototypeSelfEntry{
		{Prototype: cfg.SymbolID(20), Value: product.FromType(typ.Number)},
		{Prototype: cfg.SymbolID(21), Value: product.FromType(typ.Boolean)},
	})
	instOne := PrototypeInstancesOf([]PrototypeInstanceEntry{{Symbol: cfg.SymbolID(25), Prototypes: []cfg.SymbolID{20}}})
	instTwo := PrototypeInstancesOf([]PrototypeInstanceEntry{
		{Symbol: cfg.SymbolID(25), Prototypes: []cfg.SymbolID{20, 21}},
		{Symbol: cfg.SymbolID(26), Prototypes: []cfg.SymbolID{22}},
	})
	refsOne := WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(30), "fn").Key(), FunctionRefSetOf(FunctionRef{GraphID: 300}))
	closureOne := WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(31), "closure").Key(), ClosureRefSetOf(ClosureRefOf(
		FunctionRef{GraphID: 301},
		CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)}}),
		refsOne,
	)))
	closureTwo := WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(31), "closure").Key(), ClosureRefSetOf(
		ClosureRefOf(
			FunctionRef{GraphID: 301},
			CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.Number)}}),
			FunctionRefsDomain.Bottom(),
		),
		ClosureRefOf(
			FunctionRef{GraphID: 302},
			CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(11), Value: product.FromType(typ.Boolean)}}),
			refsOne,
		),
	))
	staticOne := StaticMemberFactsOf([]StaticMemberFact{{
		Path:  SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}}),
		Value: product.FromType(typ.String),
	}})
	staticTwo := StaticMemberFactsOf([]StaticMemberFact{
		{
			Path:  SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}}),
			Value: product.FromType(typ.Number),
		},
		{
			Path:  SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}}),
			Value: product.FromType(typ.Boolean),
		},
	})
	keyPresenceOne := KeyPresenceFactsOf([]KeyPresenceFact{{
		Table: SymbolPathKey(cfg.SymbolID(1), nil),
		Key:   SymbolPathKey(cfg.SymbolID(3), nil),
	}})
	keyPresenceTwo := KeyPresenceFactsOf([]KeyPresenceFact{
		{
			Table: SymbolPathKey(cfg.SymbolID(1), nil),
			Key:   SymbolPathKey(cfg.SymbolID(3), nil),
		},
		{
			Table: SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}}),
			Key:   SymbolPathKey(cfg.SymbolID(4), nil),
		},
	})
	valueOriginOne := ValueOriginFacts{}.WithPaths(
		constraint.NewPath(cfg.SymbolID(5), "entry"),
		constraint.NewPath(cfg.SymbolID(6), "items"),
		ValueOriginIndexedIterator,
		1,
	)
	valueOriginTwo := valueOriginOne.WithPaths(
		constraint.NewPath(cfg.SymbolID(7), "key"),
		constraint.NewPath(cfg.SymbolID(6), "items"),
		ValueOriginKeyedIterator,
		0,
	)
	indexWriteOne := IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{{
		Target:    SymbolPathKey(cfg.SymbolID(1), nil),
		KeyPath:   SymbolPathKey(cfg.SymbolID(3), nil),
		Key:       product.FromType(typ.String),
		ValuePath: SymbolPathKey(cfg.SymbolID(5), nil),
		Value:     product.FromType(typ.Number),
	}})
	indexWriteTwo := IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
		{
			Target:    SymbolPathKey(cfg.SymbolID(1), nil),
			KeyPath:   SymbolPathKey(cfg.SymbolID(3), nil),
			Key:       product.FromType(typ.String),
			ValuePath: SymbolPathKey(cfg.SymbolID(5), nil),
			Value:     product.FromType(typ.Integer),
		},
		{
			Target:    SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}}),
			KeyPath:   SymbolPathKey(cfg.SymbolID(4), nil),
			Key:       product.FromType(typ.Number),
			ValuePath: SymbolPathKey(cfg.SymbolID(8), nil),
			Value:     product.FromType(typ.Boolean),
		},
	})

	mk := func(env map[ValueKey]product.AbstractValue, cond constraint.Condition, num *numeric.State, cells CaptureCells, effects CaptureEffects, proto PrototypeSelf, instances PrototypeInstances, closures ClosureRefs, static StaticMemberFacts, keyPresence KeyPresenceFacts, valueOrigins ValueOriginFacts, indexWrites IndexWriteAdmissionFacts) PointState {
		return PointState{Env: env, Cond: cond, Num: num, Cells: cells, CellEffects: effects, PrototypeSelf: proto, PrototypeInstances: instances, ClosureRefs: closures, StaticMembers: static, KeyPresence: keyPresence, ValueOrigins: valueOrigins, IndexWrites: indexWrites}
	}

	return []PointState{
		PointStateDomain.Bottom(),
		PointStateDomain.Top(),

		// One component non-trivial at a time, the other two at Bottom.
		mk(envOne, constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), condTruthy, numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numBounded, CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), cellsOne, CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), effectsOne, PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), protoOne, PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), instOne, ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), closureOne, StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), staticOne, KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), keyPresenceOne, ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), valueOriginOne, IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), indexWriteOne),

		// Pairs and a fully-mixed point.
		mk(envTwo, condTwo, numeric.StateDomain.Bottom(), CaptureCellsDomain.Bottom(), CaptureEffectsDomain.Bottom(), PrototypeSelfDomain.Bottom(), PrototypeInstancesDomain.Bottom(), ClosureRefsDomain.Bottom(), StaticMemberFactsDomain.Bottom(), KeyPresenceFactsDomain.Bottom(), ValueOriginFactsDomain.Bottom(), IndexWriteAdmissionFactsDomain.Bottom()),
		mk(envOne, constraint.Domain.Bottom(), numBounded, cellsOne, effectsOne, protoOne, instOne, closureOne, staticOne, keyPresenceOne, valueOriginOne, indexWriteOne),
		mk(envTwo, condTwo, numBounded, cellsTwo, effectsTwo, protoTwo, instTwo, closureTwo, staticTwo, keyPresenceTwo, valueOriginTwo, indexWriteTwo),

		// One component at Top, others finite — exercises the envDomain top
		// sentinel and condition/numeric Top against finite neighbours.
		mk(envDomain.Top(), condTruthy, numBounded, cellsOne, effectsOne, protoOne, instOne, closureOne, staticOne, keyPresenceOne, valueOriginOne, indexWriteOne),
		mk(envOne, constraint.Domain.Top(), numeric.StateDomain.Top(), CaptureCellsDomain.Top(), CaptureEffectsDomain.Top(), PrototypeSelfDomain.Top(), PrototypeInstancesDomain.Top(), ClosureRefsDomain.Top(), StaticMemberFactsDomain.Top(), KeyPresenceFactsDomain.Top(), ValueOriginFactsDomain.Top(), IndexWriteAdmissionFactsDomain.Top()),
	}
}

func formatPointState(p PointState) string {
	return fmt.Sprintf("{Env:%v Cond:%v Num:%v Cells:%s Effects:%s PrototypeSelf:%s PrototypeInstances:%s ClosureRefs:%v StaticMembers:%s KeyPresence:%s ValueOrigins:%s PathAliases:%s IndexWrites:%s}",
		p.Env, constraint.Domain.Equal(p.Cond, constraint.Domain.Top()), numeric.StateDomain.Equal(p.Num, numeric.StateDomain.Top()), p.Cells.Format(), p.CellEffects.Format(), p.PrototypeSelf.Format(), p.PrototypeInstances.Format(), p.ClosureRefs, p.StaticMembers.Format(), p.KeyPresence.Format(), p.ValueOrigins.Format(), p.PathAliases.Format(), p.IndexWrites.Format())
}
