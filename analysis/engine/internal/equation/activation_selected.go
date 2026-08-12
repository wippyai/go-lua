package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// SelectedStructuralFactorEdge is one accepted structural-only activation
// transport materialized against the immutable base points of a Topology.
// It deliberately is not a Graph edge: callers receive the exact issued
// endpoint identities and boundary relation, but no mutable graph ownership
// or candidate catalog.
type SelectedStructuralFactorEdge struct {
	key    composition.Key
	source Point
	target Point
	input  Input
	factor composition.Key
}

func (edge SelectedStructuralFactorEdge) Available() bool {
	return edge.key.Available() && edge.source.Available() && edge.target.Available() && edge.input.Available() && edge.factor.Available()
}

func (edge SelectedStructuralFactorEdge) Key() composition.Key    { return edge.key }
func (edge SelectedStructuralFactorEdge) Source() Point           { return edge.source }
func (edge SelectedStructuralFactorEdge) Target() Point           { return edge.target }
func (edge SelectedStructuralFactorEdge) Input() Input            { return edge.input }
func (edge SelectedStructuralFactorEdge) Factor() composition.Key { return edge.factor }

// SelectedStructuralFactorEdges expands a canonical accepted subset into
// FactorEdge-only structural transports without copying or compiling the base
// TopologySpec.  It accepts a full accepted set or a canonical delta; every
// selected Member must resolve to a zero-payload template whose endpoints are
// already-issued Points owned by base.  Typed fragments, local points, Groups,
// summaries, and weak-target declarations fail closed and remain on the
// ordinary Graph materialization path.
func (topology *Topology) SelectedStructuralFactorEdges(base *Graph, accepted []AcceptedMember) ([]SelectedStructuralFactorEdge, bool) {
	if topology == nil || topology.source == nil || !topology.validAccepted(accepted) || !topology.ownsBaseGraph(base) {
		return nil, false
	}
	basePoints := make(map[composition.Key]Point, base.PointCount())
	for index := 0; index < base.PointCount(); index++ {
		point, ok := base.PointAt(schedule.Node(index))
		if !ok || !base.OwnsPoint(point) || !point.Available() {
			return nil, false
		}
		if _, duplicate := basePoints[point.Key()]; duplicate {
			return nil, false
		}
		basePoints[point.Key()] = point
	}
	result := make([]SelectedStructuralFactorEdge, 0)
	for _, acceptedMember := range accepted {
		member := acceptedMember.Member()
		binding, found := topology.binding(member.Binding())
		if !found || !binding.plan.data.structuralOnly() {
			return nil, false
		}
		locator, located := member.Locator()
		if !located || locator.Application != binding.application {
			return nil, false
		}
		variant, selected := binding.plan.variant(locator.Target, locator.Endpoint)
		if !selected {
			return nil, false
		}
		template, bound := variant.template.bindPrototype(binding.ports, binding.ambient)
		if !bound || !structuralFactorOnlyTemplate(template) {
			return nil, false
		}

		// Keep the base Point slice read-only and give appendMember nowhere to
		// publish any ordinary topology row.  The strict shape proof above and
		// the postcondition below make this a descriptor materializer rather
		// than a disguised copyTopologySpec/compileTopology path.
		spec := TopologySpec{Batch: topology.base.Batch, Points: topology.base.Points}
		if !binding.appendMember(&spec, member, acceptedMember.Premise()) ||
			len(spec.Rules) != 0 || len(spec.Points) != len(topology.base.Points) ||
			len(spec.Groups) != 0 || len(spec.Queries) != 0 || len(spec.EnvironmentEdges) != 0 ||
			len(spec.ActivationBindings) != 0 || len(spec.Summaries) != 0 || len(spec.WeakTargets) != 0 ||
			len(spec.FactorEdges) != len(template.value.FactorEdges) {
			return nil, false
		}
		for _, row := range spec.FactorEdges {
			edge, ok := materializeSelectedStructuralFactorEdge(topology.source, basePoints, row)
			if !ok {
				return nil, false
			}
			result = append(result, edge)
		}
	}
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left].key, result[right].key) })
	for index := 1; index < len(result); index++ {
		if result[index-1].key == result[index].key {
			return nil, false
		}
	}
	return result, true
}

