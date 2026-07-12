package semanticplan

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type EffectKind string

const (
	EffectInvalidateRoot     EffectKind = "invalidate-root-origins"
	EffectWriteValue         EffectKind = "write-value"
	EffectInvalidatePath     EffectKind = "invalidate-path"
	EffectRelatePaths        EffectKind = "relate-paths"
	EffectInvalidateHeap     EffectKind = "invalidate-heap"
	EffectClearDynamicIndex  EffectKind = "clear-dynamic-index"
	EffectClearKeyMembership EffectKind = "clear-key-membership"
	EffectClearLenFloor      EffectKind = "clear-len-floor"
	EffectCopyUserLattice    EffectKind = "copy-user-lattice"
)

type Term struct {
	Path pathdom.Path
}

type GuardKind string

const GuardSourceAvailable GuardKind = "source-available"

type Guard struct {
	Kind GuardKind
	Term Term
}

type Effect struct {
	Lane   state.LaneID
	Kind   EffectKind
	Phase  uint8
	Target Term
	Source Term
}

type GuardedRow struct {
	Guards  []Guard
	Effects []Effect
}

type SymbolicTransformer struct {
	rows      []GuardedRow
	fallback  []state.LaneID
	operation PathAssignmentOp
	// executable means an exact, deliberately narrow concrete interpreter is
	// available. Execute still rejects states whose heap/origin/static-member
	// behavior depends on production helpers that are not exported.
	executable bool
}

func (t SymbolicTransformer) Contextual() bool { return !t.executable || len(t.fallback) != 0 }

// TermComplete reports that every declared lane produced a term. It does not
// authorize publication until an exact State/boundary interpreter exists.
func (t SymbolicTransformer) TermComplete() bool { return len(t.fallback) == 0 }

func (t SymbolicTransformer) FallbackLanes() []state.LaneID {
	return append([]state.LaneID(nil), t.fallback...)
}

type LaneAdapter interface {
	Lane() state.LaneID
	LiftPathAssignment(PathAssignmentOp) ([]Effect, bool)
}

type SymbolicRegistry struct {
	order    []state.LaneID
	adapters map[state.LaneID]LaneAdapter
}

func NewSymbolicRegistry(adapters ...LaneAdapter) (*SymbolicRegistry, error) {
	order := state.DefaultLaneCatalog().LaneSet().IDs()
	known := make(map[state.LaneID]struct{}, len(order))
	for _, lane := range order {
		known[lane] = struct{}{}
	}
	byLane := make(map[state.LaneID]LaneAdapter, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			return nil, fmt.Errorf("semanticplan: nil lane adapter")
		}
		lane := adapter.Lane()
		if _, ok := known[lane]; !ok {
			return nil, fmt.Errorf("semanticplan: orphan lane adapter %q", lane)
		}
		if _, duplicate := byLane[lane]; duplicate {
			return nil, fmt.Errorf("semanticplan: duplicate lane adapter %q", lane)
		}
		byLane[lane] = adapter
	}
	return &SymbolicRegistry{order: order, adapters: byLane}, nil
}

func DefaultPathAssignmentRegistry() *SymbolicRegistry {
	registry, err := NewSymbolicRegistry(
		pathAssignmentLaneAdapter{lane: state.LaneValues, effects: []phasedEffect{{EffectInvalidateRoot, 10}, {EffectWriteValue, 40}}},
		pathAssignmentLaneAdapter{lane: state.LanePathEvidence, effects: []phasedEffect{{EffectInvalidatePath, 30}, {EffectRelatePaths, 50}}},
		pathAssignmentLaneAdapter{lane: state.LaneHeapTableIdentity, effects: []phasedEffect{{EffectInvalidateHeap, 20}}},
		pathAssignmentLaneAdapter{lane: state.LaneDynamicIndex, effects: []phasedEffect{{EffectClearDynamicIndex, 30}}},
		pathAssignmentLaneAdapter{lane: state.LaneKeyMemberships, effects: []phasedEffect{{EffectClearKeyMembership, 30}}},
		pathAssignmentLaneAdapter{lane: state.LaneLenFloors, effects: []phasedEffect{{EffectClearLenFloor, 30}}},
		pathAssignmentLaneAdapter{lane: state.LaneUserLattices, effects: []phasedEffect{{EffectCopyUserLattice, 60}}},
		unaffectedLaneAdapter{lane: state.LaneFrozenTables},
		unaffectedLaneAdapter{lane: state.LaneEffectDeltas},
		unaffectedLaneAdapter{lane: state.LaneEscapeEvents},
		unaffectedLaneAdapter{lane: state.LaneChannelSelect},
		unaffectedLaneAdapter{lane: state.LaneStoreRelations},
		unaffectedLaneAdapter{lane: state.LaneTypestates},
		unaffectedLaneAdapter{lane: state.LanePlacement},
		unaffectedLaneAdapter{lane: state.LaneNumFloors},
		unaffectedLaneAdapter{lane: state.LaneNumCeils},
		unaffectedLaneAdapter{lane: state.LaneDiffRelations},
	)
	if err != nil {
		panic(err)
	}
	return registry
}

