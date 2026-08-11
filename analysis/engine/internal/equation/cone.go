package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// Demand is the selected Point closure for registered Queries. It references
// the one point-only schedule and never builds another recurrence order.
type Demand struct {
	graph  *Graph
	nodes  []schedule.Node
	events []int
}

// Demand derives the transitive predecessor closure of registered query
// Points, then expands every reached WTO region to its complete Point
// membership before exposing its schedule events.
func (graph *Graph) Demand() (*Demand, bool) {
	if !graph.valid() || len(graph.queries) == 0 {
		return nil, false
	}
	selected := make([]bool, len(graph.points))
	frontier := make([]schedule.Node, 0, len(graph.queries))
	for _, query := range graph.queries {
		node, ok := graph.pointAt[query.point.key]
		if !ok {
			return nil, false
		}
		if !selected[node] {
			selected[node] = true
			frontier = append(frontier, node)
		}
	}
	for len(frontier) != 0 {
		last := len(frontier) - 1
		node := frontier[last]
		frontier = frontier[:last]
		for _, edgeIndex := range graph.producers[node] {
			edge := graph.groups[edgeIndex]
			for _, input := range edge.inputs {
				predecessor, ok := graph.pointAt[input.point.key]
				if !ok {
					return nil, false
				}
				if !selected[predecessor] {
					selected[predecessor] = true
					frontier = append(frontier, predecessor)
				}
			}
			if edge.environmentInput.Available() {
				predecessor, ok := graph.pointAt[edge.environmentInput.point.key]
				if !ok {
					return nil, false
				}
				if !selected[predecessor] {
					selected[predecessor] = true
					frontier = append(frontier, predecessor)
				}
			}
		}
		for _, edgeIndex := range graph.environmentIncoming[node] {
			if edgeIndex < 0 || edgeIndex >= len(graph.environments) {
				return nil, false
			}
			predecessor, ok := graph.pointAt[graph.environments[edgeIndex].input.point.key]
			if !ok {
				return nil, false
			}
			if !selected[predecessor] {
				selected[predecessor] = true
				frontier = append(frontier, predecessor)
			}
		}
		for _, edgeIndex := range graph.factorIncoming[node] {
			if edgeIndex < 0 || edgeIndex >= len(graph.factorEdges) {
				return nil, false
			}
			predecessor, ok := graph.pointAt[graph.factorEdges[edgeIndex].input.point.key]
			if !ok {
				return nil, false
			}
			if !selected[predecessor] {
				selected[predecessor] = true
				frontier = append(frontier, predecessor)
			}
		}
		for _, trigger := range graph.activationReverses[node] {
			if trigger < 0 || int(trigger) >= len(selected) {
				return nil, false
			}
			if !selected[trigger] {
				selected[trigger] = true
				frontier = append(frontier, trigger)
			}
		}
	}
	if !graph.expandRegions(selected) {
		return nil, false
	}
	return graph.selectedDemand(selected)
}

// expandRegions is linear in the one event stream plus the region catalog.
// Region reachability is tested by an O(1) selected-node prefix count; one
// stack pass then includes every Point enclosed by any reached region.
func (graph *Graph) expandRegions(selected []bool) bool {
	if !graph.valid() || len(selected) != len(graph.points) || len(graph.eventNodes) != graph.schedule.EventCount()+1 || len(graph.regionNodes) != graph.schedule.RegionCount() {
		return false
	}
	selectedPrefix := make([]int, graph.schedule.EventCount()+1)
	for index := 0; index < graph.schedule.EventCount(); index++ {
		event, ok := graph.schedule.EventAt(index)
		if !ok {
			return false
		}
		selectedPrefix[index+1] = selectedPrefix[index]
		if event.Kind == schedule.EventNode {
			if event.Node < 0 || int(event.Node) >= len(selected) {
				return false
			}
			if selected[event.Node] {
				selectedPrefix[index+1]++
			}
		}
	}
	reached := make([]bool, len(graph.regionNodes))
	for index := range reached {
		region, ok := graph.schedule.RegionAt(index)
		if !ok || region.Enter < 0 || region.Exit < region.Enter || region.Exit >= graph.schedule.EventCount() || graph.regionNodes[index] != graph.eventNodes[region.Exit+1]-graph.eventNodes[region.Enter] {
			return false
		}
		reached[index] = selectedPrefix[region.Exit+1]-selectedPrefix[region.Enter] != 0
	}
	active := make([]bool, 0, len(reached))
	include := false
	for index := 0; index < graph.schedule.EventCount(); index++ {
		event, ok := graph.schedule.EventAt(index)
		if !ok {
			return false
		}
		switch event.Kind {
		case schedule.EventEnter:
			if event.Region < 0 || event.Region >= len(reached) {
				return false
			}
			active = append(active, include)
			include = include || reached[event.Region]
		case schedule.EventNode:
			if event.Node < 0 || int(event.Node) >= len(selected) {
				return false
			}
			if include {
				selected[event.Node] = true
			}
		case schedule.EventExit:
			if event.Region < 0 || event.Region >= len(reached) || len(active) == 0 {
				return false
			}
			include = active[len(active)-1]
			active = active[:len(active)-1]
		default:
			return false
		}
	}
	return len(active) == 0
}

