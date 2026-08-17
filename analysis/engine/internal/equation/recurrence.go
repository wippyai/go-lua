package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// RegionView is a graph-owned view of one derived WTO region. Its ordinal is
// private to this compiled graph: it is not a durable semantic identity.
type RegionView struct {
	graph *Graph
	index int
}

// InterfaceInput identifies one exact external input occurrence of a Group
// whose output belongs to a recurrence region.
type InterfaceInput struct {
	graph *Graph
	group int
	input int
}

// interfaceRef is the compact graph-owned representation of InterfaceInput.
// RegionView reconstructs the opaque view on demand, so one interface row does
// not retain a redundant Graph pointer.
type interfaceRef struct {
	group int
	input int
}

type regionData struct {
	head   schedule.Node
	parent int

	// Points are the exact [pointBegin, pointEnd) interval in Graph.eventPoints.
	// WTO brackets make every descendant Point contiguous in that one immutable
	// row, including nested Regions. This replaces the former Region×Point
	// contains bitmap.
	pointBegin int
	pointEnd   int

	interfaceBegin           int
	interfaceEnd             int
	faceBegin                int
	faceEnd                  int
	externalBegin            int
	externalEnd              int
	backBegin                int
	backEnd                  int
	internalBegin            int
	internalEnd              int
	environmentExternalBegin int
	environmentExternalEnd   int
	environmentBackBegin     int
	environmentBackEnd       int
	factorExternalBegin      int
	factorExternalEnd        int
	factorBackBegin          int
	factorBackEnd            int
	factorInternalBegin      int
	factorInternalEnd        int
	factorBegin              int
	factorEnd                int
}

// pendingRegion is construction-only sparse membership. Its rows contain only
// actual Region relations; they are compacted into graph-owned CSR storage
// before Graph becomes observable.
type pendingRegion struct {
	interfaces          []interfaceRef
	faces               []int
	external            []int
	back                []int
	internal            []int
	environmentExternal []int
	environmentBack     []int
	factorExternal      []int
	factorBack          []int
	factorInternal      []int
	factors             []composition.Key
}

