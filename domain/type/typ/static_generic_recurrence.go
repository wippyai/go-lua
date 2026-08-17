package typ

import (
	"errors"
	"reflect"
)

// ErrInvalidStaticGenericRecurrence reports a static generic graph that cannot
// be represented by the one declaration-binder law.  Static self recursion is
// permitted only through a productive constructor.  Mutual generic groups and
// a bare self equation would require a second binder vocabulary, so they stay
// symbolic at their source boundary and never enter a portable type graph.
var ErrInvalidStaticGenericRecurrence = errors.New("typ: invalid static generic recurrence")

// ValidateStaticGenericRecurrence is the one cold admission law for a closed
// existing typ graph. It validates lexical formal ownership and instantiation
// arity, accepts a productive self edge through one Generic declaration, and
// rejects mutual Generic SCCs or an unproductive direct self equation. It
// introduces neither a GenericGroup nor an implicit Recursive node.
//
// This is intentionally a graph check, not a TypeEquals/Hash operation.  Hot
// facts carry handles and never call it.
func ValidateStaticGenericRecurrence(root Type) error {
	return validateStaticGenericRecurrence(root, staticFormalScope{})
}

// ValidateStaticGenericRecurrenceOpen applies the same law to a node that is
// lawfully open at its boundary: every formal occurring free in root is its own
// external scope, so the verdict is the declaration law alone. A consumer that
// mints identity for an open node discharges the law here; the free formals
// travel with the node to the boundary that binds them.
func ValidateStaticGenericRecurrenceOpen(root Type) error {
	return validateStaticGenericRecurrence(root, staticFormalScope{open: true})
}

// ValidateStaticGenericRecurrenceWithFormals applies the same law at a scoped
// artifact boundary. Only the exact supplied external formal identities may
// occur free in root; every other formal must be owned by a lexical Function or
// Generic binder. Ordinary canonical values, manifests, static authorities, and
// Table origins use ValidateStaticGenericRecurrence instead.
func ValidateStaticGenericRecurrenceWithFormals(root Type, formals []*TypeParam) error {
	external := make(map[*TypeParam]struct{}, len(formals))
	for _, formal := range formals {
		if formal == nil {
			return ErrInvalidStaticGenericRecurrence
		}
		if _, duplicate := external[formal]; duplicate {
			return ErrInvalidStaticGenericRecurrence
		}
		external[formal] = struct{}{}
	}
	return validateStaticGenericRecurrence(root, staticFormalScope{external: external})
}

// staticFormalScope is the set of formals a graph may leave free. An open scope
// admits every free formal, which is the verdict for a node whose boundary owns
// the formals rather than the node itself.
type staticFormalScope struct {
	external map[*TypeParam]struct{}
	open     bool
}

func (scope staticFormalScope) permits(param *TypeParam) bool {
	if scope.open {
		return true
	}
	_, permitted := scope.external[param]
	return permitted
}

func validateStaticGenericRecurrence(root Type, scope staticFormalScope) error {
	// A nil canonical type is a valid closed bottom-level payload and carries
	// no generic recurrence.  This law adjudicates declarations, not a second
	// whole-type validity checker.
	if root == nil {
		return nil
	}
	if err := validateStaticFormalOwnership(root, scope); err != nil {
		return err
	}
	generics, err := collectStaticGenerics(root)
	if err != nil {
		return err
	}
	if len(generics) == 0 {
		return nil
	}
	index := make(map[*Generic]int, len(generics))
	for i, generic := range generics {
		index[generic] = i
	}
	edges := make([][]staticGenericEdge, len(generics))
	for i, generic := range generics {
		out, err := directGenericEdges(generic, index)
		if err != nil {
			return err
		}
		edges[i] = out
	}
	component, sizes := staticGenericComponents(edges)
	for i := range generics {
		componentSize := sizes[component[i]]
		if componentSize > 1 {
			return ErrInvalidStaticGenericRecurrence
		}
		for _, edge := range edges[i] {
			if edge.to != i {
				continue
			}
			if !edge.guarded {
				return ErrInvalidStaticGenericRecurrence
			}
		}
	}
	return nil
}

