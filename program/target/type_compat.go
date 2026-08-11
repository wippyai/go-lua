package target

import (
	"context"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

// typePair is Seal-local cold memoization. It is deliberately keyed by frozen
// type bytes, not by pointers or a second type identity.
type typePair struct {
	source      string
	destination string
}

func (d *operationDraft) decodedType(key string) (typ.Type, bool) {
	if value, ok := d.decoded[key]; ok {
		return value, true
	}
	encoded, ok := d.types[key]
	if !ok {
		return nil, false
	}
	value, err := typ.DecodeCanonicalFormals(context.Background(), encoded, d.formals)
	if err != nil {
		return nil, false
	}
	d.decoded[key] = value
	return value, true
}

// typeAssignable is the one Target-side type transport law. It allows the
// explicit gradual Any boundary, never widens into Never, and refuses an
// unproved nonidentical destination formal rather than treating its bound as
// its identity. The memo is cold Seal state and is discarded with the draft.
func (d *operationDraft) typeAssignable(sourceKey, destinationKey string) bool {
	if sourceKey == destinationKey {
		return true
	}
	pair := typePair{source: sourceKey, destination: destinationKey}
	if result, ok := d.assignable[pair]; ok {
		return result
	}
	source, sourceOK := d.decodedType(sourceKey)
	destination, destinationOK := d.decodedType(destinationKey)
	result := sourceOK && destinationOK && typeAssignableValue(source, destination)
	d.assignable[pair] = result
	return result
}

func (d *operationDraft) typeAccepts(source typ.Type, destinationKey string) bool {
	destination, ok := d.decodedType(destinationKey)
	return ok && typeAssignableValue(source, destination)
}

func typeAssignableValue(source, destination typ.Type) bool {
	if source == nil || destination == nil {
		return false
	}
	if typ.TypeEquals(source, destination) || typ.IsNever(source) {
		return true
	}
	if typ.IsNever(destination) {
		return false
	}
	if typ.IsAny(destination) || typ.IsAny(source) {
		return true
	}
	// A constraint is not a value identity. The one exact-equality case above
	// is allowed; all other external-formal destinations require a later
	// explicit instantiation proof.
	if typ.ContainsTypeParam(destination) {
		return false
	}
	return subtype.IsSubtype(source, destination)
}

func (d *operationDraft) admitsAdmission(key string, admission Admission) bool {
	value, ok := d.decodedType(key)
	if !ok || typ.IsNever(value) {
		return false
	}
	if typ.IsAny(value) {
		return true
	}
	switch admission {
	case DirectFunction:
		return admitsDirectFunction(value)
	case OrdinaryCallable:
		_, callable := typecall.Callable(value)
		return callable
	default:
		return false
	}
}

// admitsDirectFunction deliberately excludes records callable only through
// __call. A Produced result is a function value, not an ordinary call target.
func admitsDirectFunction(value typ.Type) bool {
	if value == nil || typ.IsNever(value) {
		return false
	}
	if typ.IsAny(value) {
		return true
	}
	return admitsRuntimeEvidence(value, directFunctionLeaf)
}

func (d *operationDraft) freshCompatible(key string, fresh FreshKind) bool {
	value, ok := d.decodedType(key)
	if !ok || value == nil || typ.IsNever(value) {
		return false
	}
	switch fresh {
	case FreshTable:
		return admitsRuntimeEvidence(value, directTableLeaf)
	case FreshFunction:
		return admitsRuntimeEvidence(value, directFunctionLeaf)
	case FreshThread, FreshUserdata, FreshError, FreshReflection:
		return lacksConcreteRuntimeContradiction(value, fresh)
	default:
		return false
	}
}

// runtimeSemanticCheck is the one wrapper-unfolding law for Target runtime
// checks. Recursive binders are unfolded coinductively and instantiated
// generics through the type package's finite root expansion. A repeated
// recursive binder is a semantic cycle, not a reason to recurse indefinitely;
// callers choose its polarity: direct construction evidence requires a
// productive witness, while an unmarked fresh kind may remain opaque.
//
// A generative generic recurrence (G<T> = G<Array<T>>) has no finite regular
// head. It remains symbolic at that boundary rather than being repeatedly
// expanded. This is an exact termination boundary, not a depth or fuel limit.
type runtimeSemanticCheck struct {
	active         map[typ.Type]bool
	activeGenerics map[*typ.Generic]bool
	cycle          bool
	followBounds   bool
	leaf           func(typ.Type) bool
}

type semanticFrameState uint8

const (
	semanticFrameEnter semanticFrameState = iota
	semanticFrameUnary
	semanticFrameUnion
	semanticFrameIntersection
)

// semanticFrame is an explicit DFS continuation. It keeps Seal's wrapper
// inspection independent of the Go call stack even for machine-authored
// wrapper chains. The active fields are released only when this exact path
// completes, preserving coinductive cycle polarity across siblings.
type semanticFrame struct {
	value         typ.Type
	state         semanticFrameState
	members       []typ.Type
	next          int
	activeType    typ.Type
	activeGeneric *typ.Generic
}

func runtimeSemanticCheckValue(value typ.Type, leaf func(typ.Type) bool, cycle, followBounds bool) bool {
	return runtimeSemanticCheck{
		active:         make(map[typ.Type]bool),
		activeGenerics: make(map[*typ.Generic]bool),
		cycle:          cycle,
		followBounds:   followBounds,
		leaf:           leaf,
	}.check(value)
}

// check evaluates the wrapper algebra with an explicit continuation stack.
// Union is existential, intersection universal, and a repeated active node is
// resolved according to the caller's stated cycle polarity. No traversal
// depth, fuel counter, or host-stack recursion participates in the law.
func (check runtimeSemanticCheck) check(value typ.Type) bool {
	stack := []semanticFrame{{value: value}}
	result := false
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		switch top.state {
		case semanticFrameEnter:
			current := top.value
			if current == nil || typ.IsNever(current) {
				check.complete(&stack, false, &result)
				continue
			}
			if typ.IsAny(current) {
				check.complete(&stack, true, &result)
				continue
			}
			if check.active[current] {
				check.complete(&stack, check.cycle, &result)
				continue
			}
			check.active[current] = true
			top.activeType = current
			switch current := current.(type) {
			case *typ.Annotated:
				top.state, top.value = semanticFrameUnary, current.Inner
				stack = append(stack, semanticFrame{value: current.Inner})
			case *typ.Alias:
				child := current.UnaliasedTarget()
				top.state, top.value = semanticFrameUnary, child
				stack = append(stack, semanticFrame{value: child})
			case *typ.Optional:
				top.state, top.value = semanticFrameUnary, current.Inner
				stack = append(stack, semanticFrame{value: current.Inner})
			case *typ.Recursive:
				top.state, top.value = semanticFrameUnary, current.Body
				stack = append(stack, semanticFrame{value: current.Body})
			case *typ.TypeParam:
				if check.followBounds && current.Constraint != nil {
					top.state, top.value = semanticFrameUnary, current.Constraint
					stack = append(stack, semanticFrame{value: current.Constraint})
					continue
				}
				check.complete(&stack, check.leaf(current), &result)
			case *typ.Instantiated:
				if current.Generic == nil || len(current.TypeArgs) != len(current.Generic.TypeParams) || current.Generic.Body == nil {
					check.complete(&stack, check.leaf(current), &result)
					continue
				}
				if check.activeGenerics[current.Generic] {
					check.complete(&stack, check.cycle, &result)
					continue
				}
				check.activeGenerics[current.Generic] = true
				top.activeGeneric = current.Generic
				expanded := subst.ExpandInstantiatedRoot(current)
				if expanded == current {
					check.complete(&stack, check.leaf(current), &result)
					continue
				}
				top.state, top.value = semanticFrameUnary, expanded
				stack = append(stack, semanticFrame{value: expanded})
			case *typ.Union:
				if len(current.Members) == 0 {
					check.complete(&stack, false, &result)
					continue
				}
				top.state, top.members, top.next = semanticFrameUnion, current.Members, 1
				stack = append(stack, semanticFrame{value: current.Members[0]})
			case *typ.Intersection:
				if len(current.Members) == 0 {
					check.complete(&stack, false, &result)
					continue
				}
				top.state, top.members, top.next = semanticFrameIntersection, current.Members, 1
				stack = append(stack, semanticFrame{value: current.Members[0]})
			default:
				check.complete(&stack, check.leaf(current), &result)
			}
		default:
			panic("target: invalid semantic wrapper frame")
		}
	}
	return result
}

