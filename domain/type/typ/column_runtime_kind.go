package typ

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
)

// MayRuntimeKinds is the sound may-projection of one type onto the closed Lua
// runtime vocabulary: the families a value of that type may carry at run time.
// It is the column every consumer reads; no consumer restates the fold.
//
// The projection is the least fixed point of the structural equations. Bottom
// is the empty set, a union joins, an intersection meets, and a cycle is
// iterated upwards from bottom rather than answered with the whole vocabulary.
// A recursive group therefore reports exactly the families its productive
// members contribute: μX. string | X is string, not every family. Only a form
// the type language gives no runtime representation for - a bare formal, a
// tuple, an interface, an unresolved declaration, any, unknown - answers the
// whole vocabulary, and never fewer families than the type admits.
//
// A generic application is a binder, not an opaque node. Its formals are bound
// to the projections of its arguments and its declaration body is evaluated
// under those bindings, which is exactly the projection of the substituted
// body: this fold is compositional, so binding the leaves and expanding the
// graph give the same answer.
func MayRuntimeKinds(t Type) runtimekind.Set {
	node := UnwrapStructuralWrappers(NormalizeNil(t))
	if node == nil {
		return runtimekind.All
	}
	if !runtimeKindDerived(node) {
		return runtimeKindLeaf(node, nil)
	}
	if published, ok := loadRuntimeKinds(node); ok {
		return published
	}
	var evaluator runtimeKindEvaluator
	result := evaluator.fixpoint(node)
	// A published value is permanent, so it is only stored once every
	// recursive placeholder and every generic declaration the graph reaches
	// already has a body. Either can still be filled in, and either can change
	// the projection.
	if !evaluator.open && columnsOf(node).closed {
		evaluator.publish()
	}
	return result
}

// runtimeKindDerived reports whether a node's projection depends on the nodes
// it reaches. Every other constructor - a record, an array, a function - has
// one runtime family regardless of what it carries, which is what keeps the
// fixpoint confined to union, intersection, optional, recursion and generic
// application chains.
func runtimeKindDerived(t Type) bool {
	switch t.(type) {
	case *Optional, *Union, *Intersection, *Recursive, *Instantiated:
		return true
	default:
		return false
	}
}

// runtimeKindEvaluator carries one query's fixpoint state. values holds the
// approximation for nodes evaluated outside any generic binder, where a node
// denotes one type; inside a binder the same node denotes a different type per
// argument list, so those results are neither read from nor written to it.
type runtimeKindEvaluator struct {
	values  map[Type]runtimekind.Set
	active  map[Type]bool
	changed bool
	cyclic  bool
	open    bool
}

// fixpoint iterates the monotone equations upward from bottom. An acyclic
// graph is exact after one pass; a cyclic one repeats until no approximation
// grows, which terminates because the vocabulary is finite and every value
// only ever grows.
func (e *runtimeKindEvaluator) fixpoint(root Type) runtimekind.Set {
	for {
		e.changed = false
		e.cyclic = false
		result := e.eval(root, nil)
		if !e.cyclic || !e.changed {
			return result
		}
	}
}

func (e *runtimeKindEvaluator) eval(t Type, bindings map[*TypeParam]runtimekind.Set) runtimekind.Set {
	node := UnwrapStructuralWrappers(NormalizeNil(t))
	if node == nil {
		return runtimekind.All
	}
	if !runtimeKindDerived(node) {
		if declaration, ok := node.(*Generic); ok && (declaration == nil || declaration.Body == nil) {
			e.open = true
		}
		return runtimeKindLeaf(node, bindings)
	}
	if bindings == nil {
		if published, ok := loadRuntimeKinds(node); ok {
			return published
		}
	}
	if e.active[node] {
		e.cyclic = true
		if bindings == nil {
			return e.values[node]
		}
		return 0
	}
	if e.active == nil {
		e.active = make(map[Type]bool)
	}
	e.active[node] = true

	var result runtimekind.Set
	switch typed := node.(type) {
	case *Optional:
		result = runtimekind.Bit(runtimekind.Nil) | e.eval(typed.Inner, bindings)
	case *Union:
		for _, member := range typed.Members {
			result |= e.eval(member, bindings)
		}
	case *Intersection:
		// A value inhabiting an intersection inhabits every member, so meeting
		// their may sets remains a sound over-approximation.
		result = runtimekind.All
		for _, member := range typed.Members {
			result &= e.eval(member, bindings)
		}
	case *Recursive:
		if typed == nil || typed.Body == nil {
			e.open = true
			result = runtimekind.All
		} else {
			result = e.eval(typed.Body, bindings)
		}
	case *Instantiated:
		result = e.application(typed, bindings)
	}

	delete(e.active, node)
	if bindings == nil {
		if e.values == nil {
			e.values = make(map[Type]runtimekind.Set)
		}
		if previous, known := e.values[node]; !known || previous != result {
			e.changed = true
			e.values[node] = result
		}
	}
	return result
}