type pathAssignmentLaneAdapter struct {
	lane    state.LaneID
	effects []phasedEffect
}

type phasedEffect struct {
	kind  EffectKind
	phase uint8
}

type unaffectedLaneAdapter struct{ lane state.LaneID }

func (a unaffectedLaneAdapter) Lane() state.LaneID { return a.lane }
func (unaffectedLaneAdapter) LiftPathAssignment(PathAssignmentOp) ([]Effect, bool) {
	return nil, true
}

func (a pathAssignmentLaneAdapter) Lane() state.LaneID { return a.lane }
func (a pathAssignmentLaneAdapter) LiftPathAssignment(op PathAssignmentOp) ([]Effect, bool) {
	if !op.HasSourcePath || op.Target.IsEmpty() || len(op.Target.Segments) == 0 || op.HasStaticMemberWrite || op.CovariantExposures != 0 {
		return nil, false
	}
	target, source := Term{Path: op.Target.Clone()}, Term{Path: op.SourcePath.Clone()}
	out := make([]Effect, len(a.effects))
	for i, effect := range a.effects {
		out[i] = Effect{Lane: a.lane, Kind: effect.kind, Phase: effect.phase, Target: target, Source: source}
	}
	return out, true
}

func (r *SymbolicRegistry) Lift(op PathAssignmentOp) SymbolicTransformer {
	if r == nil {
		return SymbolicTransformer{fallback: accessesToLanes(op.Accesses())}
	}
	row := GuardedRow{Guards: []Guard{{Kind: GuardSourceAvailable, Term: Term{Path: op.SourcePath.Clone()}}}}
	var fallback []state.LaneID
	for _, lane := range r.order {
		adapter := r.adapters[lane]
		if adapter == nil {
			fallback = append(fallback, lane)
			continue
		}
		effects, ok := adapter.LiftPathAssignment(op)
		if !ok {
			fallback = append(fallback, lane)
			continue
		}
		row.Effects = append(row.Effects, effects...)
	}
	sort.SliceStable(row.Effects, func(i, j int) bool { return row.Effects[i].Phase < row.Effects[j].Phase })
	return SymbolicTransformer{rows: []GuardedRow{row}, fallback: fallback, operation: op, executable: len(fallback) == 0}
}

// Execute applies the compiled path-assignment operation without calling the
// production fact applicator. It is intentionally fail-closed. The admitted
// slice uses exported State operations for subtree invalidation, alias writes,
// equality, typestate canonicalization, and user-lattice propagation.
//
// Root-origin invalidation, heap-member invalidation, and copying source static
// descendants currently live behind unexported factapply helpers. Execute
// rejects states where those helpers could matter rather than duplicating
// their behavior in this POC.
func (t SymbolicTransformer) Execute(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources factapply.FactsNodeTransferConfig,
	in state.State,
) (state.State, bool) {
	if !t.executable || t.Contextual() || resolver == nil || sources.Sources == nil {
		return in, false
	}
	op := t.operation
	if op.Point != ctx.Point || !op.HasSourcePath || op.Target.IsEmpty() || len(op.Target.Segments) == 0 {
		return in, false
	}
	value, ok := sources.Sources.ValueOfSource(ctx.Point, op.Source, in, func(_ cfg.Point) state.State { return in })
	if !ok || !exactExecutableState(ctx.Registry, resolver.KeySpace(), in) {
		return in, false
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, op.Target).VisibleStateKey()
	if !ok {
		return in, false
	}
	sourceKey, ok := visibility.AddressAt(resolver, ctx.Point, op.SourcePath).VisibleStateKey()
	if !ok {
		return in, false
	}

	// Capture aliases before invalidation removes the equality proofs that
	// justify them. This is the same transaction boundary as production.
	targets := append([]pathaddr.StateKey{targetKey}, in.EquivalentStateKeys(resolver.KeySpace(), targetKey)...)
	out := in
	for _, target := range dedupeStateKeys(targets) {
		var valid bool
		out, valid = out.InvalidatePathKeySubtree(resolver.KeySpace(), target.PathKey())
		if !valid {
			return in, false
		}
	}
	edit := out.EditPathEvidence(ctx.Registry)
	for _, target := range dedupeStateKeys(targets) {
		local, valid := resolver.KeySpace().FromPathKey(target.PathKey())
		if !valid {
			return in, false
		}
		edit.WriteLocalPathKey(local, value)
		if canonical, valid := resolver.KeySpace().FieldCanonical(local); valid {
			edit.WriteLocalPathKey(canonical, value)
		}
	}
	out = edit.Done()
	if targetKey != sourceKey {
		target, targetOK := visibility.KeyspaceKeyFromStateKey(resolver, targetKey)
		source, sourceOK := visibility.KeyspaceKeyFromStateKey(resolver, sourceKey)
		if !targetOK || !sourceOK {
			return in, false
		}
		out = out.AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: target, Other: source})
		out = out.CanonicalizeTypestateResources(resolver.KeySpace())
	}
	out = out.PropagateUserAssignmentFrom(ctx.Registry, resolver.KeySpace(), targetKey, in, sourceKey)
	return out, true
}

