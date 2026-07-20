package factapply

import (
	"fmt"
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TestCoreAbstractInterpretationLaws is the bounded, deterministic law suite
// for the State product. Each row isolates one registered State lane; that
// keeps a violation attributable even though production solves use the full
// product. The last row exercises the complete value product with every
// standard axis represented.
//
// Widening bounds are deliberately small. A chain here has a fixed sample
// growth element, so it cannot introduce new map keys after its first join.
// Finite/must lanes therefore settle in at most two updates; four permits the
// one extra widening step used by product components. Thresholded num-ceils
// get six steps for their configured three thresholds before Top.
func TestCoreAbstractInterpretationLaws(t *testing.T) {
	t.Run("product-value-axes", func(t *testing.T) {
		reg := standard.Registry()
		domain := product.Domain(reg)
		latticelaws.LawSuite[product.Value]{
			Name:          "product.value-axes",
			Domain:        domain,
			Sample:        lawProductSamples(reg),
			Format:        lawFormatProduct(reg),
			WideningBound: 4,
		}.Run(t)
	})

	for _, tc := range stateLawCases() {
		t.Run(string(tc.lane), func(t *testing.T) {
			reg := tc.registry(t)
			domain, err := state.TryDomainWithOptionalLanesAndOptions(reg, []state.LaneID{tc.lane}, tc.options)
			if err != nil {
				t.Fatalf("build %s domain: %v", tc.lane, err)
			}
			latticelaws.LawSuite[state.State]{
				Name:          "state." + string(tc.lane),
				Domain:        domain,
				Sample:        tc.sample(t, reg, keyspace.New(), domain),
				Format:        lawFormatState,
				WideningBound: tc.wideningBound,
			}.Run(t)
		})
	}

	// These are the application points where non-monotonicity would invalidate
	// Kildall's worklist invariant: direct assignment, branch restriction,
	// implication-triggered restriction, and write invalidation. The grid is
	// intentionally small and ordered; full Cartesian fact coverage belongs to
	// their focused fixture tests, not this fast law suite.
	t.Run("core-transfer-monotonicity", testCoreTransferMonotonicity)
}

type stateLawCase struct {
	lane          state.LaneID
	registry      func(*testing.T) *axis.Registry
	options       state.DomainOptions
	sample        func(*testing.T, *axis.Registry, *keyspace.KeySpace, lattice.Lattice[state.State]) []state.State
	wideningBound int
}

func stateLawCases() []stateLawCase {
	standardRegistry := func(*testing.T) *axis.Registry { return standard.Registry() }
	return []stateLawCase{
		{state.LaneValues, standardRegistry, state.DomainOptions{}, sampleValuesLane, 4},
		{state.LanePathEvidence, standardRegistry, state.DomainOptions{}, samplePathEvidenceLane, 4},
		{state.LaneDynamicIndex, standardRegistry, state.DomainOptions{}, sampleDynamicIndexLane, 4},
		{state.LaneHeapTableIdentity, standardRegistry, state.DomainOptions{}, sampleHeapIdentityLane, 4},
		{state.LaneFrozenTables, standardRegistry, state.DomainOptions{}, sampleFrozenTablesLane, 4},
		{state.LaneEffectDeltas, standardRegistry, state.DomainOptions{}, sampleEffectDeltasLane, 4},
		{state.LaneEscapeEvents, standardRegistry, state.DomainOptions{}, sampleEscapeEventsLane, 4},
		{state.LaneChannelSelect, standardRegistry, state.DomainOptions{}, sampleChannelSelectLane, 4},
		{state.LaneStoreRelations, standardRegistry, state.DomainOptions{}, sampleStoreRelationsLane, 4},
		{state.LaneKeyMemberships, standardRegistry, state.DomainOptions{}, sampleKeyMembershipsLane, 4},
		{state.LaneTypestates, standardRegistry, state.DomainOptions{}, sampleTypestatesLane, 4},
		{state.LanePlacement, standardRegistry, state.DomainOptions{}, samplePlacementLane, 4},
		{state.LaneLenFloors, standardRegistry, state.DomainOptions{}, sampleLenFloorsLane, 4},
		{state.LaneNumFloors, standardRegistry, state.DomainOptions{}, sampleNumFloorsLane, 4},
		{state.LaneNumCeils, standardRegistry, state.DomainOptions{WidenThresholds: []int64{0, 10, 100}}, sampleNumCeilsLane, 6},
		{state.LaneDiffRelations, standardRegistry, state.DomainOptions{}, sampleDiffRelationsLane, 4},
		{state.LaneUserLattices, lawUserLatticeRegistry, state.DomainOptions{}, sampleUserLatticesLane, 4},
	}
}

func withStateExtrema(d lattice.Lattice[state.State], values ...state.State) []state.State {
	out := make([]state.State, 0, len(values)+2)
	out = append(out, d.Bottom(), d.Top())
	return append(out, values...)
}

func sampleValuesLane(t *testing.T, reg *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	first := statekey.SymbolValue(symbol.ID(9001))
	second := statekey.SymbolValue(symbol.ID(9002))
	values := lawProductSamples(reg)
	return withStateExtrema(d,
		d.Bottom().WriteValue(reg, first, values[2]),
		d.Bottom().WriteValue(reg, first, values[3]),
		d.Bottom().WriteValue(reg, first, values[9]).WriteValue(reg, second, values[10]),
	)
}

func samplePathEvidenceLane(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	trigger := lawPathKey(t, ks, "sym901@1.trigger")
	target := lawPathKey(t, ks, "sym902@1.target")
	member := pathdom.PathKey("sym902@1.target.field")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	implication := pathevidence.NewPathPresenceImplication(trigger, presence.Present(), target, presence.Absent())
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: trigger, Other: target}
	return withStateExtrema(d,
		d.Bottom().WritePathKey(reg, ks, triggerPathKey(t, ks, trigger), present),
		d.Bottom().WritePathStaticMember(ks, member, present).AddBranchProof(proof),
		d.Bottom().WritePathKey(reg, ks, triggerPathKey(t, ks, trigger), present).WritePathKey(reg, ks, triggerPathKey(t, ks, target), absent).AddPathPresenceImplication(implication),
	)
}