func (graph *Graph) deriveRegions() bool {
	if !graph.valid() || len(graph.regions) != 0 || len(graph.regionInterfaces) != 0 || len(graph.regionFaces) != 0 || len(graph.regionExternal) != 0 || len(graph.regionBack) != 0 || len(graph.regionInternal) != 0 || len(graph.regionEnvironmentExternal) != 0 || len(graph.regionEnvironmentBack) != 0 || len(graph.regionFactorExternal) != 0 || len(graph.regionFactorBack) != 0 || len(graph.regionFactorInternal) != 0 || len(graph.regionFactors) != 0 || len(graph.eventPoints) != len(graph.points) || len(graph.pointOrder) != len(graph.points) || len(graph.pointRegion) != len(graph.points) {
		return false
	}
	count := graph.schedule.RegionCount()
	if count == 0 {
		return true
	}
	regions := make([]regionData, count)
	pending := make([]pendingRegion, count)
	for index := range regions {
		region, ok := graph.schedule.RegionAt(index)
		if !ok || region.Head < 0 || int(region.Head) >= len(graph.points) || region.Parent < schedule.NoRegion || region.Parent >= count || region.Enter < 0 || region.Exit <= region.Enter || region.Exit >= graph.schedule.EventCount() {
			return false
		}
		begin, end := graph.eventNodes[region.Enter], graph.eventNodes[region.Exit]
		if begin < 0 || end <= begin || end > len(graph.eventPoints) || end-begin != graph.regionNodes[index] || graph.eventPoints[begin] != region.Head || graph.pointOrder[region.Head] != begin {
			return false
		}
		regions[index] = regionData{head: region.Head, parent: region.Parent, pointBegin: begin, pointEnd: end}
	}
	// Derive each Group's Factor footprint once. It is then appended only to
	// Regions in which that Group has an internal input; no Region re-scans all
	// Group members or the cold Rule catalog.
	rules := make(map[composition.Key]composition.Rule, len(graph.composition.Rules()))
	for _, rule := range graph.composition.Rules() {
		if !rule.Key.Available() {
			return false
		}
		rules[rule.Key] = rule
	}
	groupFactors := make([][]composition.Key, len(graph.groups))
	for groupIndex, group := range graph.groups {
		factors, ok := deriveGroupFactors(group, rules)
		if !ok {
			return false
		}
		groupFactors[groupIndex] = factors
	}
	// A Group belongs only to ancestors of its output's innermost Region. That
	// output relation is already a deterministic schedule fact. We therefore
	// walk actual ancestry rather than scanning every Group for every Region.
	for groupIndex, group := range graph.groups {
		output, ok := graph.pointAt[group.output.key]
		if !ok || output < 0 || int(output) >= len(graph.pointRegion) {
			return false
		}
		for regionIndex := graph.pointRegion[output]; regionIndex != schedule.NoRegion; {
			if regionIndex < 0 || regionIndex >= len(regions) || !regionContainsPoint(graph, regions[regionIndex], output) {
				return false
			}
			inside := false
			for inputIndex, input := range group.inputs {
				point, indexed := graph.pointAt[input.point.key]
				if !indexed || point < 0 || int(point) >= len(graph.pointOrder) {
					return false
				}
				if regionContainsPoint(graph, regions[regionIndex], point) {
					inside = true
				} else {
					pending[regionIndex].interfaces = append(pending[regionIndex].interfaces, interfaceRef{group: groupIndex, input: inputIndex})
					pending[regionIndex].faces = append(pending[regionIndex].faces, int(point))
				}
			}
			if group.environmentInput.Available() {
				point, indexed := graph.pointAt[group.environmentInput.point.key]
				if !indexed || point < 0 || int(point) >= len(graph.pointOrder) {
					return false
				}
				if regionContainsPoint(graph, regions[regionIndex], point) {
					inside = true
				} else {
					pending[regionIndex].faces = append(pending[regionIndex].faces, int(point))
				}
			}
			if output == regions[regionIndex].head {
				if inside {
					pending[regionIndex].back = append(pending[regionIndex].back, groupIndex)
				} else {
					pending[regionIndex].external = append(pending[regionIndex].external, groupIndex)
				}
			}
			if inside {
				pending[regionIndex].internal = append(pending[regionIndex].internal, groupIndex)
				pending[regionIndex].factors = append(pending[regionIndex].factors, groupFactors[groupIndex]...)
			}
			regionIndex = regions[regionIndex].parent
		}
	}
	// Structural environment edges have no Group producer row, but they still
	// contribute interfaces and head ingress/back classification to the same
	// WTO regions.
	for edgeIndex, edge := range graph.environments {
		if edge.transportOnly {
			continue
		}
		target, targetOK := graph.pointAt[edge.target.key]
		source, sourceOK := graph.pointAt[edge.input.point.key]
		if !targetOK || !sourceOK {
			return false
		}
		for regionIndex := graph.pointRegion[target]; regionIndex != schedule.NoRegion; {
			if regionIndex < 0 || regionIndex >= len(regions) || !regionContainsPoint(graph, regions[regionIndex], target) {
				return false
			}
			if regionContainsPoint(graph, regions[regionIndex], source) {
				if target == regions[regionIndex].head {
					pending[regionIndex].environmentBack = append(pending[regionIndex].environmentBack, edgeIndex)
				}
			} else {
				pending[regionIndex].faces = append(pending[regionIndex].faces, int(source))
				if target == regions[regionIndex].head {
					pending[regionIndex].environmentExternal = append(pending[regionIndex].environmentExternal, edgeIndex)
				}
			}
			regionIndex = regions[regionIndex].parent
		}
	}
	for edgeIndex, edge := range graph.factorEdges {
		target, targetOK := graph.pointAt[edge.target.key]
		source, sourceOK := graph.pointAt[edge.input.point.key]
		if !targetOK || !sourceOK || !edge.factor.Available() {
			return false
		}
		for regionIndex := graph.pointRegion[target]; regionIndex != schedule.NoRegion; {
			if regionIndex < 0 || regionIndex >= len(regions) || !regionContainsPoint(graph, regions[regionIndex], target) {
				return false
			}
			inside := regionContainsPoint(graph, regions[regionIndex], source)
			if inside {
				pending[regionIndex].factors = append(pending[regionIndex].factors, edge.factor)
				pending[regionIndex].factorInternal = append(pending[regionIndex].factorInternal, edgeIndex)
				if target == regions[regionIndex].head {
					pending[regionIndex].factorBack = append(pending[regionIndex].factorBack, edgeIndex)
				}
			} else {
				pending[regionIndex].faces = append(pending[regionIndex].faces, int(source))
				if target == regions[regionIndex].head {
					pending[regionIndex].factorExternal = append(pending[regionIndex].factorExternal, edgeIndex)
				}
			}
			regionIndex = regions[regionIndex].parent
		}
	}
	for index := range regions {
		data, staged := &regions[index], &pending[index]
		if !compactRegionFactors(staged) {
			return false
		}
		data.interfaceBegin = len(graph.regionInterfaces)
		graph.regionInterfaces = append(graph.regionInterfaces, staged.interfaces...)
		data.interfaceEnd = len(graph.regionInterfaces)
		data.faceBegin = len(graph.regionFaces)
		graph.regionFaces = append(graph.regionFaces, staged.faces...)
		data.faceEnd = len(graph.regionFaces)
		data.externalBegin = len(graph.regionExternal)
		graph.regionExternal = append(graph.regionExternal, staged.external...)
		data.externalEnd = len(graph.regionExternal)
		data.backBegin = len(graph.regionBack)
		graph.regionBack = append(graph.regionBack, staged.back...)
		data.backEnd = len(graph.regionBack)
		data.internalBegin = len(graph.regionInternal)
		graph.regionInternal = append(graph.regionInternal, staged.internal...)
		data.internalEnd = len(graph.regionInternal)
		data.environmentExternalBegin = len(graph.regionEnvironmentExternal)
		graph.regionEnvironmentExternal = append(graph.regionEnvironmentExternal, staged.environmentExternal...)
		data.environmentExternalEnd = len(graph.regionEnvironmentExternal)
		data.environmentBackBegin = len(graph.regionEnvironmentBack)
		graph.regionEnvironmentBack = append(graph.regionEnvironmentBack, staged.environmentBack...)
		data.environmentBackEnd = len(graph.regionEnvironmentBack)
		data.factorExternalBegin = len(graph.regionFactorExternal)
		graph.regionFactorExternal = append(graph.regionFactorExternal, staged.factorExternal...)
		data.factorExternalEnd = len(graph.regionFactorExternal)
		data.factorBackBegin = len(graph.regionFactorBack)
		graph.regionFactorBack = append(graph.regionFactorBack, staged.factorBack...)
		data.factorBackEnd = len(graph.regionFactorBack)
		data.factorInternalBegin = len(graph.regionFactorInternal)
		graph.regionFactorInternal = append(graph.regionFactorInternal, staged.factorInternal...)
		data.factorInternalEnd = len(graph.regionFactorInternal)
		data.factorBegin = len(graph.regionFactors)
		graph.regionFactors = append(graph.regionFactors, staged.factors...)
		data.factorEnd = len(graph.regionFactors)
	}
	graph.regions = regions
	return true
}

