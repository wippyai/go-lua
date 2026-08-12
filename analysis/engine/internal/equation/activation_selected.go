package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
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
		namespace, namespaced := memberNamespace(member)
		alpha, alphaBound := template.decisionAlpha(binding.key, namespace)
		if !namespaced || !alphaBound || len(template.instances) != 0 {
			return nil, false
		}
		for _, row := range template.value.FactorEdges {
			edge, ok := materializeSelectedStructuralFactorEdge(topology.source, base, template, binding.key, namespace, alpha, acceptedMember.Premise(), template.key, row)
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

// ownsBaseGraph rejects a selected Graph before edge materialization. The
// graph's private storage cannot be assembled outside this package; exact
// endpoint ownership is then proven by selectedStructuralBasePoint's direct
// base.pointAt lookup for each requested external Site or sealed Port. This
// avoids rebuilding a Point map or scanning every base Point per frontier.
func (topology *Topology) ownsBaseGraph(graph *Graph) bool {
	if topology == nil || graph == nil || topology.source == nil || topology.base.Batch == nil || !topology.base.Batch.Sealed() ||
		!graph.OwnsComposition(topology.source) || graph.PointCount() != len(topology.base.Points) ||
		graph.GroupCount() != len(topology.base.Groups) || graph.QueryCount() != len(topology.base.Queries) ||
		graph.EnvironmentEdgeTotal() != len(topology.base.EnvironmentEdges) || graph.FactorEdgeTotal() != len(topology.base.FactorEdges) ||
		len(graph.points) != len(graph.pointAt) {
		return false
	}
	return true
}

func structuralFactorOnlyTemplate(template sealedTemplate) bool {
	return template.source != nil && template.batch != nil && template.batch.Sealed() && template.key.Available() &&
		len(template.instances) == 0 && len(template.points) == 0 && len(template.value.Rules) == 0 &&
		len(template.value.Points) == 0 && len(template.value.Groups) == 0 && len(template.value.FactorEdges) != 0 &&
		len(template.value.Summaries) == 0 && len(template.value.WeakTargets) == 0
}

// materializeSelectedStructuralFactorEdge is the structural-only lowering of
// sealedTemplate.appendMember. It deliberately binds the immutable template
// rows directly: no disposable TopologySpec, no appendMember path, and no
// linear resolveExternalPoint scan appear on the selected hot path.
func materializeSelectedStructuralFactorEdge(source *composition.Composition, base *Graph, template sealedTemplate, binding, namespace composition.Key, alpha decisionAlpha, premise Expr, provenanceRow composition.Key, row FragmentFactorEdge) (SelectedStructuralFactorEdge, bool) {
	if source == nil || base == nil || !template.ambient.Available() || !binding.Available() || !namespace.Available() || alpha == nil || !premise.Available() || !provenanceRow.Available() || !row.Factor.Available() {
		return SelectedStructuralFactorEdge{}, false
	}
	if _, known := source.FactorIndex(row.Factor); !known {
		return SelectedStructuralFactorEdge{}, false
	}
	resolvedSource, sourceOK := selectedStructuralSourcePoint(base, template, row)
	resolvedTarget, targetOK := selectedStructuralTargetPoint(base, template, row)
	if !sourceOK || !targetOK {
		return SelectedStructuralFactorEdge{}, false
	}
	reindex, reindexed := boundReindex(row.Reindex, resolvedSource.resolved, resolvedTarget.resolved, template.ambient, alpha)
	if !reindexed {
		return SelectedStructuralFactorEdge{}, false
	}
	pre, post := row.Pre, row.Post
	if resolvedSource.resolved.local {
		var bound bool
		pre, bound = boundExpr(pre, alpha)
		if !bound {
			return SelectedStructuralFactorEdge{}, false
		}
	}
	if resolvedTarget.resolved.local {
		var bound bool
		post, bound = boundExpr(post, alpha)
		if !bound {
			return SelectedStructuralFactorEdge{}, false
		}
	}
	// This is the same attachment law as sealedTemplate.appendMember: premise
	// evidence belongs at the endpoint whose exact scope owns it.
	var attached bool
	if validScopedExpr(premise, resolvedSource.resolved.scope) {
		pre, attached = AndExpr(premise, pre)
	} else if validScopedExpr(premise, resolvedTarget.resolved.scope) {
		post, attached = AndExpr(premise, post)
	}
	if !attached {
		return SelectedStructuralFactorEdge{}, false
	}
	provenance, provenanced := boundProvenance(row.Provenance, binding, namespace, provenanceRow)
	if !provenanced {
		return SelectedStructuralFactorEdge{}, false
	}
	input := BoundaryInput(resolvedSource.resolved.site, resolvedTarget.resolved.site, provenance, pre, reindex, post)
	if !input.Available() {
		return SelectedStructuralFactorEdge{}, false
	}
	key, keyed := identityKey("analysis/engine/equation/factor-edge", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, input.Key()) && writePoint(writer, resolvedTarget.point) && writeKey(writer, row.Factor)
	})
	if !keyed || !key.Available() {
		return SelectedStructuralFactorEdge{}, false
	}
	input.point = resolvedSource.point
	return SelectedStructuralFactorEdge{key: key, source: resolvedSource.point, target: resolvedTarget.point, input: input, factor: row.Factor}, true
}