func sampleDynamicIndexLane(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	table := lawPathKey(t, ks, "sym903@1.table")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	first := dynamicindex.Key{Table: table, Site: "law-first"}
	second := dynamicindex.Key{Table: table, Site: "law-second"}
	return withStateExtrema(d,
		d.Bottom().WriteDynamicIndexFact(reg, first, dynamicindex.NewFact(reg, dynamicindex.FactConfig{KeyValue: present, HasKeyValue: true, Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})),
		d.Bottom().WriteDynamicIndexFact(reg, first, dynamicindex.NewFact(reg, dynamicindex.FactConfig{KeyValue: absent, HasKeyValue: true, Value: absent, HasValue: true, Admission: dynamicindex.AdmissionRejected})),
		d.Bottom().WriteDynamicIndexFact(reg, first, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionUnknown})).WriteDynamicIndexFact(reg, second, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: absent, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})),
	)
}

func sampleHeapIdentityLane(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	member := lawPathKey(t, ks, "sym904@1.field")
	dynamic := dynamicindex.Key{Table: lawPathKey(t, ks, "sym904@1.table"), Site: "heap-law"}
	first := identity.ID{Kind: "table", Site: "heap-law", Index: 1}
	second := identity.ID{Kind: "table", Site: "heap-law", Index: 2}
	prefix := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              present,
		StaticMembers:     map[keyspace.Key]product.Value{member: present},
		PrefixStableShape: true,
	})
	stable := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          present,
		StaticMembers: map[keyspace.Key]product.Value{member: present},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynamic: dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted}),
		},
		StableShape: true,
	})
	return withStateExtrema(d,
		d.Bottom().WriteHeapTableObject(reg, first, prefix),
		d.Bottom().WriteHeapTableObject(reg, first, stable),
		d.Bottom().WriteHeapTableObject(reg, first, prefix).WriteHeapTableObject(reg, second, stable),
	)
}

func sampleFrozenTablesLane(_ *testing.T, _ *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := identity.ID{Kind: "table", Site: "freeze-law", Index: 1}
	second := identity.ID{Kind: "table", Site: "freeze-law", Index: 2}
	return withStateExtrema(d, d.Bottom().FreezeTable(first), d.Bottom().FreezeTable(second), d.Bottom().FreezeTable(first).FreezeTable(second))
}

