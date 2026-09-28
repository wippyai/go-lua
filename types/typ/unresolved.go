package typ

import (
	"sync"

	"github.com/wippyai/go-lua/types/kind"
)

var finalitySeenPool = sync.Pool{
	New: func() any { return make(map[Type]bool, 64) },
}

// IsUnresolved reports an inference hole, distinct from a dynamic value.
func IsUnresolved(t Type) bool {
	return t != nil && t.Kind() == kind.Unresolved
}

// IsFinal reports whether every reachable inference position has evidence.
// Unknown and Any are final: both describe values, rather than work left to do.
func IsFinal(t Type) bool {
	if t == nil || IsUnresolved(t) {
		return false
	}
	switch t.(type) {
	case *Annotated, *Alias, *Optional, *Array, *Map, *Tuple, *Union,
		*Intersection, *Record, *Function, *Recursive, *Meta,
		*TypeParam, *Generic, *Instantiated, *Interface, *Sum,
		*FieldAccess, *IndexAccess:
	default:
		return true
	}
	var state finalityState
	final := state.visit(t)
	if state.seen != nil {
		clear(state.seen)
		finalitySeenPool.Put(state.seen)
	}
	return final
}

type finalityState struct {
	// A finality walk is an AND over reachable nodes. Keep completed nodes as
	// well as ancestors so shared subgraphs are checked only once.
	visited [32]Type
	count   int
	seen    map[Type]bool
}

func (s *finalityState) visit(t Type) bool {
	if t == nil || IsUnresolved(t) {
		return false
	}
	if s.seen != nil {
		if s.seen[t] {
			return true
		}
		s.seen[t] = true
	} else {
		for i := 0; i < s.count; i++ {
			if s.visited[i] == t {
				return true
			}
		}
		if s.count == len(s.visited) {
			s.seen = finalitySeenPool.Get().(map[Type]bool)
			for _, visited := range s.visited {
				s.seen[visited] = true
			}
			s.seen[t] = true
		} else {
			s.visited[s.count] = t
			s.count++
		}
	}

	switch v := t.(type) {
	case *Annotated:
		return s.visit(v.Inner)
	case *Alias:
		return s.visit(v.Target)
	case *Optional:
		return s.visit(v.Inner)
	case *Array:
		return s.visit(v.Element)
	case *Map:
		return s.visit(v.Key) && s.visit(v.Value)
	case *Tuple:
		for _, e := range v.Elements {
			if !s.visit(e) {
				return false
			}
		}
	case *Union:
		for _, m := range v.Members {
			if !s.visit(m) {
				return false
			}
		}
	case *Intersection:
		for _, m := range v.Members {
			if !s.visit(m) {
				return false
			}
		}
	case *Record:
		for _, f := range v.Fields {
			if !s.visit(f.Type) {
				return false
			}
		}
		if v.MapKey != nil && (!s.visit(v.MapKey) || !s.visit(v.MapValue)) {
			return false
		}
		if v.Metatable != nil && !s.visit(v.Metatable) {
			return false
		}
	case *Function:
		for _, p := range v.TypeParams {
			if p != nil && p.Constraint != nil && !s.visit(p.Constraint) {
				return false
			}
		}
		for _, p := range v.Params {
			if !s.visit(p.Type) {
				return false
			}
		}
		for _, r := range v.Returns {
			if !s.visit(r) {
				return false
			}
		}
		if v.Variadic != nil && !s.visit(v.Variadic) {
			return false
		}
	case *Recursive:
		return v.Body != nil && s.visit(v.Body)
	case *Meta:
		return s.visit(v.Of)
	case *TypeParam:
		return v.Constraint == nil || s.visit(v.Constraint)
	case *Generic:
		for _, p := range v.TypeParams {
			if p != nil && !s.visit(p) {
				return false
			}
		}
		return v.Body != nil && s.visit(v.Body)
	case *Instantiated:
		if v.Generic == nil || !s.visit(v.Generic) {
			return false
		}
		for _, arg := range v.TypeArgs {
			if !s.visit(arg) {
				return false
			}
		}
	case *Interface:
		for _, method := range v.Methods {
			if !s.visit(method.Type) {
				return false
			}
		}
	case *Sum:
		for _, variant := range v.Variants {
			for _, arg := range variant.Types {
				if !s.visit(arg) {
					return false
				}
			}
		}
	case *FieldAccess:
		return s.visit(v.Base)
	case *IndexAccess:
		return s.visit(v.Base) && s.visit(v.Index)
	}
	return true
}

// Resolve fills only pending positions, matching composite positions in the
// current estimate with the corresponding positions in evidence. A final leaf
// is never replaced by a later estimate.
func Resolve(current, evidence Type) Type {
	if IsFinal(current) {
		return current
	}
	return resolvePending(current, evidence, make(map[resolvePair]Type))
}

type resolvePair struct{ current, evidence Type }

func resolvePending(current, evidence Type, seen map[resolvePair]Type) Type {
	pair := resolvePair{current, evidence}
	if prior, ok := seen[pair]; ok {
		return prior
	}
	seen[pair] = current
	resolved := resolvePendingNode(current, evidence, seen)
	seen[pair] = resolved
	return resolved
}