type selectedStructuralResolvedPoint struct {
	point    Point
	resolved templateResolvedPoint
}

func selectedStructuralSourcePoint(base *Graph, template sealedTemplate, edge FragmentFactorEdge) (selectedStructuralResolvedPoint, bool) {
	if edge.ExternalSource.Available() {
		return selectedStructuralExternalPoint(base, edge.ExternalSource)
	}
	return selectedStructuralPortPoint(base, template, edge.Source, PortImport)
}

func selectedStructuralTargetPoint(base *Graph, template sealedTemplate, edge FragmentFactorEdge) (selectedStructuralResolvedPoint, bool) {
	if edge.ExternalTarget.Available() {
		return selectedStructuralExternalPoint(base, edge.ExternalTarget)
	}
	return selectedStructuralPortPoint(base, template, edge.Target, PortExport)
}

func selectedStructuralExternalPoint(base *Graph, site Site) (selectedStructuralResolvedPoint, bool) {
	expected, issued := derivePoint(site)
	point, ref, found := selectedStructuralBasePoint(base, expected)
	if !issued || !found || !point.Site().Same(site) || !sameScope(point.Scope(), site.Scope()) {
		return selectedStructuralResolvedPoint{}, false
	}
	return selectedStructuralResolvedPoint{point: point, resolved: templateResolvedPoint{ref: ref, site: point.Site(), scope: point.Scope(), rawScope: point.Scope()}}, true
}

func selectedStructuralPortPoint(base *Graph, template sealedTemplate, value FragmentPoint, required PortMode) (selectedStructuralResolvedPoint, bool) {
	if value.Local != 0 || !value.Port.Available() {
		return selectedStructuralResolvedPoint{}, false
	}
	port, found := template.ports[value.Port]
	if !found || required == PortImport && !port.mode.imports() || required == PortExport && !port.mode.exports() || port.base == 0 || !port.point.Available() || !template.ambient.Available() || !sameScope(port.point.Scope(), template.ambient) {
		return selectedStructuralResolvedPoint{}, false
	}
	point, ref, owned := selectedStructuralBasePoint(base, port.point)
	if !owned || !point.Site().Same(port.point.Site()) || !sameScope(point.Scope(), port.point.Scope()) {
		return selectedStructuralResolvedPoint{}, false
	}
	return selectedStructuralResolvedPoint{point: point, resolved: templateResolvedPoint{ref: ref, site: point.Site(), scope: template.ambient, rawScope: EmptyScope()}}, true
}

// selectedStructuralBasePoint converts a sealed Site/Port Point into the
// graph-owned capability needed by the runtime. It consults Graph.pointAt
// directly, so selected materialization never rebuilds an O(P) lookup map.
func selectedStructuralBasePoint(base *Graph, expected Point) (Point, PointRef, bool) {
	if base == nil || !base.valid() || !expected.Available() {
		return Point{}, 0, false
	}
	node, found := base.pointAt[expected.key]
	if !found || node < 0 || int(node) >= len(base.points) {
		return Point{}, 0, false
	}
	point := base.points[node]
	if !base.OwnsPoint(point) || point.key != expected.key || !point.Site().Same(expected.Site()) || !sameScope(point.Scope(), expected.Scope()) {
		return Point{}, 0, false
	}
	return point, PointAt(int(node)), true
}