func regionContainsPoint(graph *Graph, region regionData, point schedule.Node) bool {
	if graph == nil || point < 0 || int(point) >= len(graph.pointOrder) {
		return false
	}
	order := graph.pointOrder[point]
	return order >= region.pointBegin && order < region.pointEnd
}

func deriveGroupFactors(group GroupNode, rules map[composition.Key]composition.Rule) ([]composition.Key, bool) {
	factors := make([]composition.Key, 0, group.MemberCount())
	for _, member := range group.members {
		rule, ok := rules[member.rule]
		if !ok {
			return nil, false
		}
		if rule.OutputKind != composition.FactorOutput {
			continue
		}
		if !rule.Output.Available() {
			return nil, false
		}
		factors = append(factors, rule.Output)
		for _, carry := range rule.Carries {
			if !carry.Factor.Available() {
				return nil, false
			}
			factors = append(factors, carry.Factor)
		}
	}
	if len(factors) == 0 {
		return nil, true
	}
	sort.Slice(factors, func(left, right int) bool { return lessKey(factors[left], factors[right]) })
	end := 1
	for _, factor := range factors[1:] {
		if factor != factors[end-1] {
			factors[end] = factor
			end++
		}
	}
	return factors[:end], true
}

func compactRegionFactors(region *pendingRegion) bool {
	if region == nil || len(region.factors) == 0 {
		return region != nil
	}
	sort.Slice(region.factors, func(left, right int) bool { return lessKey(region.factors[left], region.factors[right]) })
	end := 1
	for _, factor := range region.factors[1:] {
		if !factor.Available() {
			return false
		}
		if factor != region.factors[end-1] {
			region.factors[end] = factor
			end++
		}
	}
	if !region.factors[0].Available() {
		return false
	}
	region.factors = region.factors[:end]
	return true
}

