package typ

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/domain/type/kind"
)

// IsUnknown reports whether t is explicitly the unknown type.
func IsUnknown(t Type) bool {
	return t != nil && t.Kind() == kind.Unknown
}

// IsAny reports whether t is explicitly the any type.
func IsAny(t Type) bool {
	return t != nil && t.Kind() == kind.Any
}

// IsNever reports whether t is explicitly the never type.
func IsNever(t Type) bool {
	return t != nil && t.Kind() == kind.Never
}

// IsTopLike reports whether t is an explicit top-like type.
//
// A missing type (nil) is not top-like; use AbsentOrTopLike when absence should
// be treated as unknown by a caller.
func IsTopLike(t Type) bool {
	return IsAny(t) || IsUnknown(t)
}

// AbsentOrTopLike reports whether t is missing or explicitly top-like.
func AbsentOrTopLike(t Type) bool {
	return t == nil || IsTopLike(t)
}

// AdmitsFalse reports whether t's value set can contain the literal false.
func AdmitsFalse(t Type) bool {
	return evaluatePredicate(t, predicateAdmitsFalse)
}

// IsBooleanType reports whether t is definitely contained in boolean.
func IsBooleanType(t Type) bool {
	return evaluatePredicate(t, predicateBoolean)
}

// IsIntegerIndexType reports whether t is definitely usable as an integer
// index. It is a type-domain predicate used by analyses that need proof that a
// dynamic key ranges over integer slots, such as ipairs-style generic-for
// transfer.
func IsIntegerIndexType(t Type) bool {
	return evaluatePredicate(t, predicateIntegerIndex)
}

// predicateKind selects one of the small monotone Boolean equations evaluated
// by predicateWork. Keeping the equations together avoids three subtly
// different recursive walkers and gives every public predicate the same finite
// graph and cycle semantics.
type predicateKind uint8

const (
	predicateAdmitsFalse predicateKind = iota
	predicateBoolean
	predicateIntegerIndex
)

type predicateOperation uint8

const (
	predicateFalse predicateOperation = iota
	predicateTrue
	predicateAny
	predicateAll
)

// predicateNode is one memoized Type node. A node starts false and can become
// true once. This is the least fixed point of the predicate equations, so an
// ungrounded cycle fails closed while a cycle with a sufficient witness still
// succeeds. parents includes duplicate edges deliberately: an all-node with a
// repeated member still needs one proof per member.
type predicateNode struct {
	t       Type
	parents []*predicateNode
	pending int
	op      predicateOperation
	value   bool
}

// predicateWork owns the whole finite work machine for one public query. It
// never calls a predicate recursively: discovery expands each Type identity at
// most once, then a backward work list propagates true witnesses to a fixed
// point. The map is intentionally created only for non-leaf queries; common
// primitive and literal calls remain allocation-free.
type predicateWork struct {
	kind  predicateKind
	nodes map[Type]*predicateNode
	stack []*predicateNode
	ready []*predicateNode
}

func evaluatePredicate(t Type, which predicateKind) bool {
	t = NormalizeNil(t)
	if value, known := predicateLeaf(t, which); known {
		return value
	}
	if value, known := loadPredicate(t, which); known {
		return value
	}

	work := predicateWork{
		kind:  which,
		nodes: make(map[Type]*predicateNode),
	}
	root := work.intern(t)
	work.expandAll()
	work.propagate()
	work.publish()
	return root.value
}

// The predicate equations never descend through a recursive or generic
// declaration: both are terminal for every predicateKind. Nothing else a node
// reaches can change after construction, so a value the fixpoint settles on is
// permanent and is published without a closure gate. One query resolves a
// whole reachable graph, so the whole graph is published, not just the root.
func (w *predicateWork) publish() {
	for subject, node := range w.nodes {
		storePredicate(subject, w.kind, node.value)
	}
}

// predicatePublished is the low bit of the published half of the column word.
const predicatePublished uint32 = 1 << 8

func loadPredicate(t Type, which predicateKind) (bool, bool) {
	properties := nodeProperties(t)
	if properties == nil {
		return false, false
	}
	raw := atomic.LoadUint32(&properties.predicates)
	if raw&(predicatePublished<<uint(which)) == 0 {
		return false, false
	}
	return raw&(1<<uint(which)) != 0, true
}

func storePredicate(t Type, which predicateKind, value bool) {
	properties := nodeProperties(t)
	if properties == nil {
		return
	}
	published := predicatePublished << uint(which)
	if value {
		published |= 1 << uint(which)
	}
	for {
		current := atomic.LoadUint32(&properties.predicates)
		next := current | published
		if next == current || atomic.CompareAndSwapUint32(&properties.predicates, current, next) {
			return
		}
	}
}