func exactExecutableState(reg *axis.Registry, ks *keyspace.KeySpace, in state.State) bool {
	if reg == nil || ks == nil {
		return false
	}
	values := in.ValuesSnapshot()
	if values.Top || len(values.Values) != 0 {
		return false
	}
	heap := in.HeapTableObjectsSnapshot()
	if heap.Top || len(heap.Objects) != 0 {
		return false
	}
	static := in.PathStaticMembersSnapshot(ks)
	return len(static.Members) == 0
}

func dedupeStateKeys(keys []pathaddr.StateKey) []pathaddr.StateKey {
	seen := make(map[pathaddr.StateKey]struct{}, len(keys))
	out := keys[:0]
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func accessesToLanes(accesses []LaneAccess) []state.LaneID {
	out := make([]state.LaneID, len(accesses))
	for i, access := range accesses {
		out[i] = access.Lane
	}
	return out
}

type Bindings struct {
	Roots  map[symbol.ID]pathdom.Path
	Values map[pathdom.PathKey]product.Value
}

type BoundEffect struct {
	Lane   state.LaneID
	Kind   EffectKind
	Phase  uint8
	Target pathdom.Path
	Source pathdom.Path
	Value  product.Value
}

type BoundRow struct {
	Effects []BoundEffect
}

// SubstituteTerms is a cheap, allocation-bounded structural substitution used
// only to measure the optimistic term model. Missing
// source evidence makes the guarded row infeasible; malformed rebasing aborts
// atomically rather than returning a partial row. The result is not executable
// concrete State semantics and must never be published as analysis output.
func (t SymbolicTransformer) SubstituteTerms(bindings Bindings) ([]BoundRow, bool) {
	if !t.TermComplete() {
		return nil, false
	}
	rows := make([]BoundRow, 0, len(t.rows))
	for _, row := range t.rows {
		feasible := true
		for _, guard := range row.Guards {
			if guard.Kind != GuardSourceAvailable {
				return nil, false
			}
			if _, ok := bindings.Values[guard.Term.Path.Key()]; !ok {
				feasible = false
				break
			}
		}
		if !feasible {
			continue
		}
		bound := BoundRow{Effects: make([]BoundEffect, 0, len(row.Effects))}
		for _, effect := range row.Effects {
			target, ok := bindRoot(effect.Target.Path, bindings.Roots)
			if !ok {
				return nil, false
			}
			source, ok := bindRoot(effect.Source.Path, bindings.Roots)
			if !ok {
				return nil, false
			}
			bound.Effects = append(bound.Effects, BoundEffect{
				Lane: effect.Lane, Kind: effect.Kind, Phase: effect.Phase, Target: target, Source: source,
				Value: bindings.Values[effect.Source.Path.Key()],
			})
		}
		rows = append(rows, bound)
	}
	return rows, true
}

func bindRoot(term pathdom.Path, roots map[symbol.ID]pathdom.Path) (pathdom.Path, bool) {
	root, ok := roots[term.Symbol]
	if !ok || root.IsEmpty() {
		return pathdom.Path{}, false
	}
	out := root.Clone()
	out.Segments = append(out.Segments, term.Segments...)
	return out, true
}

func sortEffects(effects []BoundEffect) {
	sort.Slice(effects, func(i, j int) bool {
		if effects[i].Lane != effects[j].Lane {
			return effects[i].Lane < effects[j].Lane
		}
		if effects[i].Kind != effects[j].Kind {
			return effects[i].Kind < effects[j].Kind
		}
		return effects[i].Target.Key() < effects[j].Target.Key()
	})
}
