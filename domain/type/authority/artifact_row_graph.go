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

type canonicalProgramChildRow interface {
	programschema.Row
	ChildID() identity.ContentID
}

func appendCanonicalProgramFamily[V canonicalProgramChildRow](count uint32, at func(int) (V, bool), children *[]identity.ContentID) bool {
	for position := uint32(0); position < count; position++ {
		child, ok := at(int(position))
		if !ok || !child.Available() || !child.ChildID().Available() {
			return false
		}
		*children = append(*children, child.ChildID())
	}
	return true
}

// canonicalRowDependencies reads the typed canonical families in their
// historical semantic order. It deliberately does not synthesize a generic
// child row or retain a projected child vocabulary.
func canonicalProgramStaticNodeChildren(program programschema.Program, index int, row programschema.StaticTypeNode) ([]identity.ContentID, bool) {
	if !program.Available() || index < 0 {
		return nil, false
	}
	var result []identity.ContentID
	add := func(id identity.ContentID, ok bool) bool {
		if !ok || !id.Available() {
			return false
		}
		result = append(result, id)
		return true
	}
	switch row.Kind() {
	case programschema.StaticNodeOptional:
		id, ok := row.OptionalInner()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeUnion:
		_, count, spanOK := row.UnionMemberSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeUnionMember, bool) {
			return program.StaticTypeNodeUnionMemberFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeIntersection:
		_, count, spanOK := row.IntersectionMemberSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeIntersectionMember, bool) {
			return program.StaticTypeNodeIntersectionMemberFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeGeneric:
		id, ok := row.GenericBase()
		if !add(id, ok) {
			return nil, false
		}
		_, count, spanOK := row.GenericArgumentSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeGenericArgument, bool) {
			return program.StaticTypeNodeGenericArgumentFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeArray:
		id, ok := row.ArrayElement()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeMap:
		id, ok := row.MapKey()
		if !add(id, ok) {
			return nil, false
		}
		id, ok = row.MapValue()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeRecord:
		_, count, spanOK := row.RecordFieldSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeRecordField, bool) {
			return program.StaticTypeNodeRecordFieldFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeReference:
		id, ok := row.ReferenceTarget()
		if id.Available() && !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeAlias:
		id, ok := row.AliasTarget()
		if !add(id, ok) {
			return nil, false
		}
		_, count, spanOK := row.AliasParameterSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeAliasParameter, bool) {
			return program.StaticTypeNodeAliasParameterFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeTypeParam:
		id, ok := row.TypeParamConstraint()
		if id.Available() && !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeInterface:
		_, count, spanOK := row.InterfaceExtendSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeInterfaceExtend, bool) {
			return program.StaticTypeNodeInterfaceExtendFor(index, position)
		}, &result) {
			return nil, false
		}
		_, count, spanOK = row.InterfaceMemberSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeInterfaceMember, bool) {
			return program.StaticTypeNodeInterfaceMemberFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeTypeFunction:
		if id, ok := row.TypeFunctionVariadic(); ok {
			if !add(id, true) {
				return nil, false
			}
		}
		_, count, spanOK := row.TypeFunctionTypeParameterSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeTypeFunctionTypeParameter, bool) {
			return program.StaticTypeNodeTypeFunctionTypeParameterFor(index, position)
		}, &result) {
			return nil, false
		}
		_, count, spanOK = row.TypeFunctionParameterSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeTypeFunctionParameter, bool) {
			return program.StaticTypeNodeTypeFunctionParameterFor(index, position)
		}, &result) {
			return nil, false
		}
		_, count, spanOK = row.TypeFunctionReturnSpan()
		if !spanOK {
			return nil, false
		}
		if !appendCanonicalProgramFamily(count, func(position int) (programschema.StaticTypeNodeTypeFunctionReturn, bool) {
			return program.StaticTypeNodeTypeFunctionReturnFor(index, position)
		}, &result) {
			return nil, false
		}
	case programschema.StaticNodeKeyOf:
		id, ok := row.KeyOfChild()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeIndex:
		id, ok := row.IndexObject()
		if !add(id, ok) {
			return nil, false
		}
		id, ok = row.IndexKey()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeConditional:
		a, b, c, d, ok := row.ConditionalChildren()
		if !ok || !add(a, true) || !add(b, true) || !add(c, true) || !add(d, true) {
			return nil, false
		}
	case programschema.StaticNodeAssertion:
		id, ok := row.AssertionNarrowID()
		if id.Available() && !add(id, ok) {
			return nil, false
		}
	}
	return result, true
}

func canonicalRowDependencies(row programschema.StaticTypeNode, location canonicalRowLocation, out []identity.ContentID) ([]identity.ContentID, bool) {
	dependencies, ok := canonicalProgramStaticNodeChildren(location.program, location.index, row)
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
