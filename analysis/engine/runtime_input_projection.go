package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
)

// runtimeInputProjection is the sealed address of one dense Group input port.
// The source point, source Input identity, source provenance and composition
// generation are all issued by the equation Graph at seal.  The read factor
// is copied from the generated CompiledRule read plan when that port is used
// by a generated member.  Runtime therefore consumes one owner-issued source
// address and one already-lowered carrier transport; it cannot rediscover a
// point, factor, or generation from the live graph.
//
// The projection intentionally does not retain a PointState. State is
// contextual and changes per epoch. The executor combines this immutable
// source point with the epoch's one state address, then applies the same
// sealed transport. There is one point plane and one transport boundary, not
// a second generated-input state representation.
type runtimeInputProjection struct {
	sourcePoint       int
	sourceKey         composition.Key
	sourceInputKey    composition.Key
	sourceProvenance  composition.Key
	sourceComposition composition.ID
	sourceGeneration  identity.Generation
	transport         runtimeInput
}

// sealRuntimeInputProjection converts the already constructed Group input
// rows into dense graph-point ordinals. The Group row is the owner-issued
// predecessor-State geometry; this function resolves its address exactly once
// in the committed graph and attaches the transport assembled for that row.
// Factor identity remains solely on each CompiledRule read. PointState already
// carries the complete opaque Factor product, so copying Factor tuples here
// would create a second authority without changing transport.
//
// No input is omitted, reordered, or recovered by a fallback. Arity and
// ownership are closed at this boundary, so a foreign or holey relation
// refuses during assembly instead of being rediscovered at runtime.
func sealRuntimeInputProjection(graph *equation.Graph, plan *linkexecutionplan.LinkExecutionPlan, group equation.GroupNode, transports []runtimeInput) ([]runtimeInputProjection, bool) {
	if graph == nil || !graph.OwnsGroup(group) || !group.Key().Available() || group.InputCount() < 0 || len(transports) != group.InputCount() || !graph.CompositionID().Available() {
		return nil, false
	}
	generation := identity.Generation(0)
	if plan != nil {
		if !plan.Available() || plan.Graph() != graph || !plan.Generation().Available() {
			return nil, false
		}
		generation = plan.Generation()
	}
	projection := make([]runtimeInputProjection, group.InputCount())
	for index := range projection {
		input, inputOK := group.InputAt(index)
		if !inputOK || !input.Point().Available() || !input.Key().Available() || !input.Provenance().Available() || !transports[index].valid() || transports[index].key != input.Key() || transports[index].provenance != input.Provenance() {
			return nil, false
		}
		pointIndex, indexed := graph.PointIndex(input.Point())
		if !indexed || pointIndex < 0 || pointIndex >= graph.PointCount() {
			return nil, false
		}
		point, pointOK := graph.PointAt(schedule.Node(pointIndex))
		if !pointOK || !graph.OwnsPoint(point) || !point.Available() || point.Key() != input.Point().Key() {
			return nil, false
		}
		projection[index] = runtimeInputProjection{
			sourcePoint:       pointIndex,
			sourceKey:         point.Key(),
			sourceInputKey:    input.Key(),
			sourceProvenance:  input.Provenance(),
			sourceComposition: graph.CompositionID(),
			sourceGeneration:  generation,
			transport:         transports[index],
		}
		if !projection[index].validFor(graph, generation) {
			return nil, false
		}
	}
	return projection, true
}

// validFor authenticates the immutable source projection against the exact
// graph and generation that issued it. A copied projection from a different
// mounted revision is therefore refused before transport, even when its
// point ordinal and semantic key happen to match.
func (projection runtimeInputProjection) validFor(graph *equation.Graph, generation identity.Generation) bool {
	if graph == nil || projection.sourcePoint < 0 || projection.sourcePoint >= graph.PointCount() || !projection.sourceKey.Available() || !projection.sourceInputKey.Available() || !projection.sourceProvenance.Available() || projection.sourceComposition != graph.CompositionID() || !projection.validTransport() {
		return false
	}
	if projection.sourceGeneration != generation {
		return false
	}
	point, pointOK := graph.PointAt(schedule.Node(projection.sourcePoint))
	return pointOK && graph.OwnsPoint(point) && point.Available() && point.Key() == projection.sourceKey
}

// validTransport is the post-seal, graph-free runtime check. It checks only
// immutable handles and the already-lowered carrier boundary; no graph point,
// source row, generation, or factor directory is looked up here.
func (projection runtimeInputProjection) validTransport() bool {
	if !projection.sourceKey.Available() || !projection.sourceInputKey.Available() || !projection.sourceProvenance.Available() || !projection.transport.valid() || projection.transport.key != projection.sourceInputKey || projection.transport.provenance != projection.sourceProvenance {
		return false
	}
	return true
}

func (producer *runtimeProducer) inputProjectionAt(index int) (runtimeInputProjection, bool) {
	if producer == nil || index < 0 || index >= len(producer.inputProjection) {
		return runtimeInputProjection{}, false
	}
	projection := producer.inputProjection[index]
	if !projection.validTransport() {
		return runtimeInputProjection{}, false
	}
	return projection, true
}
