package linkexecutionplan

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
)

// BoundEdge is the authenticated state projection of one module-call edge.
//
// The point handles, graph, layout, directory, transition, and generation are
// retained together so a caller cannot detach the state ordinals from the
// authorities that issued them.  There is intentionally no constructor from
// point or state ordinals: NewBoundEdge is the only way to issue one.
type BoundEdge struct {
	graph     *equation.Graph
	layout    contextfiber.Layout
	directory executioncontext.Directory

	sourcePoint, targetPoint     equation.Point
	sourceOrdinal, targetOrdinal contextfiber.PointOrdinal
	sourceContext, targetContext contextfiber.ContextOrdinal
	from, to                     contextfiber.StateOrdinal

	transition modulecomposition.ModuleCallTransition
	generation modulecomposition.InitGeneration

	available bool
}

// NewBoundEdge authenticates one module-call edge against the exact equation
// Graph, compact Layout, and sealed execution-context Directory that will
// execute it.  The transition must be the exact Directory transition and its
// GenerationID must join the supplied target InitGeneration.  Endpoint
// points must be graph-owned mounted points whose owners agree with the
// transition's source module and generation's target module respectively.
func NewBoundEdge(
	graph *equation.Graph,
	layout contextfiber.Layout,
	directory executioncontext.Directory,
	source, target equation.Point,
	transition modulecomposition.ModuleCallTransition,
	generation modulecomposition.InitGeneration,
) (BoundEdge, bool) {
	if graph == nil || !graph.OwnsPoint(source) || !graph.OwnsPoint(target) ||
		!layout.Available() || !directory.Available() ||
		layout.Graph() != graph ||
		layout.PointCount() != graph.PointCount() ||
		!transition.Available() || !generation.Available() {
		return BoundEdge{}, false
	}
	if !contextLayoutDirectoryMatch(layout, directory) ||
		transition.LinkID() != directory.LinkID() ||
		generation.LinkID() != directory.LinkID() ||
		transition.CacheIngressID() != generation.CacheIngressID() ||
		transition.GenerationID() != generation.ID() {
		return BoundEdge{}, false
	}

	sourceIndex, sourceIndexOK := graph.PointIndex(source)
	targetIndex, targetIndexOK := graph.PointIndex(target)
	if !sourceIndexOK || !targetIndexOK {
		return BoundEdge{}, false
	}
	sourceOrdinal := contextfiber.PointOrdinal(sourceIndex)
	targetOrdinal := contextfiber.PointOrdinal(targetIndex)
	sourceOwner, sourceOwnerOK := layout.PointOwnerAt(sourceOrdinal)
	targetOwner, targetOwnerOK := layout.PointOwnerAt(targetOrdinal)
	if !sourceOwnerOK || !targetOwnerOK || !sourceOwner.Mounted() || !targetOwner.Mounted() ||
		sourceOwner.ModuleKey() != transition.SourceModuleKey() ||
		targetOwner.ModuleKey() != generation.ModuleKey() {
		return BoundEdge{}, false
	}

	canonicalTransition, transitionOK := directory.Transition(transition.FromContextID(), transition.ToContextID())
	if !transitionOK || canonicalTransition.ID() != transition.TransitionID() ||
		canonicalTransition.LinkID() != transition.LinkID() ||
		canonicalTransition.FromContextID() != transition.FromContextID() ||
		canonicalTransition.ToContextID() != transition.ToContextID() {
		return BoundEdge{}, false
	}
	sourceContext, sourceContextOK := contextOrdinal(layout, transition.FromContextID())
	targetContext, targetContextOK := contextOrdinal(layout, transition.ToContextID())
	if !sourceContextOK || !targetContextOK {
		return BoundEdge{}, false
	}
	sourceContextModule, sourceContextModuleOK := layout.ContextModuleKey(sourceContext)
	targetContextModule, targetContextModuleOK := layout.ContextModuleKey(targetContext)
	if !sourceContextModuleOK || !targetContextModuleOK ||
		sourceContextModule != sourceOwner.ModuleKey() ||
		targetContextModule != targetOwner.ModuleKey() {
		return BoundEdge{}, false
	}

	from, fromOK := layout.Lookup(sourceContext, sourceOrdinal)
	to, toOK := layout.Lookup(targetContext, targetOrdinal)
	if !fromOK || !toOK {
		return BoundEdge{}, false
	}
	edge := BoundEdge{
		graph: graph, layout: layout, directory: directory,
		sourcePoint: source, targetPoint: target,
		sourceOrdinal: sourceOrdinal, targetOrdinal: targetOrdinal,
		sourceContext: sourceContext, targetContext: targetContext,
		from: from, to: to,
		transition: transition, generation: generation,
		available:  true,
	}
	return edge, true
}