// ownsBaseGraph rejects a selected Graph before edge materialization.  A
// structural-only selected member contributes at least one FactorEdge, so the
// exact base cardinality check prevents a caller from mixing descriptor
// ownership across graph generations.  The point-key pass independently
// proves that every base Site has its graph-issued Point capability.
func (topology *Topology) ownsBaseGraph(graph *Graph) bool {
	if topology == nil || graph == nil || topology.source == nil || topology.base.Batch == nil || !topology.base.Batch.Sealed() ||
		!graph.OwnsComposition(topology.source) || graph.PointCount() != len(topology.base.Points) ||
		graph.GroupCount() != len(topology.base.Groups) || graph.QueryCount() != len(topology.base.Queries) ||
		graph.EnvironmentEdgeTotal() != len(topology.base.EnvironmentEdges) || graph.FactorEdgeTotal() != len(topology.base.FactorEdges) {
		return false
	}
	expected := make(map[composition.Key]struct{}, len(topology.base.Points))
	for _, row := range topology.base.Points {
		point, ok := derivePoint(row.Site)
		if !ok || !point.Available() {
			return false
		}
		if _, duplicate := expected[point.Key()]; duplicate {
			return false
		}
		expected[point.Key()] = struct{}{}
	}
	for index := 0; index < graph.PointCount(); index++ {
		point, ok := graph.PointAt(schedule.Node(index))
		if !ok || !graph.OwnsPoint(point) {
			return false
		}
		if _, present := expected[point.Key()]; !present {
			return false
		}
	}
	return true
}

func structuralFactorOnlyTemplate(template sealedTemplate) bool {
	return template.source != nil && template.batch != nil && template.batch.Sealed() && template.key.Available() &&
		len(template.instances) == 0 && len(template.points) == 0 && len(template.value.Rules) == 0 &&
		len(template.value.Points) == 0 && len(template.value.Groups) == 0 && len(template.value.FactorEdges) != 0 &&
		len(template.value.Summaries) == 0 && len(template.value.WeakTargets) == 0
}

func materializeSelectedStructuralFactorEdge(source *composition.Composition, base map[composition.Key]Point, row FactorEdge) (SelectedStructuralFactorEdge, bool) {
	if source == nil || len(base) == 0 || !row.Input.Available() || !row.Factor.Available() {
		return SelectedStructuralFactorEdge{}, false
	}
	if _, known := source.FactorIndex(row.Factor); !known {
		return SelectedStructuralFactorEdge{}, false
	}
	sourcePoint, sourceOK := derivePoint(row.Input.Source())
	targetPoint, targetOK := derivePoint(row.Input.Target())
	baseSource, sourcePresent := base[sourcePoint.Key()]
	baseTarget, targetPresent := base[targetPoint.Key()]
	if !sourceOK || !targetOK || !sourcePoint.Available() || !targetPoint.Available() || !sourcePresent || !targetPresent ||
		baseSource.Key() != sourcePoint.Key() || baseTarget.Key() != targetPoint.Key() {
		return SelectedStructuralFactorEdge{}, false
	}
	key, keyed := identityKey("analysis/engine/equation/factor-edge", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, row.Input.Key()) && writePoint(writer, targetPoint) && writeKey(writer, row.Factor)
	})
	if !keyed || !key.Available() {
		return SelectedStructuralFactorEdge{}, false
	}
	input := row.Input
	input.point = baseSource
	return SelectedStructuralFactorEdge{key: key, source: baseSource, target: baseTarget, input: input, factor: row.Factor}, true
}