func (graph *Graph) RegionCount() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.regions)
}

func (graph *Graph) RegionAt(index int) (RegionView, bool) {
	if !graph.valid() || index < 0 || index >= len(graph.regions) {
		return RegionView{}, false
	}
	return RegionView{graph: graph, index: index}, true
}

func (view RegionView) data() (*regionData, bool) {
	if view.graph == nil || !view.graph.valid() || view.index < 0 || view.index >= len(view.graph.regions) {
		return nil, false
	}
	return &view.graph.regions[view.index], true
}

func (view RegionView) Head() (Point, bool) {
	data, ok := view.data()
	if !ok {
		return Point{}, false
	}
	return view.graph.PointAt(data.head)
}

// Parent is this Region's enclosing recurrence ordinal, or schedule.NoRegion
// for a root recurrence. The ordinal is graph-private scheduler topology, not
// a durable semantic identity.
func (view RegionView) Parent() (int, bool) {
	data, ok := view.data()
	if !ok {
		return schedule.NoRegion, false
	}
	return data.parent, true
}

func (view RegionView) PointCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.pointEnd - data.pointBegin
}

func (view RegionView) PointAt(index int) (Point, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.pointBegin+index >= data.pointEnd {
		return Point{}, false
	}
	return view.graph.PointAt(view.graph.eventPoints[data.pointBegin+index])
}

func (view RegionView) InterfaceCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.interfaceEnd - data.interfaceBegin
}

// FaceAt returns one external source Point version that can affect this
// Region's exact head RHS. Faces include ordinary Group inputs, designated
// environment inputs, structural environment edges, and Factor edges;
// InterfaceAt remains
// the typed Group-input projection for callers that need the original port.
func (view RegionView) FaceCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.faceEnd - data.faceBegin
}

func (view RegionView) FaceAt(index int) (Point, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.faceBegin+index >= data.faceEnd {
		return Point{}, false
	}
	return view.graph.PointAt(schedule.Node(view.graph.regionFaces[data.faceBegin+index]))
}

func (view RegionView) ExternalHeadProducerCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.externalEnd - data.externalBegin
}

func (view RegionView) ExternalHeadProducerAt(index int) (GroupNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.externalBegin+index >= data.externalEnd {
		return GroupNode{}, false
	}
	return view.graph.HyperedgeAt(view.graph.regionExternal[data.externalBegin+index])
}

func (view RegionView) BackHeadProducerCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.backEnd - data.backBegin
}

func (view RegionView) BackHeadProducerAt(index int) (GroupNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.backBegin+index >= data.backEnd {
		return GroupNode{}, false
	}
	return view.graph.HyperedgeAt(view.graph.regionBack[data.backBegin+index])
}

func (view RegionView) ExternalEnvironmentEdgeCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.environmentExternalEnd - data.environmentExternalBegin
}