// Available reports whether this edge carries a complete, internally
// authenticated binding.  NewBoundEdge is the sole issuer and seals the
// verdict; the zero BoundEdge is unavailable.
func (edge BoundEdge) Available() bool { return edge.available }

// From returns the source compact state row.
func (edge BoundEdge) From() contextfiber.StateOrdinal {
	if !edge.Available() {
		return 0
	}
	return edge.from
}

// To returns the target compact state row.
func (edge BoundEdge) To() contextfiber.StateOrdinal {
	if !edge.Available() {
		return 0
	}
	return edge.to
}

// SourcePoint returns the exact graph-owned source point.
func (edge BoundEdge) SourcePoint() equation.Point {
	if !edge.Available() {
		return equation.Point{}
	}
	return edge.sourcePoint
}

// TargetPoint returns the exact graph-owned target point.
func (edge BoundEdge) TargetPoint() equation.Point {
	if !edge.Available() {
		return equation.Point{}
	}
	return edge.targetPoint
}

// SourceContext returns the exact source context ordinal named by the
// transition.
func (edge BoundEdge) SourceContext() (contextfiber.ContextOrdinal, bool) {
	if !edge.Available() {
		return 0, false
	}
	return edge.sourceContext, true
}

// TargetContext returns the exact target context ordinal named by the
// transition.
func (edge BoundEdge) TargetContext() (contextfiber.ContextOrdinal, bool) {
	if !edge.Available() {
		return 0, false
	}
	return edge.targetContext, true
}

// Transition returns the canonical module-call transition witness.
func (edge BoundEdge) Transition() modulecomposition.ModuleCallTransition {
	if !edge.Available() {
		return modulecomposition.ModuleCallTransition{}
	}
	return edge.transition
}

// Generation returns the matching target initialization-generation witness.
func (edge BoundEdge) Generation() modulecomposition.InitGeneration {
	if !edge.Available() {
		return modulecomposition.InitGeneration{}
	}
	return edge.generation
}

func (edge BoundEdge) pointPair() pointPair {
	return pointPair{from: edge.sourceOrdinal, to: edge.targetOrdinal}
}

func (edge BoundEdge) stateEdge() StateEdge {
	return StateEdge{
		from: edge.from, to: edge.to,
		sourcePoint: edge.sourceOrdinal, targetPoint: edge.targetOrdinal,
		sourceContext: edge.sourceContext, targetContext: edge.targetContext,
		sourceContextOK: true, targetContextOK: true,
		transitionID: edge.transition.ID(), generationID: edge.generation.ID(),
	}
}

// boundEdgeBelongsTo proves that an already-issued edge is for these exact
// graph/layout/directory authorities.  The graph pointer is intentionally an
// identity check; equal Point keys in another Graph are not interchangeable.
func (edge BoundEdge) boundEdgeBelongsTo(graph *equation.Graph, layout contextfiber.Layout, directory executioncontext.Directory) bool {
	if !edge.Available() || edge.graph != graph || !layoutDirectoryEquivalent(edge.layout, edge.directory, layout, directory) ||
		edge.layout.PointCount() != layout.PointCount() || edge.layout.StateCount() != layout.StateCount() {
		return false
	}
	canonical, ok := directory.Transition(edge.transition.FromContextID(), edge.transition.ToContextID())
	return ok && canonical.ID() == edge.transition.TransitionID() &&
		canonical.LinkID() == edge.transition.LinkID() &&
		canonical.FromContextID() == edge.transition.FromContextID() &&
		canonical.ToContextID() == edge.transition.ToContextID()
}