func sampleEffectDeltasLane(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	target := lawPathKey(t, ks, "sym905@1.target")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	first := effectdelta.Key{Target: target, Site: "effect-first", Kind: effectdelta.Mutation}
	second := effectdelta.Key{Target: target, Site: "effect-second", Kind: effectdelta.Mutation}
	changed := effectdelta.Value{Before: present, After: absent, Change: effectdelta.ChangeChanged}
	unchanged := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeNone}
	return withStateExtrema(d,
		d.Bottom().WriteEffectDelta(first, changed),
		d.Bottom().WriteEffectDelta(first, unchanged),
		d.Bottom().WriteEffectDelta(first, changed).WriteEffectDelta(second, unchanged),
	)
}

func sampleEscapeEventsLane(_ *testing.T, _ *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := escapeevent.Fact{Target: pathaddr.StateKey("sym906@1"), Kind: escapeevent.KindBorrow}
	second := escapeevent.Fact{Target: pathaddr.StateKey("sym907@1"), Kind: escapeevent.KindSend, Recursive: true}
	return withStateExtrema(d, d.Bottom().AddEscapeEvent(first), d.Bottom().AddEscapeEvent(second), d.Bottom().AddEscapeEvent(first).AddEscapeEvent(second))
}

func sampleChannelSelectLane(_ *testing.T, reg *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	first := channelselectfact.Fact{Select: "select-law", Kind: channelselectfact.FactSelect, Result: pathaddr.StateKey("sym908@1")}
	second := channelselectfact.Fact{Select: "select-law", Kind: channelselectfact.FactCase, Case: pathaddr.StateKey("sym909@1"), Index: 2, HasPayload: true, Payload: present}
	return withStateExtrema(d, d.Bottom().AddChannelSelectFact(first), d.Bottom().AddChannelSelectFact(second), d.Bottom().AddChannelSelectFact(first).AddChannelSelectFact(second))
}

func sampleStoreRelationsLane(_ *testing.T, _ *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := state.StoreRelation{Source: pathaddr.StateKey("sym910@1"), Into: pathaddr.StateKey("sym911@1")}
	second := state.StoreRelation{Source: pathaddr.StateKey("sym912@1"), Into: pathaddr.StateKey("sym913@1")}
	return withStateExtrema(d, d.Bottom().AddStoreRelation(first), d.Bottom().AddStoreRelation(second), d.Bottom().AddStoreRelation(first).AddStoreRelation(second))
}

func sampleKeyMembershipsLane(t *testing.T, _ *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	key := pathaddr.StateKey("sym914@1.key")
	table := pathaddr.StateKey("sym915@1.table")
	container := lawPathKey(t, ks, "sym916@1.container")
	return withStateExtrema(d,
		d.Bottom().AddPathKeyMembership(key, table),
		d.Bottom().AddDynamicIndexValueKeyMembership(container, dynamicindex.Site("membership"), table).AddDynamicIndexValueOrigin(key, container, dynamicindex.Site("membership")),
		d.Bottom().AddDynamicIndexAllValuesKeyMembership(container, table).
			AddDynamicIndexReadOrigin(key, container, pathaddr.StateKey("sym917@1.index")).
			AddPendingDynamicAllValueRestore(container, table, key),
	)
}

func sampleTypestatesLane(_ *testing.T, _ *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := typestate.Resource{ID: "sym918@1", Protocol: "transaction"}
	second := typestate.Resource{ID: "sym919@1", Protocol: "stream"}
	open := typestate.Obligation{Final: "closed"}
	return withStateExtrema(d,
		d.Bottom().AcquireTypestate(first, "open", open),
		d.Bottom().AcquireTypestate(second, "open", typestate.Obligation{Finals: typestate.NewFinalStates("closed", "committed")}).EscapeTypestate(second),
		d.Bottom().AcquireTypestate(first, "open", open).TransitionTypestate(first, "open", "closed"),
	)
}

func samplePlacementLane(_ *testing.T, _ *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := identity.ID{Kind: "alloc", Site: "placement-law", Index: 1}
	second := identity.ID{Kind: "alloc", Site: "placement-law", Index: 2}
	return withStateExtrema(d,
		d.Bottom().WritePlacement(first, placement.Stack),
		d.Bottom().WritePlacement(first, placement.OwnedHeap),
		d.Bottom().WritePlacement(first, placement.SharedHeap).WritePlacement(second, placement.Unknown),
	)
}

