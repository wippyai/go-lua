package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PathTypeProjector projects a path through a structural root type. Engine
// packages receive this from the language/check layer so they stay syntax-free.
type PathTypeProjector func(root typ.Type, path pathdom.Path) (typ.Type, bool)

// CovariantWiden rebuilds a covariantly-exposed object's witness type. Given the
// object's current witness type, the exposure contract type, and the source path
// segments locating the exposed sub-object under its ancestor symbol, it returns
// the ancestor witness type with every strictly-wider field widened to the
// contract and the deduplicated top segment of every widened leaf (so the engine
// can drop the precise per-field facts beneath it). It returns ok=false when no
// field widens. Engine packages receive this from the check layer to keep the
// subtype/unwrap reasoning out of the engine.
type CovariantWiden func(sourceWitness, contract typ.Type, segments []segment.Segment) (widened typ.Type, topWidenedSegments [][]segment.Segment, ok bool)

// ClosedDynamicAllValueInvariant states that every present value reachable via
// Container is a key of Table. The program layer infers these for closed
// reverse-map writer sets; fact application seeds them when the container is
// created as a fresh empty table.
type ClosedDynamicAllValueInvariant struct {
	Container pathdom.Path
	Table     pathdom.Path
}

// FactsNodeTransferConfig configures the generic fact applicator.
type FactsNodeTransferConfig struct {
	Facts                  factflow.Facts
	Sources                sourcevalue.SourceValues
	CallOutcome            callpayload.CallOutcomeProvider
	Visibility             *visibility.Resolver
	ProjectPath            PathTypeProjector
	CovariantWiden         CovariantWiden
	TypeValues             *typevalue.Cache
	ClosedDynamicAllValues []ClosedDynamicAllValueInvariant
}

// FactsEdgeTransferConfig configures the generic edge fact applicator.
type FactsEdgeTransferConfig struct {
	Facts       factflow.Facts
	Sources     sourcevalue.SourceValues
	CallOutcome callpayload.CallOutcomeProvider
	Visibility  *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
}

func activeBranchRefinementHasStrictPrefix(refinements []ActiveBranchRefinement, target pathdom.Path) bool {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return false
	}
	for _, fact := range refinements {
		if fact.targetPath.Symbol != target.Symbol || len(fact.targetPath.Segments) >= len(target.Segments) {
			continue
		}
		if branchRefinementSegmentsHavePrefix(target.Segments, fact.targetPath.Segments) {
			return true
		}
	}
	return false
}

func branchRefinementSegmentsHavePrefix(target []segment.Segment, prefix []segment.Segment) bool {
	if len(prefix) > len(target) {
		return false
	}
	for i := range prefix {
		if target[i] != prefix[i] {
			return false
		}
	}
	return true
}

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment, member/path
// assignment, descendant path invalidation, call return-slot production, and
// return-slot facts; branch-edge refinements are handled by NewFactsEdgeTransfer.
func NewFactsNodeTransfer(config FactsNodeTransferConfig) transfer.NodeTransfer {
	executor := NewConcreteNodePointExecutor(config)
	return func(ctx transfer.NodeContext, in state.State) state.State {
		return executor.Apply(ctx, in).Output
	}
}

// NewFactsEdgeTransfer returns a generic edge transfer that applies
// branch refinements for the selected branch edge, including root-origin
// narrowing recovered from descendant path refinements when flow state carries
// the structural root type.
func NewFactsEdgeTransfer(config FactsEdgeTransferConfig) transfer.EdgeTransfer {
	executor := &ConcreteBranchEdgePointExecutor{}
	return func(ctx transfer.EdgeContext, out state.State) state.State {
		return executor.Apply(ConcreteBranchEdgePointRequest{
			Context: ctx, Facts: config.Facts, Sources: config.Sources,
			CallOutcome: config.CallOutcome, Resolver: config.Visibility,
			ProjectPath: config.ProjectPath, TypeValues: config.TypeValues,
			Output: out,
		}).Output
	}
}

func tokenOf(session *cancellation.Session) *cancellation.Token {
	if session == nil {
		return nil
	}
	return session.Token()
}

func branchConditionEdgeUnreachable(
	ctx transfer.EdgeContext,
	branch BranchAlgebra,
	sources sourcevalue.SourceValues,
	in state.State,
) bool {
	if sources == nil {
		return false
	}
	condition, ok := branch.Condition()
	if !ok {
		return false
	}
	source := condition.Source()
	value, ok := sources.ValueOfSource(ctx.Edge.From, source, in, ctx.Read)
	if !ok {
		return false
	}
	if product.Equal(ctx.Registry, value, product.Bottom(ctx.Registry)) {
		return false
	}
	if condition.TruthyOnEdge(ctx.Edge.Cond) {
		return !valuerefine.CanBeTruthy(ctx.Registry, value)
	}
	return !valuerefine.CanBeFalsy(ctx.Registry, value)
}

func stateIsBottom(reg *axis.Registry, st state.State) bool {
	return state.IsBottom(reg, st)
}

func unreachableState(reg *axis.Registry) state.State {
	return state.Domain(reg).Bottom()
}

func branchTruthyEvidenceContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath)
	if !ok || product.Equal(reg, current.value, product.Bottom(reg)) {
		return false
	}
	return !valuerefine.CanBeTruthy(reg, current.value)
}

func branchFalsyEvidenceContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath)
	if !ok || product.Equal(reg, current.value, product.Bottom(reg)) {
		return false
	}
	return !valuerefine.CanBeFalsy(reg, current.value)
}