func collectStaticGenerics(root Type) ([]*Generic, error) {
	seen := make(map[uintptr]struct{})
	seenGeneric := make(map[*Generic]struct{})
	out := make([]*Generic, 0)
	stack := []Type{root}
	for len(stack) != 0 {
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if staticNilType(value) {
			return nil, ErrInvalidStaticGenericRecurrence
		}
		if pointer, ok := staticTypePointer(value); ok {
			if _, duplicate := seen[pointer]; duplicate {
				continue
			}
			seen[pointer] = struct{}{}
		}
		if generic, ok := value.(*Generic); ok {
			if generic == nil {
				return nil, ErrInvalidStaticGenericRecurrence
			}
			if _, duplicate := seenGeneric[generic]; !duplicate {
				seenGeneric[generic] = struct{}{}
				out = append(out, generic)
			}
		}
		if instantiated, ok := value.(*Instantiated); ok {
			if instantiated == nil || instantiated.Generic == nil || len(instantiated.TypeArgs) != len(instantiated.Generic.TypeParams) {
				return nil, ErrInvalidStaticGenericRecurrence
			}
		}
		WalkChildren(value, func(child Type) bool {
			stack = append(stack, child)
			return false
		})
	}
	return out, nil
}

func directGenericEdges(owner *Generic, index map[*Generic]int) ([]staticGenericEdge, error) {
	if owner == nil {
		return nil, ErrInvalidStaticGenericRecurrence
	}
	seen := make(map[staticGenericVisit]struct{})
	stack := make([]staticGenericFrame, 0, len(owner.TypeParams)+1)
	if owner.Body != nil {
		stack = append(stack, staticGenericFrame{value: owner.Body})
	}
	// Constraints are declaration semantics even when the body does not use
	// the corresponding formal. Omitting them would admit a recurrence hidden
	// in an unused constraint, creating a second implicit fixed-point path.
	for _, param := range owner.TypeParams {
		if param == nil {
			return nil, ErrInvalidStaticGenericRecurrence
		}
		if param.Constraint != nil {
			stack = append(stack, staticGenericFrame{value: param.Constraint})
		}
	}
	var out []staticGenericEdge
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if staticNilType(current.value) {
			return nil, ErrInvalidStaticGenericRecurrence
		}
		if pointer, ok := staticTypePointer(current.value); ok {
			key := staticGenericVisit{pointer: pointer, guarded: current.guarded}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
		}
		// Recursive is the one explicit fixed-point binder. It delimits the
		// generic declaration graph rather than being a productive wrapper that
		// lets a Generic recurrence sneak through under another name.
		if _, explicitMu := current.value.(*Recursive); explicitMu {
			continue
		}
		if generic, ok := current.value.(*Generic); ok {
			to, found := index[generic]
			if !found {
				return nil, ErrInvalidStaticGenericRecurrence
			}
			out = append(out, staticGenericEdge{to: to, guarded: current.guarded})
			continue
		}
		if instantiated, ok := current.value.(*Instantiated); ok {
			if instantiated == nil || instantiated.Generic == nil || len(instantiated.TypeArgs) != len(instantiated.Generic.TypeParams) {
				return nil, ErrInvalidStaticGenericRecurrence
			}
		}
		guarded := current.guarded || staticGenericGuard(current.value)
		WalkChildren(current.value, func(child Type) bool {
			stack = append(stack, staticGenericFrame{value: child, guarded: guarded})
			return false
		})
	}
	return out, nil
}

type staticGenericFrame struct {
	value   Type
	guarded bool
}

type staticGenericEdge struct {
	to      int
	guarded bool
}

type staticGenericVisit struct {
	pointer uintptr
	guarded bool
}

func staticGenericGuard(value Type) bool {
	switch value.(type) {
	case *Generic, *Instantiated, *TypeParam, *Alias, *Meta, *Annotated, *Recursive:
		return false
	default:
		// Every remaining existing constructor is a semantic layer around its
		// child. Optional, for example, carries the nil alternative. Recursive
		// is handled as a delimiter by directGenericEdges, not as a productive
		// wrapper.
		return true
	}
}

func staticNilType(value Type) bool {
	if value == nil {
		return true
	}
	reflection := reflect.ValueOf(value)
	return reflection.Kind() == reflect.Pointer && reflection.IsNil()
}