func sampleLenFloorsLane(_ *testing.T, _ *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := pathaddr.StateKey("sym920@1.array")
	second := pathaddr.StateKey("sym921@1.array")
	return withStateExtrema(d,
		d.Bottom().WriteLenFloor(ks, first, 1),
		d.Bottom().WriteLenFloor(ks, first, math.MaxInt64-1),
		d.Bottom().WriteLenFloor(ks, first, 4).WriteLenFloor(ks, second, 2),
	)
}

func sampleNumFloorsLane(_ *testing.T, _ *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := pathaddr.StateKey("sym922@1.number")
	second := pathaddr.StateKey("sym923@1.number")
	return withStateExtrema(d,
		d.Bottom().WriteNumFloor(ks, first, math.MinInt64+1),
		d.Bottom().WriteNumFloor(ks, first, 0),
		d.Bottom().WriteNumFloor(ks, first, math.MaxInt64-1).WriteNumFloor(ks, second, -1),
	)
}

func sampleNumCeilsLane(_ *testing.T, _ *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	first := pathaddr.StateKey("sym924@1.number")
	second := pathaddr.StateKey("sym925@1.number")
	return withStateExtrema(d,
		d.Bottom().WriteNumCeil(ks, first, math.MinInt64+1),
		d.Bottom().WriteNumCeil(ks, first, 10),
		d.Bottom().WriteNumCeil(ks, first, math.MaxInt64-1).WriteNumCeil(ks, second, 100),
	)
}

func sampleDiffRelationsLane(_ *testing.T, _ *axis.Registry, _ *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	a := state.RelValueOperand(pathaddr.StateKey("sym926@1.a"))
	b := state.RelValueOperand(pathaddr.StateKey("sym927@1.b"))
	c := state.RelLengthOperand(pathaddr.StateKey("sym928@1.items"))
	return withStateExtrema(d,
		d.Bottom().WriteDiffConstraint(a, b, -1),
		d.Bottom().WriteSumConstraint(a, b, c, 2),
		d.Bottom().WriteScaledConstraint(2, a, 3, b, c, 5),
	)
}

func sampleUserLatticesLane(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, d lattice.Lattice[state.State]) []state.State {
	t.Helper()
	const axisID userlattice.AxisID = "law.taint"
	path := pathaddr.StateKey("sym929@1.value")
	return withStateExtrema(d,
		d.Bottom().WriteUserElement(reg, ks, axisID, path, "Sanitized"),
		d.Bottom().WriteUserElement(reg, ks, axisID, path, "Tainted"),
		d.Bottom().WriteUserElement(reg, ks, axisID, path, "Sanitized").ApplyUserClaim(reg, ks, axisID, path, "tainted"),
	)
}

func lawUserLatticeRegistry(t *testing.T) *axis.Registry {
	t.Helper()
	reg := axis.NewRegistry()
	if _, err := userlattice.Register(reg, userlattice.Spec{
		ID:       "law.taint",
		Elements: []userlattice.ElementID{"Untainted", "Sanitized", "Tainted", "Unknown"},
		Bottom:   "Untainted",
		Top:      "Unknown",
		Order: []userlattice.OrderPair{
			{Lower: "Untainted", Upper: "Sanitized"},
			{Lower: "Untainted", Upper: "Tainted"},
			{Lower: "Sanitized", Upper: "Unknown"},
			{Lower: "Tainted", Upper: "Unknown"},
		},
		Hooks: userlattice.Hooks{OnClaim: []userlattice.ClaimHook{{Claim: "tainted", Element: "Tainted"}}},
	}); err != nil {
		t.Fatalf("register user lattice: %v", err)
	}
	return reg.Freeze()
}