// predicateLeaf handles all terminal cases and the malformed wrapper forms
// that cannot provide a child. It is deliberately separate from expansion so
// direct primitive calls do not allocate a graph work machine.
func predicateLeaf(t Type, which predicateKind) (bool, bool) {
	if t == nil {
		return false, true
	}
	switch tt := t.(type) {
	case *Literal:
		if tt == nil {
			return false, true
		}
		switch which {
		case predicateAdmitsFalse:
			return tt.base == kind.Boolean && tt.value == false, true
		case predicateBoolean:
			return tt.base == kind.Boolean, true
		case predicateIntegerIndex:
			return tt.base == kind.Integer, true
		}
	case *Annotated:
		if tt == nil || tt.Inner == nil || tt.Inner == t {
			return false, true
		}
		return false, false
	case *Alias:
		if tt == nil {
			return false, true
		}
		next := tt.UnaliasedTarget()
		if next == nil || next == t {
			return false, true
		}
		return false, false
	case *Optional:
		if tt == nil {
			return false, true
		}
		if which != predicateAdmitsFalse {
			return false, true
		}
		if tt.Inner == nil {
			return false, true
		}
		return false, false
	case *Union:
		return tt == nil, tt == nil
	case *Intersection:
		return tt == nil, tt == nil
	}

	switch which {
	case predicateAdmitsFalse, predicateBoolean:
		return TypeEquals(t, Boolean), true
	case predicateIntegerIndex:
		return TypeEquals(t, Integer), true
	default:
		return false, true
	}
}

func (w *predicateWork) intern(t Type) *predicateNode {
	t = NormalizeNil(t)
	if t == nil {
		return nil
	}
	if node := w.nodes[t]; node != nil {
		return node
	}
	node := &predicateNode{t: t}
	w.nodes[t] = node
	w.stack = append(w.stack, node)
	return node
}

func (w *predicateWork) expandAll() {
	for len(w.stack) != 0 {
		last := len(w.stack) - 1
		node := w.stack[last]
		w.stack[last] = nil
		w.stack = w.stack[:last]
		w.expand(node)
	}
}

func (w *predicateWork) expand(node *predicateNode) {
	if value, known := predicateLeaf(node.t, w.kind); known {
		if value {
			node.op = predicateTrue
			w.ready = append(w.ready, node)
		} else {
			node.op = predicateFalse
		}
		return
	}

	switch tt := node.t.(type) {
	case *Annotated:
		node.op = predicateAll
		w.link(node, tt.Inner)
	case *Alias:
		node.op = predicateAll
		w.link(node, tt.UnaliasedTarget())
	case *Optional:
		node.op = predicateAll
		w.link(node, tt.Inner)
	case *Union:
		if len(tt.Members) == 0 {
			node.op = predicateFalse
			return
		}
		if w.kind == predicateAdmitsFalse {
			node.op = predicateAny
		} else {
			node.op = predicateAll
		}
		for _, member := range tt.Members {
			w.link(node, member)
		}
	case *Intersection:
		if len(tt.Members) == 0 {
			node.op = predicateFalse
			return
		}
		if w.kind == predicateAdmitsFalse {
			node.op = predicateAll
		} else {
			node.op = predicateAny
		}
		for _, member := range tt.Members {
			w.link(node, member)
		}
	default:
		// predicateLeaf classifies every non-wrapper and non-composite Type.
		node.op = predicateFalse
	}
}

func (w *predicateWork) link(parent *predicateNode, child Type) {
	if parent.op == predicateAll {
		parent.pending++
	}
	childNode := w.intern(child)
	if childNode != nil {
		childNode.parents = append(childNode.parents, parent)
	}
}

func (w *predicateWork) propagate() {
	for len(w.ready) != 0 {
		last := len(w.ready) - 1
		node := w.ready[last]
		w.ready[last] = nil
		w.ready = w.ready[:last]
		if node.value {
			continue
		}
		node.value = true
		for _, parent := range node.parents {
			switch parent.op {
			case predicateAny:
				w.ready = append(w.ready, parent)
			case predicateAll:
				parent.pending--
				if parent.pending == 0 {
					w.ready = append(w.ready, parent)
				}
			}
		}
	}
}

// AbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func AbsentOrUnknown(t Type) bool {
	return t == nil || IsUnknown(t)
}
