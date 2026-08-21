package typeauthority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// rowComponent places one canonical static node in the strongly connected
// component of the node dependency graph. Component membership is a
// transaction-local index; the authority retains only the canonical Programs.
type rowComponent struct {
	id     int
	cyclic bool
}

type canonicalRowLocation struct {
	program programschema.Program
	index   int
}

func canonicalRowDependencies(row programschema.StaticTypeNode, location canonicalRowLocation, out []identity.ContentID) ([]identity.ContentID, bool) {
	dependencies, ok := location.program.StaticTypeNodeChildren(location.index, row, false)
	if !ok {
		return nil, false
	}
	return append(out, dependencies...), true
}

// componentsOfRows decomposes the canonical row graph into strongly
// connected components with an iterative Tarjan walk. locations is a
// transient owner/index lookup; the resulting authority keeps only components.
func componentsOfRows(locations map[identity.ContentID]canonicalRowLocation) (map[identity.ContentID]rowComponent, bool) {
	edges := make(map[identity.ContentID][]identity.ContentID, len(locations))
	for id, location := range locations {
		row, ok := location.program.StaticTypeNodeAt(location.index)
		if !ok {
			return nil, false
		}
		dependencies, ok := canonicalRowDependencies(row, location, nil)
		if !ok {
			return nil, false
		}
		edges[id] = dependencies
	}
	walk := tarjanWalk{
		nodes:     make(map[identity.ContentID]struct{}, len(locations)),
		edgesByID: edges,
		index:     make(map[identity.ContentID]int, len(locations)),
		lowlink:   make(map[identity.ContentID]int, len(locations)),
		onStack:   make(map[identity.ContentID]bool, len(locations)),
		component: make(map[identity.ContentID]rowComponent, len(locations)),
	}
	for id := range locations {
		walk.nodes[id] = struct{}{}
	}
	for id := range locations {
		if _, seen := walk.index[id]; !seen {
			walk.visit(id)
		}
	}
	return walk.component, true
}

type tarjanFrame struct {
	id       identity.ContentID
	edges    []identity.ContentID
	position int
}

type tarjanWalk struct {
	nodes      map[identity.ContentID]struct{}
	edgesByID  map[identity.ContentID][]identity.ContentID
	index      map[identity.ContentID]int
	lowlink    map[identity.ContentID]int
	onStack    map[identity.ContentID]bool
	component  map[identity.ContentID]rowComponent
	stack      []identity.ContentID
	frames     []tarjanFrame
	members    []identity.ContentID
	edges      []identity.ContentID
	next       int
	components int
}

func (w *tarjanWalk) visit(root identity.ContentID) {
	w.push(root)
	for len(w.frames) > 0 {
		frame := &w.frames[len(w.frames)-1]
		if frame.position < len(frame.edges) {
			child := frame.edges[frame.position]
			frame.position++
			if _, known := w.nodes[child]; !known {
				continue
			}
			if _, seen := w.index[child]; !seen {
				w.push(child)
				continue
			}
			if w.onStack[child] {
				w.lowlink[frame.id] = min(w.lowlink[frame.id], w.index[child])
			}
			continue
		}
		id := frame.id
		w.frames = w.frames[:len(w.frames)-1]
		if len(w.frames) > 0 {
			parent := &w.frames[len(w.frames)-1]
			w.lowlink[parent.id] = min(w.lowlink[parent.id], w.lowlink[id])
		}
		if w.lowlink[id] == w.index[id] {
			w.close(id)
		}
	}
}

func (w *tarjanWalk) push(id identity.ContentID) {
	w.index[id] = w.next
	w.lowlink[id] = w.next
	w.next++
	w.stack = append(w.stack, id)
	w.onStack[id] = true
	w.frames = append(w.frames, tarjanFrame{id: id, edges: w.edgesByID[id]})
}

func (w *tarjanWalk) close(root identity.ContentID) {
	id := w.components
	w.components++
	members := w.members[:0]
	for {
		member := w.stack[len(w.stack)-1]
		w.stack = w.stack[:len(w.stack)-1]
		w.onStack[member] = false
		members = append(members, member)
		if member == root {
			break
		}
	}
	cyclic := len(members) > 1
	if !cyclic {
		for _, edge := range w.edgesByID[root] {
			if edge == root {
				cyclic = true
				break
			}
		}
	}
	for _, member := range members {
		w.component[member] = rowComponent{id: id, cyclic: cyclic}
	}
	w.members = members[:0]
}