func lawProductSamples(reg *axis.Registry) []product.Value {
	top := product.Top()
	present := product.WithPresence(reg, top, presence.Present())
	absent := product.WithPresence(reg, top, presence.Absent())
	variantA := product.Set(reg, top, variantorigin.Key, variantorigin.Singleton(1, 0))
	variantB := product.Set(reg, top, variantorigin.Key, variantorigin.Singleton(1, 1))
	identityA := product.Set(reg, top, identity.Key, identity.Singleton(identity.ID{Kind: "alloc", Site: "law", Index: 1}))
	identityB := product.Set(reg, top, identity.Key, identity.Singleton(identity.ID{Kind: "alloc", Site: "law", Index: 2}))
	tableKind := product.Set(reg, top, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	stringKind := product.Set(reg, top, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	stringWitness := product.Set(reg, top, typewitness.Key, typewitness.Of(typ.String))
	numberWitness := product.Set(reg, top, typewitness.Key, typewitness.Of(typ.Number))
	fresh := product.Set(reg, top, escape.Key, escape.Fresh())
	gradual := product.Set(reg, top, evidence.Key, evidence.GradualTop())
	explicit := product.Set(reg, top, evidence.Key, evidence.ExplicitTop())
	claimed := product.Set(reg, top, assertion.Key, assertion.Type())
	combo := product.Set(reg, present, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	combo = product.Set(reg, combo, escape.Key, escape.Fresh())
	combo = product.Set(reg, combo, assertion.Key, assertion.NonNil())
	return []product.Value{
		product.Bottom(reg), top, present, absent,
		variantA, variantB, identityA, identityB, tableKind, stringKind,
		stringWitness, numberWitness, fresh, gradual, explicit, claimed, combo,
		product.WithPresence(reg, top, presence.Bottom()),
	}
}

func lawFormatProduct(reg *axis.Registry) func(product.Value) string {
	return func(v product.Value) string {
		return fmt.Sprintf("hash=%d shape=%s presence=%s", product.Hash(reg, v), product.ShapeOf(v), product.PresenceOf(v))
	}
}

func lawFormatState(s state.State) string { return fmt.Sprintf("%#v", s) }

func lawPathKey(t *testing.T, ks *keyspace.KeySpace, raw string) keyspace.Key {
	t.Helper()
	key, ok := ks.FromStateKey(pathdom.PathKey(raw))
	if !ok {
		t.Fatalf("parse state key %q", raw)
	}
	return key
}

func triggerPathKey(t *testing.T, ks *keyspace.KeySpace, keyspaceKey keyspace.Key) pathdom.PathKey {
	t.Helper()
	key := ks.Format(keyspaceKey)
	if key == "" {
		t.Fatal("format law path key")
	}
	return key
}

func testCoreTransferMonotonicity(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	point := cfg.Point(930)
	target := symbol.ID(930)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	top := product.Top()
	base := state.Reachable(state.State{})

	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	trigger := symbol.ID(931)
	builder.Define(point, trigger, "trigger")
	resolver := visibility.NewResolver(builder.Build())

	t.Run("implication-activation", func(t *testing.T) {
		ks := resolver.KeySpace()
		triggerKey := ks.FromPath(pathdom.NewPath(trigger, "trigger"))
		targetKey := ks.FromPath(pathdom.NewPath(target, "target"))
		implication := pathevidence.NewPathPresenceImplication(triggerKey, presence.Present(), targetKey, presence.Absent())
		withImplication := func(triggerValue product.Value) state.State {
			return base.
				WriteValue(reg, statekey.SymbolValue(trigger), triggerValue).
				WriteValue(reg, statekey.SymbolValue(target), top).
				AddPathPresenceImplication(implication)
		}
		assertTransferMonotone(t, domain, []state.State{withImplication(present), withImplication(top)}, func(in state.State) state.State {
			return activatePathPresenceImplications(reg, resolver, point, in)
		})
	})

	t.Run("path-invalidation", func(t *testing.T) {
		path := pathdom.NewPath(target, "target").Field("field")
		pathKey := resolver.KeyAt(point, path)
		withFact := base.WritePathKey(reg, resolver.KeySpace(), pathKey, present)
		assertTransferMonotone(t, domain, []state.State{withFact, base}, func(in state.State) state.State {
			out, ok := invalidatePathSubtreeAt(in, resolver, point, path)
			if !ok {
				return in
			}
			return out
		})
	})
}

func assertTransferMonotone(t *testing.T, domain lattice.Lattice[state.State], sample []state.State, apply func(state.State) state.State) {
	t.Helper()
	for _, a := range sample {
		for _, b := range sample {
			if !domain.LessOrEq(a, b) {
				continue
			}
			fa, fb := apply(a), apply(b)
			if !domain.LessOrEq(fa, fb) {
				t.Fatalf("non-monotone transfer: a ⊑ b but f(a) ⊑ f(b) fails\na=%#v\nb=%#v\nf(a)=%#v\nf(b)=%#v", a, b, fa, fb)
			}
		}
	}
}