func (graph *Graph) selectedDemand(selected []bool) (*Demand, bool) {
	if !graph.valid() || len(selected) != len(graph.points) {
		return nil, false
	}
	selectedPrefix := make([]int, graph.schedule.EventCount()+1)
	for index := 0; index < graph.schedule.EventCount(); index++ {
		event, ok := graph.schedule.EventAt(index)
		if !ok {
			return nil, false
		}
		selectedPrefix[index+1] = selectedPrefix[index]
		if event.Kind == schedule.EventNode {
			if event.Node < 0 || int(event.Node) >= len(selected) {
				return nil, false
			}
			if selected[event.Node] {
				selectedPrefix[index+1]++
			}
		}
	}
	full := make([]bool, graph.schedule.RegionCount())
	for index := range full {
		region, ok := graph.schedule.RegionAt(index)
		if !ok || graph.regionNodes[index] == 0 {
			return nil, false
		}
		count := selectedPrefix[region.Exit+1] - selectedPrefix[region.Enter]
		if count > graph.regionNodes[index] {
			return nil, false
		}
		full[index] = count == graph.regionNodes[index]
		if count != 0 && !full[index] {
			return nil, false
		}
	}
	nodes := make([]schedule.Node, 0, len(graph.points))
	for index, selected := range selected {
		if selected {
			nodes = append(nodes, schedule.Node(index))
		}
	}
	events := make([]int, 0, graph.schedule.EventCount())
	for index := 0; index < graph.schedule.EventCount(); index++ {
		event, ok := graph.schedule.EventAt(index)
		if !ok {
			return nil, false
		}
		switch event.Kind {
		case schedule.EventNode:
			if selected[event.Node] {
				events = append(events, index)
			}
		case schedule.EventEnter, schedule.EventExit:
			if event.Region < 0 || event.Region >= len(full) {
				return nil, false
			}
			if full[event.Region] {
				events = append(events, index)
			}
		default:
			return nil, false
		}
	}
	if len(nodes) == 0 || len(events) == 0 {
		return nil, false
	}
	return &Demand{graph: graph, nodes: nodes, events: events}, true
}

func (demand *Demand) PointCount() int {
	if demand == nil || demand.graph == nil {
		return 0
	}
	return len(demand.nodes)
}
func (demand *Demand) PointAt(index int) (Point, bool) {
	if demand == nil || demand.graph == nil || index < 0 || index >= len(demand.nodes) {
		return Point{}, false
	}
	return demand.graph.PointAt(demand.nodes[index])
}
func (demand *Demand) EventCount() int {
	if demand == nil || demand.graph == nil {
		return 0
	}
	return len(demand.events)
}
func (demand *Demand) EventAt(index int) (schedule.Event, int, bool) {
	if demand == nil || demand.graph == nil || index < 0 || index >= len(demand.events) {
		return schedule.Event{}, 0, false
	}
	original := demand.events[index]
	event, ok := demand.graph.schedule.EventAt(original)
	return event, original, ok
}
func (demand *Demand) PositionAtOrAfter(original int) (int, bool) {
	if demand == nil || demand.graph == nil || original < 0 {
		return 0, false
	}
	position := sort.SearchInts(demand.events, original)
	return position, position < len(demand.events)
}
