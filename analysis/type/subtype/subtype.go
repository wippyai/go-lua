package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// IsSubtype reports whether sub is a strict subtype of super.
func IsSubtype(sub, super typ.Type) bool {
	c := &checker{}
	return c.check(sub, super, 0)
}

func isOptionalTop(t typ.Type) bool {
	opt, ok := unwrap.Alias(t).(*typ.Optional)
	if !ok || opt == nil {
		return false
	}
	inner := unwrap.Alias(opt.Inner)
	return typ.IsAny(inner) || typ.IsUnknown(inner)
}

type checker struct {
	inProgress map[typePair]bool
	memo       map[typePair]bool
	gradual    bool
}

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

	if !isRecursiveRoot(sub) && !isRecursiveRoot(super) && sub.Hash() == super.Hash() && sub.Equals(super) {
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
		if subInst.Generic.Equals(superInst.Generic) {
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
		if c.gradual || unwrap.IsBuiltinTableTop(super) {
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

	if unwrap.IsBuiltinTableTop(super) {
		return isTableLikeType(sub)
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
			return tp.Equals(sp)
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
		return sub.Equals(super)
	}
}

func (c *checker) checkNil(super typ.Type, depth int) bool {
	return c.check(typ.Nil, super, depth)
}

func (c *checker) checkFunction(sub, super *typ.Function, depth int) bool {
	subReq := minRequiredArgs(sub)
	superReq := minRequiredArgs(super)
	if subReq > superReq || (super.Variadic == nil && subReq > len(super.Params)) {
		return false
	}
	if sub.Variadic == nil && len(super.Params) > len(sub.Params) {
		return false
	}

	maxParams := len(sub.Params)
	if len(super.Params) > maxParams {
		maxParams = len(super.Params)
	}
	for i := 0; i < maxParams; i++ {
		var subT, superT typ.Type
		if i < len(sub.Params) {
			subT = sub.Params[i].Type
		} else if sub.Variadic != nil {
			subT = sub.Variadic
		}
		if i < len(super.Params) {
			superT = super.Params[i].Type
		} else if super.Variadic != nil {
			superT = super.Variadic
		}
		if subT == nil || superT == nil {
			continue
		}
		if !c.check(superT, subT, depth+1) {
			return false
		}
	}
	if sub.Variadic != nil && super.Variadic != nil {
		if !c.check(super.Variadic, sub.Variadic, depth+1) {
			return false
		}
	}
	for i := 0; i < len(super.Returns); i++ {
		subReturn := typ.Nil
		if i < len(sub.Returns) {
			subReturn = sub.Returns[i]
		}
		if !c.check(subReturn, super.Returns[i], depth+1) {
			return false
		}
	}
	return true
}

func minRequiredArgs(fn *typ.Function) int {
	if fn == nil {
		return 0
	}
	required := 0
	for i, p := range fn.Params {
		if !p.Optional {
			required = i + 1
		}
	}
	return required
}

func (c *checker) checkRecord(sub, super *typ.Record, depth int) bool {
	for _, sf := range super.Fields {
		subField := sub.GetField(sf.Name)
		if subField == nil {
			if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
				return false
			}
			continue
		}
		if !c.checkRecordMember(subField.Type, subField.Optional, subField.Readonly, sf.Type, sf.Optional, sf.Readonly, depth+1) {
			return false
		}
	}
	for _, sm := range super.StaticMembers {
		subMember := sub.GetStaticMember(sm.Kind, sm.Name, sm.Index)
		if subMember == nil {
			if !sm.Optional && !unwrap.IsOptionalLike(sm.Type) {
				return false
			}
			continue
		}
		if !c.checkRecordMember(subMember.Type, subMember.Optional, subMember.Readonly, sm.Type, sm.Optional, sm.Readonly, depth+1) {
			return false
		}
	}

	if super.HasMapComponent() {
		if !sub.HasMapComponent() {
			return false
		}
		if !c.check(sub.MapKey, super.MapKey, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.MapValue, depth+1) {
			return false
		}
	}
	return c.metaSubtype(sub.Metatable, super.Metatable, depth+1)
}

func (c *checker) checkRecordMember(subType typ.Type, subOptional, subReadonly bool, superType typ.Type, superOptional, superReadonly bool, depth int) bool {
	if superOptional && subType != nil && subType.Kind() == kind.Nil {
		return true
	}
	effectiveSuper := superType
	if superOptional {
		effectiveSuper = typ.NewOptional(superType)
	}
	if superReadonly {
		if !c.check(subType, effectiveSuper, depth+1) {
			return false
		}
	} else {
		if subReadonly {
			return false
		}
		if !c.check(subType, effectiveSuper, depth+1) {
			return false
		}
		if !c.check(effectiveSuper, subType, depth+1) && !c.canWidenTo(subType, effectiveSuper, depth+1) {
			return false
		}
	}
	if !superOptional && !unwrap.IsOptionalLike(superType) && subOptional {
		return false
	}
	return true
}

func (c *checker) metaSubtype(subMT, superMT typ.Type, depth int) bool {
	if subMT == nil && superMT == nil {
		return true
	}
	subUnconstrained := subMT != nil && typetable.IsMetatableUnconstrained(subMT)
	superUnconstrained := superMT != nil && typetable.IsMetatableUnconstrained(superMT)
	if subUnconstrained && (superMT == nil || superUnconstrained) {
		return true
	}
	if superUnconstrained || (superMT != nil && typ.IsUnknown(superMT)) {
		return true
	}
	if subMT != nil && typ.IsUnknown(subMT) {
		return false
	}
	if subUnconstrained {
		return false
	}
	if subMT == nil || superMT == nil {
		return false
	}
	return c.check(subMT, superMT, depth)
}

func (c *checker) canWidenTo(narrow, wide typ.Type, depth int) bool {
	if stopDepthPair(narrow, wide, depth) {
		return false
	}
	wide = unwrap.Alias(wide)
	narrow = unwrap.Alias(narrow)

	if typ.IsAny(wide) {
		return true
	}
	if narrow.Kind() == kind.Nil {
		if _, ok := wide.(*typ.Optional); ok {
			return true
		}
		if u, ok := wide.(*typ.Union); ok {
			for _, m := range u.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
		}
	}
	if opt, ok := wide.(*typ.Optional); ok {
		if narrowOpt, ok := narrow.(*typ.Optional); ok {
			return c.check(narrowOpt.Inner, opt.Inner, depth+1) ||
				c.canWidenTo(narrowOpt.Inner, opt.Inner, depth+1)
		}
		if c.check(narrow, opt.Inner, depth+1) {
			return true
		}
	}
	if u, ok := narrow.(*typ.Union); ok {
		if len(u.Members) == 0 {
			return false
		}
		for _, m := range u.Members {
			if c.check(m, wide, depth+1) || c.canWidenTo(m, wide, depth+1) {
				continue
			}
			return false
		}
		return true
	}
	if u, ok := wide.(*typ.Union); ok {
		for _, m := range u.Members {
			if m.Kind() == kind.Literal {
				continue
			}
			if c.check(narrow, m, depth+1) || c.canWidenTo(narrow, m, depth+1) {
				return true
			}
		}
		return false
	}
	if narrow.Kind() == kind.Integer && wide.Kind() == kind.Number {
		return true
	}
	if lit, ok := narrow.(*typ.Literal); ok {
		wideInner := wide
		if opt, ok := wide.(*typ.Optional); ok {
			wideInner = unwrap.Alias(opt.Inner)
		}
		switch lit.Base {
		case kind.Boolean:
			return wideInner.Kind() == kind.Boolean
		case kind.String:
			return wideInner.Kind() == kind.String
		case kind.Integer:
			return wideInner.Kind() == kind.Integer || wideInner.Kind() == kind.Number
		case kind.Number:
			return wideInner.Kind() == kind.Number
		}
	}
	if subRec, ok := narrow.(*typ.Record); ok {
		if supRec, ok := wide.(*typ.Record); ok {
			return c.canWidenRecordTo(subRec, supRec, depth+1)
		}
	}
	if subMap, ok := narrow.(*typ.Map); ok {
		if supMap, ok := wide.(*typ.Map); ok {
			return c.canWidenMapTo(subMap, supMap, depth+1)
		}
	}
	if subArray, ok := narrow.(*typ.Array); ok {
		if supArray, ok := wide.(*typ.Array); ok {
			return c.check(subArray.Element, supArray.Element, depth+1) ||
				c.canWidenTo(subArray.Element, supArray.Element, depth+1)
		}
		if supMap, ok := wide.(*typ.Map); ok {
			return c.check(typ.Integer, supMap.Key, depth+1) &&
				(c.check(subArray.Element, supMap.Value, depth+1) ||
					c.canWidenTo(subArray.Element, supMap.Value, depth+1))
		}
	}
	if rec, ok := narrow.(*typ.Recursive); ok && rec.Body != nil && rec.Body != rec {
		return c.check(rec.Body, wide, depth+1) || c.canWidenTo(rec.Body, wide, depth+1)
	}
	if subTuple, ok := narrow.(*typ.Tuple); ok {
		if supTuple, ok := wide.(*typ.Tuple); ok {
			if len(subTuple.Elements) != len(supTuple.Elements) {
				return false
			}
			for i := range subTuple.Elements {
				if c.check(subTuple.Elements[i], supTuple.Elements[i], depth+1) ||
					c.canWidenTo(subTuple.Elements[i], supTuple.Elements[i], depth+1) {
					continue
				}
				return false
			}
			return true
		}
	}
	if subFn, ok := narrow.(*typ.Function); ok {
		if supFn, ok := wide.(*typ.Function); ok {
			if !c.functionParamsEquivalent(subFn, supFn, depth+1) {
				return false
			}
			for i := 0; i < len(supFn.Returns); i++ {
				subReturn := typ.Nil
				if i < len(subFn.Returns) {
					subReturn = subFn.Returns[i]
				}
				if c.check(subReturn, supFn.Returns[i], depth+1) || c.canWidenTo(subReturn, supFn.Returns[i], depth+1) {
					continue
				}
				return false
			}
			return true
		}
	}
	return false
}

func (c *checker) canWidenMapTo(narrow, wide *typ.Map, depth int) bool {
	if narrow == nil || wide == nil {
		return false
	}
	if !c.check(narrow.Key, wide.Key, depth+1) || !c.check(wide.Key, narrow.Key, depth+1) {
		return false
	}
	return c.check(narrow.Value, wide.Value, depth+1) ||
		c.canWidenTo(narrow.Value, wide.Value, depth+1)
}

func (c *checker) functionParamsEquivalent(a, b *typ.Function, depth int) bool {
	if a == nil || b == nil || len(a.Params) != len(b.Params) {
		return false
	}
	for i := 0; i < len(a.Params); i++ {
		ap := a.Params[i]
		bp := b.Params[i]
		if ap.Optional != bp.Optional {
			return false
		}
		if !c.check(ap.Type, bp.Type, depth+1) || !c.check(bp.Type, ap.Type, depth+1) {
			return false
		}
	}
	if a.Variadic == nil && b.Variadic == nil {
		return true
	}
	if a.Variadic == nil || b.Variadic == nil {
		return false
	}
	return c.check(a.Variadic, b.Variadic, depth+1) && c.check(b.Variadic, a.Variadic, depth+1)
}

func (c *checker) canWidenRecordTo(narrow, wide *typ.Record, depth int) bool {
	for _, wf := range wide.Fields {
		nf := narrow.GetField(wf.Name)
		if nf == nil {
			continue
		}
		if !c.check(wf.Type, nf.Type, depth+1) && !c.canWidenTo(nf.Type, wf.Type, depth+1) {
			return false
		}
	}
	for _, wm := range wide.StaticMembers {
		nm := narrow.GetStaticMember(wm.Kind, wm.Name, wm.Index)
		if nm == nil {
			continue
		}
		if !c.check(wm.Type, nm.Type, depth+1) && !c.canWidenTo(nm.Type, wm.Type, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkArray(sub, super *typ.Array, depth int) bool {
	return c.check(sub.Element, super.Element, depth+1)
}

func (c *checker) checkMap(sub, super *typ.Map, depth int) bool {
	if !c.check(sub.Key, super.Key, depth+1) || !c.check(super.Key, sub.Key, depth+1) {
		return false
	}
	if !c.check(sub.Value, super.Value, depth+1) {
		return false
	}
	return c.check(super.Value, sub.Value, depth+1) || typ.IsAny(super.Value)
}

func (c *checker) checkReadonlyMap(sub, super *typ.ReadonlyMap, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	return c.check(sub.Key, super.Key, depth+1) &&
		c.check(typetable.PresentReadonlyEntryValue(sub.Value), super.Value, depth+1)
}

func (c *checker) checkTuple(sub, super *typ.Tuple, depth int) bool {
	if len(sub.Elements) != len(super.Elements) {
		return false
	}
	for i, e := range sub.Elements {
		if !c.check(e, super.Elements[i], depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkInterface(sub, super *typ.Interface, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	if len(super.Methods) == 0 && len(sub.Methods) == 0 {
		return sub.Name == super.Name
	}
	for _, superMethod := range super.Methods {
		found := false
		for _, subMethod := range sub.Methods {
			if subMethod.Name != superMethod.Name {
				continue
			}
			if !c.check(subMethod.Type, superMethod.Type, depth+1) {
				return false
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func (c *checker) checkRecordToMap(sub *typ.Record, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, f := range sub.Fields {
		if !c.check(typ.LiteralString(f.Name), super.Key, depth+1) {
			return false
		}
		if !c.check(f.Type, super.Value, depth+1) {
			return false
		}
	}
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.Value, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkRecordToReadonlyMap(sub *typ.Record, super *typ.ReadonlyMap, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, f := range sub.Fields {
		if !c.check(typ.LiteralString(f.Name), super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(f.Type), super.Value, depth+1) {
			return false
		}
	}
	for _, m := range sub.StaticMembers {
		keyType, ok := readonlyStaticMemberKeyType(m)
		if !ok || !c.check(keyType, super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(m.Type), super.Value, depth+1) {
			return false
		}
	}
	if sub.Open || sub.Metatable != nil {
		if !c.check(typ.String, super.Key, depth+1) || !c.check(typ.Unknown, super.Value, depth+1) {
			return false
		}
	}
	if sub.HasMapComponent() {
		if !c.check(sub.MapKey, super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(sub.MapValue), super.Value, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkMapToRecord(sub *typ.Map, super *typ.Record, depth int) bool {
	if sub == nil || super == nil || !super.HasMapComponent() {
		return false
	}
	if !c.checkMap(sub, typetable.NewMap(super.MapKey, super.MapValue), depth+1) {
		return false
	}
	for _, sf := range super.Fields {
		if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
			return false
		}
		if !c.check(typ.LiteralString(sf.Name), sub.Key, depth+1) {
			continue
		}
		expected := sf.Type
		if sf.Optional && !unwrap.IsOptionalLike(expected) {
			expected = typ.NewOptional(expected)
		}
		if !c.check(sub.Value, expected, depth+1) {
			return false
		}
		if !c.check(expected, sub.Value, depth+1) && !c.canWidenTo(sub.Value, expected, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkArrayToMap(sub *typ.Array, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	return c.check(typ.Integer, super.Key, depth+1) && c.check(sub.Element, super.Value, depth+1)
}

func (c *checker) checkTupleToReadonlyMap(sub *typ.Tuple, super *typ.ReadonlyMap, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for i, elem := range sub.Elements {
		if !c.check(typ.LiteralInt(int64(i+1)), super.Key, depth+1) {
			return false
		}
		if !c.check(typetable.PresentReadonlyEntryValue(elem), super.Value, depth+1) {
			return false
		}
	}
	return true
}

func readonlyStaticMemberKeyType(member typ.StaticMember) (typ.Type, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return typ.LiteralString(member.Name), true
	case typ.StaticMemberIntIndex:
		return typ.LiteralInt(member.Index), true
	default:
		return nil, false
	}
}

func (c *checker) checkTupleToArray(sub *typ.Tuple, super *typ.Array, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, elem := range sub.Elements {
		if !c.check(elem, super.Element, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkTupleToMap(sub *typ.Tuple, super *typ.Map, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	if !c.check(typ.Integer, super.Key, depth+1) {
		return false
	}
	for _, elem := range sub.Elements {
		if !c.check(elem, super.Value, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkRecordToInterface(sub *typ.Record, super *typ.Interface, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, method := range super.Methods {
		field := sub.GetField(method.Name)
		if field == nil {
			return false
		}
		methodType := subst.Self(method.Type, sub)
		if !c.check(field.Type, methodType, depth+1) {
			return false
		}
	}
	return true
}

func (c *checker) checkInstantiated(sub, super *typ.Instantiated, depth int) bool {
	if sub == nil || super == nil || sub.Generic == nil || super.Generic == nil {
		return false
	}
	if !sub.Generic.Equals(super.Generic) || len(sub.TypeArgs) != len(super.TypeArgs) {
		return false
	}
	for i, a := range sub.TypeArgs {
		if !c.check(a, super.TypeArgs[i], depth+1) || !c.check(super.TypeArgs[i], a, depth+1) {
			return false
		}
	}
	return true
}

func isTableLikeType(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isTableLikeType(v.UnaliasedTarget())
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && isTableLikeType(v.Body)
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return false
	}
}

func isRecursiveRoot(t typ.Type) bool {
	_, ok := t.(*typ.Recursive)
	return ok
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