func staticTypePointer(value Type) (uintptr, bool) {
	if staticNilType(value) {
		return 0, false
	}
	reflection := reflect.ValueOf(value)
	if reflection.Kind() != reflect.Pointer {
		return 0, false
	}
	return reflection.Pointer(), true
}

func staticGenericComponents(edges [][]staticGenericEdge) ([]int, []int) {
	forward := make([][]int, len(edges))
	reverse := make([][]int, len(edges))
	for from, outgoing := range edges {
		for _, edge := range outgoing {
			forward[from] = append(forward[from], edge.to)
			reverse[edge.to] = append(reverse[edge.to], from)
		}
	}
	seen := make([]bool, len(edges))
	order := make([]int, 0, len(edges))
	for root := range edges {
		if seen[root] {
			continue
		}
		stack := []staticGenericDFS{{node: root}}
		seen[root] = true
		for len(stack) != 0 {
			frame := &stack[len(stack)-1]
			if frame.edge < len(forward[frame.node]) {
				next := forward[frame.node][frame.edge]
				frame.edge++
				if !seen[next] {
					seen[next] = true
					stack = append(stack, staticGenericDFS{node: next})
				}
				continue
			}
			order = append(order, frame.node)
			stack = stack[:len(stack)-1]
		}
	}
	component := make([]int, len(edges))
	for i := range component {
		component[i] = -1
	}
	var sizes []int
	for i := len(order) - 1; i >= 0; i-- {
		root := order[i]
		if component[root] >= 0 {
			continue
		}
		id := len(sizes)
		stack := []int{root}
		component[root] = id
		size := 0
		for len(stack) != 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			for _, parent := range reverse[current] {
				if component[parent] < 0 {
					component[parent] = id
					stack = append(stack, parent)
				}
			}
		}
		sizes = append(sizes, size)
	}
	return component, sizes
}

type staticGenericDFS struct{ node, edge int }

// validateStaticFormalOwnership admits a graph in which every formal occurrence
// is owned by a lexical Generic or Function binder, or by the exact supplied
// external scope.
//
// The verdict is derived from the free formals of each node rather than from
// the binder chain that reached it. A formal is free in the graph exactly when
// some path reaches it outside its binder, so the two formulations decide the
// same graphs; deriving the free set bottom up gives each node one identity
// instead of one identity per distinct enclosing chain, which is what keeps a
// shared declaration reached under many chains from being re-entered once per
// chain.
func validateStaticFormalOwnership(root Type, scope staticFormalScope) error {
	if root == nil {
		return nil
	}
	walk := formalOwnershipWalk{memo: make(map[uintptr]formalOwnershipEntry)}
	for {
		free, err := walk.freeFormals(root)
		if err != nil {
			return err
		}
		// A cyclic graph reads an in-progress node at its current
		// approximation, so the free sets are a least fixed point and the walk
		// repeats until one round adds nothing. An acyclic graph converges in
		// its first round by construction.
		if !walk.cyclic || !walk.changed {
			for _, param := range free {
				if !scope.permits(param) {
					return ErrInvalidStaticGenericRecurrence
				}
			}
			return nil
		}
	}
}

// formalOwnershipWalk derives the free formals of every reachable node exactly
// once per round.
type formalOwnershipWalk struct {
	memo    map[uintptr]formalOwnershipEntry
	stack   []formalOwnershipFrame
	round   uint32
	cyclic  bool
	changed bool
}

// formalOwnershipEntry is one node's current approximation: free is the set it
// last produced, round is the round that produced it, and live marks the node
// as an ancestor of the live path so a back edge reads the approximation
// instead of re-entering the node.
type formalOwnershipEntry struct {
	free  []*TypeParam
	round uint32
	live  bool
}

type formalOwnershipFrame struct {
	value     Type
	binder    []*TypeParam
	free      []*TypeParam
	pointer   uintptr
	parent    int32
	expanded  bool
	addressed bool
}