func (check runtimeSemanticCheck) complete(stack *[]semanticFrame, childResult bool, result *bool) {
	for {
		index := len(*stack) - 1
		frame := (*stack)[index]
		if frame.activeType != nil {
			delete(check.active, frame.activeType)
		}
		if frame.activeGeneric != nil {
			delete(check.activeGenerics, frame.activeGeneric)
		}
		*stack = (*stack)[:index]
		if len(*stack) == 0 {
			*result = childResult
			return
		}
		parent := &(*stack)[len(*stack)-1]
		switch parent.state {
		case semanticFrameUnary:
			continue
		case semanticFrameUnion:
			if childResult {
				continue
			}
			if parent.next == len(parent.members) {
				childResult = false
				continue
			}
			next := parent.members[parent.next]
			parent.next++
			*stack = append(*stack, semanticFrame{value: next})
			return
		case semanticFrameIntersection:
			if !childResult {
				continue
			}
			if parent.next == len(parent.members) {
				childResult = true
				continue
			}
			next := parent.members[parent.next]
			parent.next++
			*stack = append(*stack, semanticFrame{value: next})
			return
		default:
			panic("target: invalid semantic wrapper parent frame")
		}
	}
}

// admitsRuntimeEvidence uses neutral typ structural kinds at leaves. A union
// needs one compatible construction alternative; an intersection must retain
// compatibility through every conjunct. A recursive cycle without a reachable
// compatible head has no construction witness and is rejected.
func admitsRuntimeEvidence(value typ.Type, leaf func(typ.Type) bool) bool {
	return runtimeSemanticCheckValue(value, leaf, false, false)
}