// application binds the declaration's formals to the projections of the
// arguments and evaluates the body under them.
func (e *runtimeKindEvaluator) application(node *Instantiated, bindings map[*TypeParam]runtimekind.Set) runtimekind.Set {
	if node == nil || node.Generic == nil || node.Generic.Body == nil {
		e.open = true
		return runtimekind.All
	}
	inner := make(map[*TypeParam]runtimekind.Set, len(node.Generic.TypeParams))
	for index, formal := range node.Generic.TypeParams {
		if formal == nil {
			continue
		}
		if index < len(node.TypeArgs) {
			inner[formal] = e.eval(node.TypeArgs[index], bindings)
		} else {
			inner[formal] = runtimekind.All
		}
	}
	return e.eval(node.Generic.Body, inner)
}

func runtimeKindLeaf(node Type, bindings map[*TypeParam]runtimekind.Set) runtimekind.Set {
	if literal, ok := node.(*Literal); ok {
		return runtimeKindForBase(literal.Base())
	}
	if formal, ok := node.(*TypeParam); ok {
		if kinds, bound := bindings[formal]; bound {
			return kinds
		}
		return runtimekind.All
	}
	switch node.Kind() {
	case kind.Never:
		return 0
	case kind.Nil:
		return runtimekind.Bit(runtimekind.Nil)
	case kind.Boolean:
		return runtimekind.Bit(runtimekind.Boolean)
	case kind.Number, kind.Integer:
		return runtimekind.Bit(runtimekind.Number)
	case kind.String:
		return runtimekind.Bit(runtimekind.String)
	case kind.Function:
		return runtimekind.Bit(runtimekind.Function)
	case kind.Array, kind.Map, kind.ReadonlyMap, kind.Record:
		return runtimekind.Bit(runtimekind.Table)
	default:
		// Tuple, interface, meta, formal, unresolved reference, any, unknown
		// and every future constructor stay conservatively unclassified until
		// the type language gives them a proved Lua runtime representation.
		return runtimekind.All
	}
}

func runtimeKindForBase(base kind.Kind) runtimekind.Set {
	switch base {
	case kind.Boolean:
		return runtimekind.Bit(runtimekind.Boolean)
	case kind.Number, kind.Integer:
		return runtimekind.Bit(runtimekind.Number)
	case kind.String:
		return runtimekind.Bit(runtimekind.String)
	default:
		return runtimekind.All
	}
}

func (e *runtimeKindEvaluator) publish() {
	for node, kinds := range e.values {
		storeRuntimeKinds(node, kinds)
	}
}

// runtimeKindsPublished distinguishes a published empty projection - the one
// never carries - from an unpublished slot.
const runtimeKindsPublished uint32 = 1 << 16

func loadRuntimeKinds(t Type) (runtimekind.Set, bool) {
	var raw uint32
	if recursive, ok := t.(*Recursive); ok {
		if recursive == nil {
			return 0, false
		}
		raw = recursive.runtimeKinds.Load()
	} else {
		properties := nodeProperties(t)
		if properties == nil {
			return 0, false
		}
		raw = atomic.LoadUint32(&properties.runtimeKinds)
	}
	if raw == 0 {
		return 0, false
	}
	return runtimekind.Set(raw &^ runtimeKindsPublished), true
}

func storeRuntimeKinds(t Type, kinds runtimekind.Set) {
	raw := uint32(kinds) | runtimeKindsPublished
	if recursive, ok := t.(*Recursive); ok {
		if recursive != nil {
			recursive.runtimeKinds.Store(raw)
		}
		return
	}
	if properties := nodeProperties(t); properties != nil {
		atomic.StoreUint32(&properties.runtimeKinds, raw)
	}
}