func (w *formalOwnershipWalk) freeFormals(root Type) ([]*TypeParam, error) {
	w.round++
	w.changed = false
	w.stack = append(w.stack[:0], formalOwnershipFrame{value: root, parent: -1})
	var result []*TypeParam
	for len(w.stack) != 0 {
		index := len(w.stack) - 1
		if !w.stack[index].expanded {
			done, err := w.expand(index)
			if err != nil {
				return nil, err
			}
			if !done {
				continue
			}
		}
		w.settle(index)
		free := w.stack[index].free
		parent := w.stack[index].parent
		w.stack = w.stack[:index]
		if parent < 0 {
			result = free
			continue
		}
		w.stack[parent].free = formalUnion(w.stack[parent].free, free)
	}
	return result, nil
}

// expand admits the node, resolves it against the memo, and pushes the child
// frames it owns. It reports whether the frame is already finished, which is
// the case for a memo hit, a back edge, and a node without children.
//
// A transparent wrapper carries presentation only. Binder detection therefore
// reads the node beneath it, which is also the node WalkChildren enumerates,
// so one inventory decides both the binder and the children of a frame.
func (w *formalOwnershipWalk) expand(index int) (bool, error) {
	value := w.stack[index].value
	if staticNilType(value) {
		return false, ErrInvalidStaticGenericRecurrence
	}
	value = UnwrapTransparentWrappers(value)
	if staticNilType(value) {
		return false, ErrInvalidStaticGenericRecurrence
	}
	w.stack[index].value = value
	if pointer, ok := staticTypePointer(value); ok {
		entry := w.memo[pointer]
		if entry.round == w.round {
			w.stack[index].free = entry.free
			return true, nil
		}
		if entry.live {
			w.cyclic = true
			w.stack[index].free = entry.free
			return true, nil
		}
		entry.live = true
		w.memo[pointer] = entry
		w.stack[index].pointer, w.stack[index].addressed = pointer, true
	}
	w.stack[index].expanded = true
	parent := int32(index)
	switch node := value.(type) {
	case *TypeParam:
		w.stack[index].free = []*TypeParam{node}
	case *Generic:
		if err := formalBinder(node.TypeParams); err != nil {
			return false, err
		}
		w.stack[index].binder = node.TypeParams
	case *Function:
		if err := formalBinder(node.TypeParams); err != nil {
			return false, err
		}
		w.stack[index].binder = node.TypeParams
	}
	WalkChildren(value, func(child Type) bool {
		w.stack = append(w.stack, formalOwnershipFrame{value: child, parent: parent})
		return false
	})
	return len(w.stack) == index+1, nil
}

// settle closes one node: its binder removes the formals it owns, and the
// result becomes the node's approximation for the next round.
func (w *formalOwnershipWalk) settle(index int) {
	free := formalRemove(w.stack[index].free, w.stack[index].binder)
	w.stack[index].free = free
	if !w.stack[index].addressed {
		return
	}
	pointer := w.stack[index].pointer
	if !formalSetEqual(w.memo[pointer].free, free) {
		w.changed = true
	}
	w.memo[pointer] = formalOwnershipEntry{free: free, round: w.round}
}

// formalBinder admits one declaration's formal list. A binder that repeats or
// omits a formal has no lexical ownership to give.
func formalBinder(params []*TypeParam) error {
	for i, param := range params {
		if param == nil {
			return ErrInvalidStaticGenericRecurrence
		}
		for _, earlier := range params[:i] {
			if earlier == param {
				return ErrInvalidStaticGenericRecurrence
			}
		}
	}
	return nil
}

// A free formal set stays a small unordered slice: it is bounded by the binder
// nesting depth above the node, so linear membership beats a map allocation per
// node.
func formalUnion(into, from []*TypeParam) []*TypeParam {
	for _, param := range from {
		if !formalContains(into, param) {
			into = append(into, param)
		}
	}
	return into
}

func formalRemove(set []*TypeParam, bound []*TypeParam) []*TypeParam {
	if len(set) == 0 || len(bound) == 0 {
		return set
	}
	kept := set[:0:0]
	for _, param := range set {
		if !formalContains(bound, param) {
			kept = append(kept, param)
		}
	}
	return kept
}

func formalContains(set []*TypeParam, param *TypeParam) bool {
	for _, current := range set {
		if current == param {
			return true
		}
	}
	return false
}

func formalSetEqual(first, second []*TypeParam) bool {
	if len(first) != len(second) {
		return false
	}
	for _, param := range first {
		if !formalContains(second, param) {
			return false
		}
	}
	return true
}