func contextOrdinal(layout contextfiber.Layout, id identity.ContentID) (contextfiber.ContextOrdinal, bool) {
	if !layout.Available() || !id.Available() {
		return 0, false
	}
	for index := 0; index < layout.ContextCount(); index++ {
		contextID, contextOK := layout.ContextID(contextfiber.ContextOrdinal(index))
		if contextOK && contextID == id {
			return contextfiber.ContextOrdinal(index), true
		}
	}
	return 0, false
}

func contextLayoutDirectoryMatch(layout contextfiber.Layout, directory executioncontext.Directory) bool {
	if !layout.Available() || !directory.Available() || layout.ContextCount() != directory.ContextCount() {
		return false
	}
	if layout.ContextCount() == 0 || layout.PointCount() == 0 || layout.Generation() == 0 {
		return false
	}
	for index := 0; index < layout.ContextCount(); index++ {
		ordinal := contextfiber.ContextOrdinal(index)
		contextID, contextIDOK := layout.ContextID(ordinal)
		row, rowOK := directory.ContextAt(index)
		module, moduleOK := layout.ContextModuleKey(ordinal)
		if !contextIDOK || !rowOK || !row.Available() || row.ID() != contextID ||
			row.LinkID() != directory.LinkID() || !moduleOK || module != row.ModuleKey() {
			return false
		}
	}
	return true
}

func layoutDirectoryEquivalent(leftLayout contextfiber.Layout, leftDirectory executioncontext.Directory, rightLayout contextfiber.Layout, rightDirectory executioncontext.Directory) bool {
	if leftLayout.Graph() != rightLayout.Graph() ||
		!contextLayoutDirectoryMatch(leftLayout, leftDirectory) || !contextLayoutDirectoryMatch(rightLayout, rightDirectory) ||
		leftDirectory.LinkID() != rightDirectory.LinkID() || leftLayout.Generation() != rightLayout.Generation() ||
		leftLayout.PointCount() != rightLayout.PointCount() || leftLayout.ContextCount() != rightLayout.ContextCount() {
		return false
	}
	for point := 0; point < leftLayout.PointCount(); point++ {
		leftOwner, leftOK := leftLayout.PointOwnerAt(contextfiber.PointOrdinal(point))
		rightOwner, rightOK := rightLayout.PointOwnerAt(contextfiber.PointOrdinal(point))
		if !leftOK || !rightOK || leftOwner != rightOwner {
			return false
		}
	}
	for context := 0; context < leftLayout.ContextCount(); context++ {
		ordinal := contextfiber.ContextOrdinal(context)
		leftID, leftIDOK := leftLayout.ContextID(ordinal)
		rightID, rightIDOK := rightLayout.ContextID(ordinal)
		leftModule, leftModuleOK := leftLayout.ContextModuleKey(ordinal)
		rightModule, rightModuleOK := rightLayout.ContextModuleKey(ordinal)
		leftRow, leftRowOK := leftDirectory.ContextAt(context)
		rightRow, rightRowOK := rightDirectory.ContextAt(context)
		if !leftIDOK || !rightIDOK || leftID != rightID || !leftModuleOK || !rightModuleOK || leftModule != rightModule ||
			!leftRowOK || !rightRowOK || leftRow != rightRow {
			return false
		}
	}
	if leftDirectory.RootCount() != rightDirectory.RootCount() || leftDirectory.TransitionCount() != rightDirectory.TransitionCount() {
		return false
	}
	for root := 0; root < leftDirectory.RootCount(); root++ {
		leftRoot, leftRootOK := leftDirectory.RootAt(root)
		rightRoot, rightRootOK := rightDirectory.RootAt(root)
		if !leftRootOK || !rightRootOK || leftRoot != rightRoot {
			return false
		}
	}
	for transition := 0; transition < leftDirectory.TransitionCount(); transition++ {
		leftTransition, leftTransitionOK := leftDirectory.TransitionAt(transition)
		rightTransition, rightTransitionOK := rightDirectory.TransitionAt(transition)
		if !leftTransitionOK || !rightTransitionOK || leftTransition != rightTransition {
			return false
		}
	}
	return true
}
