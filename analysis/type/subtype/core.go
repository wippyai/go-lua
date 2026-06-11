package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (c *checker) check(sub, super typ.Type, depth int) bool {
	if stopDepthPair(sub, super, depth) {
		return false
	}
	if sub == super {
		return true
	}

	if needsCycleGuard(sub.Kind()) && needsCycleGuard(super.Kind()) {
		if pair, ok := newTypePair(sub, super); ok {
			if c.memo != nil {
				if result, ok := c.memo[pair]; ok {
					return result
				}
			}
			if c.inProgress == nil {
				c.inProgress = make(map[typePair]bool)
			}
			if c.inProgress[pair] {
				return true
			}
			c.inProgress[pair] = true
			result := c.checkCore(sub, super, depth)
			delete(c.inProgress, pair)
			if c.memo == nil {
				c.memo = make(map[typePair]bool)
			}
			c.memo[pair] = result
			return result
		}
	}

	return c.checkCore(sub, super, depth)
}

func (c *checker) checkCore(sub, super typ.Type, depth int) bool {
	if sub == super {
		return true
	}

	if ref, ok := sub.(*typ.Ref); ok && ref.Module == "" {
		if a, ok := super.(*typ.Alias); ok && a.Name == ref.Name {
			return true
		}
		if r, ok := super.(*typ.Ref); ok && r.Module == "" && r.Name == ref.Name {
			return true
		}
		if r, ok := super.(*typ.Recursive); ok && r.Name == ref.Name {
			return true
		}
	}

	if ref, ok := super.(*typ.Ref); ok && ref.Module == "" {
		if a, ok := sub.(*typ.Alias); ok && a.Name == ref.Name {
			return true
		}
		if r, ok := sub.(*typ.Ref); ok && r.Module == "" && r.Name == ref.Name {
			return true
		}
		if r, ok := sub.(*typ.Recursive); ok && r.Name == ref.Name {
			return true
		}
	}

	if typ.SameNodeOrAcyclicEqual(sub, super) {
		return true
	}

	if a, ok := sub.(*typ.Alias); ok {
		return c.check(a.UnaliasedTarget(), super, depth+1)
	}
	if a, ok := super.(*typ.Alias); ok {
		return c.check(sub, a.UnaliasedTarget(), depth+1)
	}
	if a, ok := sub.(*typ.Annotated); ok {
		return c.check(a.Inner, super, depth+1)
	}
	if a, ok := super.(*typ.Annotated); ok {
		return c.check(sub, a.Inner, depth+1)
	}
	if r, ok := sub.(*typ.Recursive); ok && super.Kind() != kind.Recursive && r.Body != nil && r.Body != r {
		return c.check(r.Body, super, depth+1)
	}
	if r, ok := super.(*typ.Recursive); ok && sub.Kind() != kind.Recursive && r.Body != nil && r.Body != r {
		return c.check(sub, r.Body, depth+1)
	}

	subInst, subIsInst := sub.(*typ.Instantiated)
	superInst, superIsInst := super.(*typ.Instantiated)
	if subIsInst && superIsInst && subInst.Generic != nil && superInst.Generic != nil {
		if typ.TypeEquals(subInst.Generic, superInst.Generic) {
			return c.checkInstantiated(subInst, superInst, depth)
		}
	}
	if subIsInst {
		expanded := subst.ExpandInstantiated(subInst)
		if expanded != nil && expanded != sub {
			return c.check(expanded, subst.Self(super, subInst), depth+1)
		}
	}
	if superIsInst {
		expanded := subst.ExpandInstantiated(superInst)
		if expanded != nil && expanded != super {
			return c.check(subst.Self(sub, superInst), expanded, depth+1)
		}
	}

	if typ.IsNever(sub) {
		return true
	}
	if typ.IsNever(super) {
		return false
	}
	if typ.IsAny(super) || typ.IsUnknown(super) || isOptionalTop(super) {
		return true
	}
	if typ.IsAny(sub) {
		if typetable.IsBuiltinTopMarker(super) {
			return true
		}
		if i, ok := super.(*typ.Intersection); ok {
			for _, m := range i.Members {
				if !c.check(sub, m, depth+1) {
					return false
				}
			}
			return true
		}
		if u, ok := super.(*typ.Union); ok {
			for _, m := range u.Members {
				if c.check(sub, m, depth+1) {
					return true
				}
			}
			return false
		}
		return false
	}
	if typ.IsUnknown(sub) {
		return false
	}

	if u, ok := sub.(*typ.Union); ok {
		for _, m := range u.Members {
			if !c.check(m, super, depth+1) {
				return false
			}
		}
		return true
	}
	if u, ok := super.(*typ.Union); ok {
		if o, ok := sub.(*typ.Optional); ok {
			return c.check(o.Inner, super, depth+1) && c.checkNil(super, depth+1)
		}
		for _, m := range u.Members {
			if c.check(sub, m, depth+1) {
				return true
			}
		}
		return false
	}
	if i, ok := sub.(*typ.Intersection); ok {
		for _, m := range i.Members {
			if c.check(m, super, depth+1) {
				return true
			}
		}
		return false
	}
	if i, ok := super.(*typ.Intersection); ok {
		for _, m := range i.Members {
			if !c.check(sub, m, depth+1) {
				return false
			}
		}
		return true
	}

	if o, ok := super.(*typ.Optional); ok {
		if subOpt, ok := sub.(*typ.Optional); ok {
			return c.check(subOpt.Inner, o.Inner, depth+1)
		}
		if sub.Kind() == kind.Nil {
			return true
		}
		return c.check(sub, o.Inner, depth+1)
	}
	if o, ok := sub.(*typ.Optional); ok {
		return c.checkNil(super, depth+1) && c.check(o.Inner, super, depth+1)
	}

	if ok, handled := checkTableTop(sub, super); handled {
		return ok
	}

	if r, ok := sub.(*typ.Record); ok && len(r.Fields) == 0 && len(r.StaticMembers) == 0 {
		if super.Kind() == kind.Array || super.Kind() == kind.Map || super.Kind() == kind.ReadonlyMap {
			return true
		}
	}

	if r, ok := sub.(*typ.Record); ok {
		if m, ok := super.(*typ.Map); ok {
			return c.checkRecordToMap(r, m, depth+1)
		}
		if m, ok := super.(*typ.ReadonlyMap); ok {
			return c.checkRecordToReadonlyMap(r, m, depth+1)
		}
	}
	if m, ok := sub.(*typ.Map); ok {
		if r, ok := super.(*typ.Record); ok {
			return c.checkMapToRecord(m, r, depth+1)
		}
		if view, ok := super.(*typ.ReadonlyMap); ok {
			return c.checkReadonlyMap(typetable.NewReadonlyMap(m.Key, m.Value), view, depth+1)
		}
	}
	if arr, ok := sub.(*typ.Array); ok {
		if m, ok := super.(*typ.Map); ok {
			return c.checkArrayToMap(arr, m, depth+1)
		}
		if view, ok := super.(*typ.ReadonlyMap); ok {
			return c.checkReadonlyMap(typetable.NewReadonlyMap(typ.Integer, arr.Element), view, depth+1)
		}
	}
	if tup, ok := sub.(*typ.Tuple); ok {
		if arr, ok := super.(*typ.Array); ok {
			return c.checkTupleToArray(tup, arr, depth+1)
		}
		if m, ok := super.(*typ.Map); ok {
			return c.checkTupleToMap(tup, m, depth+1)
		}
		if view, ok := super.(*typ.ReadonlyMap); ok {
			return c.checkTupleToReadonlyMap(tup, view, depth+1)
		}
	}
	if rec, ok := sub.(*typ.Record); ok {
		if iface, ok := super.(*typ.Interface); ok {
			return c.checkRecordToInterface(rec, iface, depth+1)
		}
	}

	if tp, ok := sub.(*typ.TypeParam); ok {
		if sp, ok := super.(*typ.TypeParam); ok {
			return typ.TypeEquals(tp, sp)
		}
		if tp.Constraint != nil {
			return c.check(tp.Constraint, super, depth+1)
		}
		return typ.IsAny(super)
	}
	if tp, ok := super.(*typ.TypeParam); ok {
		if tp.Constraint != nil {
			return c.check(sub, tp.Constraint, depth+1)
		}
		return true
	}

	if lit, ok := sub.(*typ.Literal); ok {
		switch lit.Base {
		case kind.Boolean:
			return super.Kind() == kind.Boolean
		case kind.Integer:
			return super.Kind() == kind.Integer || super.Kind() == kind.Number
		case kind.Number:
			return super.Kind() == kind.Number
		case kind.String:
			return super.Kind() == kind.String
		}
	}
	if sub.Kind() == kind.Integer && super.Kind() == kind.Number {
		return true
	}
	if sub.Kind() != super.Kind() {
		return false
	}

	switch sub.Kind() {
	case kind.Function:
		return c.checkFunction(sub.(*typ.Function), super.(*typ.Function), depth)
	case kind.Record:
		return c.checkRecord(sub.(*typ.Record), super.(*typ.Record), depth)
	case kind.Array:
		return c.checkArray(sub.(*typ.Array), super.(*typ.Array), depth)
	case kind.Map:
		return c.checkMap(sub.(*typ.Map), super.(*typ.Map), depth)
	case kind.ReadonlyMap:
		return c.checkReadonlyMap(sub.(*typ.ReadonlyMap), super.(*typ.ReadonlyMap), depth)
	case kind.Tuple:
		return c.checkTuple(sub.(*typ.Tuple), super.(*typ.Tuple), depth)
	case kind.Interface:
		return c.checkInterface(sub.(*typ.Interface), super.(*typ.Interface), depth)
	case kind.Instantiated:
		return c.checkInstantiated(sub.(*typ.Instantiated), super.(*typ.Instantiated), depth)
	case kind.Meta:
		return c.check(sub.(*typ.Meta).Of, super.(*typ.Meta).Of, depth+1)
	default:
		// Opaque or deferred same-kind nodes are accepted only by their own
		// equality relation, keeping unsupported structure closed.
		return typ.TypeEquals(sub, super)
	}
}

func (c *checker) checkNil(super typ.Type, depth int) bool {
	return c.check(typ.Nil, super, depth)
}

type typePair struct {
	sub   uintptr
	super uintptr
}

func newTypePair(sub, super typ.Type) (typePair, bool) {
	subPtr := nodeid.Pointer(sub)
	superPtr := nodeid.Pointer(super)
	if subPtr == 0 || superPtr == 0 {
		return typePair{}, false
	}
	return typePair{sub: subPtr, super: superPtr}, true
}

func needsCycleGuard(k kind.Kind) bool {
	switch k {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Self:
		return false
	default:
		return true
	}
}