func directFunctionLeaf(value typ.Type) bool {
	return value != nil && value.Kind() == kind.Function
}

func directTableLeaf(value typ.Type) bool {
	if value == nil {
		return false
	}
	if typ.IsBuiltinTableTopMarker(value) {
		return true
	}
	switch value.Kind() {
	case kind.Record, kind.Array, kind.Tuple, kind.Map, kind.ReadonlyMap:
		return true
	default:
		return false
	}
}

// lacksConcreteRuntimeContradiction is intentionally conservative for fresh
// categories lacking a static marker. A known primitive/table/function cannot
// be a fresh Thread/Userdata/Error/Reflection; opaque and type-parameterized
// values remain admitted until their own Target law supplies a marker.
func lacksConcreteRuntimeContradiction(value typ.Type, fresh FreshKind) bool {
	if value == nil || typ.IsNever(value) {
		return false
	}
	return runtimeSemanticCheckValue(value, func(value typ.Type) bool {
		if _, meta := value.(*typ.Meta); meta {
			return fresh == FreshReflection
		}
		return typ.IsAny(value) || typ.IsUnknown(value) || !knownConcreteRuntimeKind(value)
	}, true, true)
}

func knownConcreteRuntimeKind(value typ.Type) bool {
	if value == nil {
		return false
	}
	if typ.IsBuiltinTableTopMarker(value) {
		return true
	}
	if literal, ok := value.(*typ.Literal); ok {
		switch literal.Base {
		case kind.Boolean, kind.Integer, kind.Number, kind.String:
			return true
		}
	}
	switch value.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Function,
		kind.Record, kind.Array, kind.Tuple, kind.Map, kind.ReadonlyMap:
		return true
	default:
		return false
	}
}
