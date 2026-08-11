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
	return validateStaticGenericRecurrence(root, nil)
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
	return validateStaticGenericRecurrence(root, external)
}

func validateStaticGenericRecurrence(root Type, external map[*TypeParam]struct{}) error {
	// A nil canonical type is a valid closed bottom-level payload and carries
	// no generic recurrence.  This law adjudicates declarations, not a second
	// whole-type validity checker.
	if root == nil {
		return nil
	}
	if err := validateStaticFormalOwnership(root, external); err != nil {
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

func validateStaticFormalOwnership(root Type, external map[*TypeParam]struct{}) error {
	type scope struct {
		parent *scope
		params map[*TypeParam]struct{}
		binder uintptr
	}
	type frame struct {
		value Type
		scope *scope
	}
	type visit struct {
		pointer uintptr
		scope   *scope
	}
	scopeFor := func(parent *scope, binder Type, params []*TypeParam) (*scope, error) {
		pointer, ok := staticTypePointer(binder)
		if !ok {
			return nil, ErrInvalidStaticGenericRecurrence
		}
		for current := parent; current != nil; current = current.parent {
			if current.binder == pointer {
				return current, nil
			}
		}
		bound := make(map[*TypeParam]struct{}, len(params))
		for _, param := range params {
			if param == nil {
				return nil, ErrInvalidStaticGenericRecurrence
			}
			if _, duplicate := bound[param]; duplicate {
				return nil, ErrInvalidStaticGenericRecurrence
			}
			bound[param] = struct{}{}
		}
		return &scope{parent: parent, params: bound, binder: pointer}, nil
	}
	contains := func(current *scope, param *TypeParam) bool {
		for current != nil {
			if _, found := current.params[param]; found {
				return true
			}
			current = current.parent
		}
		return false
	}
	if root == nil {
		return nil
	}
	stack := []frame{{value: root}}
	seen := make(map[visit]struct{})
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if staticNilType(current.value) {
			return ErrInvalidStaticGenericRecurrence
		}
		if pointer, ok := staticTypePointer(current.value); ok {
			key := visit{pointer: pointer, scope: current.scope}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
		}
		switch value := current.value.(type) {
		case *TypeParam:
			if value == nil || !contains(current.scope, value) {
				if _, permitted := external[value]; !permitted {
					return ErrInvalidStaticGenericRecurrence
				}
			}
			if value == nil {
				return ErrInvalidStaticGenericRecurrence
			}
			if value.Constraint != nil {
				stack = append(stack, frame{value: value.Constraint, scope: current.scope})
			}
		case *Generic:
			next, err := scopeFor(current.scope, value, value.TypeParams)
			if err != nil {
				return err
			}
			if value.Body != nil {
				stack = append(stack, frame{value: value.Body, scope: next})
			}
			for _, param := range value.TypeParams {
				if param.Constraint != nil {
					stack = append(stack, frame{value: param.Constraint, scope: next})
				}
			}
		case *Function:
			next, err := scopeFor(current.scope, value, value.TypeParams)
			if err != nil {
				return err
			}
			if value.Variadic != nil {
				stack = append(stack, frame{value: value.Variadic, scope: next})
			}
			for _, param := range value.TypeParams {
				if param.Constraint != nil {
					stack = append(stack, frame{value: param.Constraint, scope: next})
				}
			}
			for _, param := range value.Params {
				if param.Type != nil {
					stack = append(stack, frame{value: param.Type, scope: next})
				}
			}
			for _, result := range value.Returns {
				if result != nil {
					stack = append(stack, frame{value: result, scope: next})
				}
			}
		default:
			WalkChildren(current.value, func(child Type) bool {
				stack = append(stack, frame{value: child, scope: current.scope})
				return false
			})
		}
	}
	return nil
}