func resolvePendingNode(current, evidence Type, seen map[resolvePair]Type) Type {
	if current == nil || evidence == nil || IsUnresolved(evidence) {
		return current
	}
	if IsUnresolved(current) {
		return evidence
	}
	if a, ok := current.(*Annotated); ok {
		other := evidence
		if b, ok := evidence.(*Annotated); ok {
			other = b.Inner
		}
		inner := resolvePending(a.Inner, other, seen)
		if inner == a.Inner {
			return current
		}
		return NewAnnotated(inner, a.Annotations)
	}
	if a, ok := current.(*Alias); ok {
		other := evidence
		if b, ok := evidence.(*Alias); ok {
			other = b.Target
		}
		target := resolvePending(a.Target, other, seen)
		if target == a.Target {
			return current
		}
		return NewAlias(a.Name, target)
	}
	switch a := current.(type) {
	case *Optional:
		other := evidence
		if b, ok := evidence.(*Optional); ok {
			other = b.Inner
		}
		inner := resolvePending(a.Inner, other, seen)
		if inner == a.Inner {
			return current
		}
		return NewOptional(inner)
	case *Array:
		b, ok := evidence.(*Array)
		if !ok {
			return current
		}
		elem := resolvePending(a.Element, b.Element, seen)
		if elem == a.Element {
			return current
		}
		return NewArray(elem)
	case *Map:
		b, ok := evidence.(*Map)
		if !ok {
			return current
		}
		key, value := resolvePending(a.Key, b.Key, seen), resolvePending(a.Value, b.Value, seen)
		if key == a.Key && value == a.Value {
			return current
		}
		return NewMap(key, value)
	case *Tuple:
		b, ok := evidence.(*Tuple)
		if !ok {
			return current
		}
		elems := append([]Type(nil), a.Elements...)
		changed := false
		for i := range elems {
			if i < len(b.Elements) {
				elems[i] = resolvePending(elems[i], b.Elements[i], seen)
				changed = changed || elems[i] != a.Elements[i]
			}
		}
		if !changed {
			return current
		}
		return NewTuple(elems...)
	case *Record:
		b, ok := evidence.(*Record)
		if !ok {
			return current
		}
		fields := append([]Field(nil), a.Fields...)
		changed := false
		for i := range fields {
			if bf := b.GetField(fields[i].Name); bf != nil {
				fields[i].Type = resolvePending(fields[i].Type, bf.Type, seen)
				changed = changed || fields[i].Type != a.Fields[i].Type
			}
		}
		key, value := a.MapKey, a.MapValue
		if a.HasMapComponent() && b.HasMapComponent() {
			key, value = resolvePending(key, b.MapKey, seen), resolvePending(value, b.MapValue, seen)
			changed = changed || key != a.MapKey || value != a.MapValue
		}
		if !changed {
			return current
		}
		return buildRecordTypeDeclared(fields, a.Metatable, key, value, a.Open, a.Declared, a.Complete, true)
	case *Union:
		// Union members are separate paths. This API has no source identity
		// with which to pair them to evidence, even when their kinds match.
		// Final evidence for the whole position supersedes a union that is
		// still pending on one of its paths.
		if a.Contains(Unresolved) && IsFinal(evidence) {
			return evidence
		}
		return current
	case *Function:
		b, ok := evidence.(*Function)
		if !ok {
			return current
		}
		params := append([]Param(nil), a.Params...)
		rets := append([]Type(nil), a.Returns...)
		changed := false
		for i := range params {
			if i < len(b.Params) {
				params[i].Type = resolvePending(params[i].Type, b.Params[i].Type, seen)
				changed = changed || params[i].Type != a.Params[i].Type
			}
		}
		for i := range rets {
			if i < len(b.Returns) {
				rets[i] = resolvePending(rets[i], b.Returns[i], seen)
				changed = changed || rets[i] != a.Returns[i]
			}
		}
		variadic := a.Variadic
		if variadic != nil && b.Variadic != nil {
			variadic = resolvePending(variadic, b.Variadic, seen)
			changed = changed || variadic != a.Variadic
		}
		if !changed {
			return current
		}
		return buildFunctionType(a.TypeParams, params, variadic, rets, a.Effects, a.Spec, a.Refinement)
	}
	return current
}

// DropPendingAlternatives removes the pending alternatives of every union in t,
// keeping the alternatives that carry evidence. An iterative inference round
// recomputes every pending read of the previous round, so its result
// supersedes those alternatives. Returns nil when t is wholly pending.
func DropPendingAlternatives(t Type) Type {
	if t == nil || IsUnresolved(t) {
		return nil
	}
	if IsFinal(t) {
		return t
	}
	return Rewrite(t, func(node Type) (Type, bool) {
		union, ok := node.(*Union)
		if !ok || !union.Contains(Unresolved) {
			return nil, false
		}
		members := make([]Type, 0, len(union.Members))
		for _, member := range union.Members {
			if !IsUnresolved(member) {
				members = append(members, DropPendingAlternatives(member))
			}
		}
		return NewUnion(members...), true
	})
}

// Finalize converts remaining inference holes at a public boundary. The
// checker retains its gradual behavior for unannotated values elsewhere.
func Finalize(t Type) Type {
	if t == nil {
		return Unknown
	}
	if IsFinal(t) {
		return t
	}
	return Rewrite(t, func(node Type) (Type, bool) {
		if IsUnresolved(node) {
			return Unknown, true
		}
		if recursive, ok := node.(*Recursive); ok && (recursive.Body == nil || !IsFinal(recursive)) {
			return Unknown, true
		}
		if union, ok := node.(*Union); ok {
			for _, member := range union.Members {
				if IsUnresolved(member) {
					return Unknown, true
				}
			}
		}
		if optional, ok := node.(*Optional); ok && IsUnresolved(optional.Inner) {
			return Unknown, true
		}
		return nil, false
	})
}
