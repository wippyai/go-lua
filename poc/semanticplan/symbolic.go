package semanticplan

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
	rows     []GuardedRow
	fallback []state.LaneID
	// executable remains false in Stage 1: the term model has no independent
	// concrete-State interpreter for the unexported invalidation/heap helpers.
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
	return SymbolicTransformer{rows: []GuardedRow{row}, fallback: fallback, executable: false}
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