func (view RegionView) ExternalEnvironmentEdgeAt(index int) (EnvironmentEdgeNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.environmentExternalBegin+index >= data.environmentExternalEnd {
		return EnvironmentEdgeNode{}, false
	}
	return view.graph.EnvironmentEdgeAtIndex(view.graph.regionEnvironmentExternal[data.environmentExternalBegin+index])
}

func (view RegionView) BackEnvironmentEdgeCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.environmentBackEnd - data.environmentBackBegin
}

func (view RegionView) BackEnvironmentEdgeAt(index int) (EnvironmentEdgeNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.environmentBackBegin+index >= data.environmentBackEnd {
		return EnvironmentEdgeNode{}, false
	}
	return view.graph.EnvironmentEdgeAtIndex(view.graph.regionEnvironmentBack[data.environmentBackBegin+index])
}

func (view RegionView) ExternalFactorEdgeCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.factorExternalEnd - data.factorExternalBegin
}

func (view RegionView) ExternalFactorEdgeAt(index int) (FactorEdgeNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.factorExternalBegin+index >= data.factorExternalEnd {
		return FactorEdgeNode{}, false
	}
	return view.graph.FactorEdgeAtIndex(view.graph.regionFactorExternal[data.factorExternalBegin+index])
}

func (view RegionView) BackFactorEdgeCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.factorBackEnd - data.factorBackBegin
}

func (view RegionView) BackFactorEdgeAt(index int) (FactorEdgeNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.factorBackBegin+index >= data.factorBackEnd {
		return FactorEdgeNode{}, false
	}
	return view.graph.FactorEdgeAtIndex(view.graph.regionFactorBack[data.factorBackBegin+index])
}

// InternalFactorEdgeCount reports every FactorEdge whose source and target
// both lie inside this Region. Back edges are included; this row is the
// complete factor-incidence witness used by runtime recurrence binding.
func (view RegionView) InternalFactorEdgeCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.factorInternalEnd - data.factorInternalBegin
}

func (view RegionView) InternalFactorEdgeAt(index int) (FactorEdgeNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.factorInternalBegin+index >= data.factorInternalEnd {
		return FactorEdgeNode{}, false
	}
	return view.graph.FactorEdgeAtIndex(view.graph.regionFactorInternal[data.factorInternalBegin+index])
}

func (view RegionView) InternalGroupCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.internalEnd - data.internalBegin
}

func (view RegionView) InternalHyperedgeAt(index int) (GroupNode, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.internalBegin+index >= data.internalEnd {
		return GroupNode{}, false
	}
	return view.graph.HyperedgeAt(view.graph.regionInternal[data.internalBegin+index])
}

func (view RegionView) FactorCount() int {
	data, ok := view.data()
	if !ok {
		return 0
	}
	return data.factorEnd - data.factorBegin
}

func (view RegionView) FactorAt(index int) (composition.Key, bool) {
	data, ok := view.data()
	if !ok || index < 0 || data.factorBegin+index >= data.factorEnd {
		return composition.Key{}, false
	}
	return view.graph.regionFactors[data.factorBegin+index], true
}

// PointRegion reports the exact innermost WTO Region containing point. It is
// derived at graph construction from the one event stream, so runtime binding
// never replays the schedule merely to recover region membership.
func (graph *Graph) PointRegion(point Point) (int, bool) {
	if !graph.valid() || !graph.ownsNode(point.graph) {
		return schedule.NoRegion, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || node < 0 || int(node) >= len(graph.pointRegion) {
		return schedule.NoRegion, false
	}
	region := graph.pointRegion[node]
	if region < schedule.NoRegion || region >= len(graph.regions) {
		return schedule.NoRegion, false
	}
	return region, true
}

func (input InterfaceInput) Group() (GroupNode, bool) {
	if input.graph == nil || !input.graph.valid() {
		return GroupNode{}, false
	}
	return input.graph.HyperedgeAt(input.group)
}

func (input InterfaceInput) Input() (Input, bool) {
	group, ok := input.Group()
	if !ok {
		return Input{}, false
	}
	return group.InputAt(input.input)
}
