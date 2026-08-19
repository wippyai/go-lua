package semanticpath

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// missingStructuralEdge reports the first unlabelled edge on the exact
// containment ascent. It is diagnostics-only and never manufactures a role.
func (r *structuralResolver) missingStructuralEdge(term, expectedBody keyspace.Term) (keyspace.Term, keyspace.Term) {
	for current := term; current != 0; {
		root, rootOK := r.source.Index().Root(current)
		body, _, _, positioned := r.source.Index().Position(current)
		if !rootOK || !positioned || body != expectedBody || current == root {
			return 0, 0
		}
		parent, parentOK := r.forest.Parent(current)
		family, ordinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
		if !parentOK || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(r.edges[family])) || r.edges[family][ordinal-1].kind == 0 {
			return current, parent
		}
		current = parent
	}
	return 0, 0
}

type structuralResolver struct {
	source      source.View
	forest      *containment.Result
	edges       [keyspace.FamilyCount][]edgeDescriptor
	descriptors [keyspace.FamilyCount][]identity.ContentID
	body        []identity.ContentID
	memo        map[keyspace.Term]identity.ContentID
	visiting    map[keyspace.Term]bool
}

func (r *structuralResolver) resolve(term, expectedBody keyspace.Term) (identity.ContentID, bool) {
	if r == nil || term == 0 {
		return identity.ContentID{}, false
	}
	if expectedBody != 0 && (keyspace.TermFamily(expectedBody) != keyspace.FamilyBody || keyspace.TermOrdinal(expectedBody) == 0 || uint64(keyspace.TermOrdinal(expectedBody)) > uint64(len(r.body))) {
		return identity.ContentID{}, false
	}
	if id, ok := r.memo[term]; ok {
		return id, true
	}
	// One containment parent means an explicit ancestor stack is sufficient:
	// ascend each unresolved edge once, then emit paths in parent-to-child
	// order. No Go call stack or repeated ancestry walk is involved.
	stack := make([]keyspace.Term, 0, 8)
	current := term
	clear := func() {
		for _, node := range stack {
			delete(r.visiting, node)
		}
	}
	for {
		if _, ok := r.memo[current]; ok {
			break
		}
		if r.visiting[current] {
			clear()
			return identity.ContentID{}, false
		}
		r.visiting[current] = true
		stack = append(stack, current)
		body, _, _, positioned := r.source.Index().Position(current)
		if positioned {
			if expectedBody != 0 && body != expectedBody {
				clear()
				return identity.ContentID{}, false
			}
			root, ok := r.source.Index().Root(current)
			if !ok || root == 0 {
				clear()
				return identity.ContentID{}, false
			}
			if current == root {
				f, o := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
				if f <= keyspace.FamilyInvalid || f >= keyspace.FamilyCount || o == 0 || uint64(o) > uint64(len(r.descriptors[f])) || !r.descriptors[f][o-1].Available() {
					clear()
					return identity.ContentID{}, false
				}
				if keyspace.TermOrdinal(body) == 0 || uint64(keyspace.TermOrdinal(body)) > uint64(len(r.body)) {
					clear()
					return identity.ContentID{}, false
				}
				r.memo[current] = digestBytes("semantic-root-occurrence-v2", r.body[keyspace.TermOrdinal(body)-1], r.descriptors[f][o-1])
				break
			}
		} else {
			// Positionless terms are not malformed Source rows. They are
			// Static-owned expression descendants, whose first canonical
			// boundary is supplied by the containment owner (usually a lexical
			// Body, or an already-issued Cell path).
			parent, ok := r.forest.Parent(current)
			if !ok || parent == 0 {
				clear()
				return identity.ContentID{}, false
			}
			if parentPath, ok := r.anchorPath(parent, expectedBody); ok {
				f, o := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
				if f <= keyspace.FamilyInvalid || f >= keyspace.FamilyCount || o == 0 || uint64(o) > uint64(len(r.edges[f])) {
					clear()
					return identity.ContentID{}, false
				}
				e := r.edges[f][o-1]
				if e.kind == 0 {
					clear()
					return identity.ContentID{}, false
				}
				r.memo[current] = digestPath3("semantic-rootless-occurrence-v2", parentPath, e.kind, e.rank, uint32(f), source.Span{})
				break
			}
			current = parent
			continue
		}
		parent, ok := r.forest.Parent(current)
		if !ok || parent == 0 {
			clear()
			return identity.ContentID{}, false
		}
		current = parent
	}
	for index := len(stack) - 1; index >= 0; index-- {
		child := stack[index]
		if _, ok := r.memo[child]; ok {
			continue
		}
		parent, ok := r.forest.Parent(child)
		if !ok || parent == 0 {
			clear()
			return identity.ContentID{}, false
		}
		parentPath, ok := r.anchorPath(parent, expectedBody)
		f, o := keyspace.TermFamily(child), keyspace.TermOrdinal(child)
		if !ok || f <= keyspace.FamilyInvalid || f >= keyspace.FamilyCount || o == 0 || uint64(o) > uint64(len(r.edges[f])) || r.edges[f][o-1].kind == 0 {
			clear()
			return identity.ContentID{}, false
		}
		e := r.edges[f][o-1]
		r.memo[child] = digestPath3("semantic-structural-edge-v2", parentPath, e.kind, e.rank, uint32(f), source.Span{})
	}
	clear()
	id, ok := r.memo[term]
	return id, ok
}

// anchorPath returns an already-issued path for a containment parent. Body
// paths are held in the dedicated Body plane, while all other anchors (most
// notably Cells and resolved Source roots) are memoized by the resolver.
func (r *structuralResolver) anchorPath(parent, expectedBody keyspace.Term) (identity.ContentID, bool) {
	if parent == 0 {
		return identity.ContentID{}, false
	}
	if id, ok := r.memo[parent]; ok && id.Available() {
		return id, true
	}
	if keyspace.TermFamily(parent) != keyspace.FamilyBody {
		return identity.ContentID{}, false
	}
	if expectedBody != 0 && parent != expectedBody {
		return identity.ContentID{}, false
	}
	ordinal := keyspace.TermOrdinal(parent)
	if ordinal == 0 || uint64(ordinal) > uint64(len(r.body)) {
		return identity.ContentID{}, false
	}
	id := r.body[ordinal-1]
	return id, id.Available()
}
